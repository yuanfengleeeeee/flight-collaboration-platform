# ADR-009 员工任务实时交付：推送提示与拉取校准

状态：T0-4 已完成；T0-5 Gateway、双 Edge、租约和共享实时基础设施已实现，双副本/故障 Docker 验收待环境恢复

日期：2026-09-02

## 背景

当前系统只有一个逻辑 Edge 服务。Core 是任务、Assignment、Personnel 和状态机的唯一事实源，Core 事务写入 Outbox 后，由 Worker 通过内部 HTTP 将版本化事件投递到 Edge；Edge 持久化 Inbox 和 Task Projection。员工端通过带有 Edge audience 的 JWT 调用 `GET /api/v1/tasks`，Edge 根据认证后的员工 Principal 过滤出该员工的 Projection。

在 T0-4 实现前，系统没有 WebSocket、SSE 或微信订阅消息接入；当前已增加员工 WebSocket 提示，但员工端任务读取仍是主动拉取，Core 到 Edge 的同步仍是服务端异步投递，两者不能混称为同一类“推送”。

## 决策摘要

采用“持久化状态为准、前台推送提示、客户端拉取校准、后台平台通知”的混合方案：

1. 不按任务类型拆分 Edge。Edge 是统一的员工接入与移动端 Projection 服务；扩容时部署多个相同副本。任务类型通过 Core 事件和 Edge Projection 字段区分，不通过不同 Edge 服务区分。
2. `GET /api/v1/tasks` 保留为员工任务的权威读取入口。客户端启动、刷新、重连、收到推送提示后，都可以从该接口获得可恢复的任务状态。
3. 前台实时体验使用 WebSocket 推送轻量的 `task_changed` 提示。提示只表示 Projection 可能发生变化，不代表客户端已经取得最终状态，也不替代数据库持久化和 HTTP 读取。
4. 后台或小程序退出后，使用经过用户授权的微信订阅消息等平台通知能力做提醒。平台通知只用于召回和提醒，不承担可靠同步。
5. 多 Edge 副本的连接表只保存在实例内存中；跨副本 fan-out 使用可选的临时通知适配器。Redis Pub/Sub 只能承担易丢失的提示分发，不能承担 Core/Edge 可靠同步；可靠事实仍由 Core Outbox、Edge Inbox 和 MySQL 保存。
6. T0 不强制引入 Kafka、RabbitMQ、NATS JetStream 或 Service Mesh。是否增加持久消息 Broker，必须等性能和堆积指标证明 HTTP Worker 链路已经成为瓶颈后再做单独 ADR。

## 拉取与推送的职责边界

| 机制 | 发起方 | 主要职责 | 可否作为最终事实 |
| --- | --- | --- | --- |
| HTTP 拉取 | 员工客户端 | 获取任务快照、刷新、重连恢复、校准状态 | 可以，来源是 Edge Projection |
| WebSocket | 员工客户端先建立连接，服务端发送提示 | 降低前台变化感知延迟 | 不可以，提示允许丢失和重复 |
| 微信订阅消息 | 服务端调用平台接口，用户事先授权 | 后台提醒和召回 | 不可以，受平台授权和下发约束 |
| Core Outbox/Edge Inbox | Core/Worker/Edge | 可靠传递业务事件和形成 Projection | 可以，受持久化幂等链路保护 |

推送消息必须允许丢失、重复和乱序；客户端不能因为收到或未收到某条提示而直接改变业务状态。客户端应使用任务版本、Projection 时间或同步游标重新读取数据，并让 Edge 的 `sync_version` 单调收敛规则保护最终状态。

## 目标链路

```text
Core 事务
  ├─ Task/Assignment/Personnel/History/Audit
  └─ Core Outbox
          ↓
       Worker
          ↓ 版本化 Event，at-least-once
    Edge Inbox + Task Projection 事务提交
          ↓ 提交后、尽力而为
    Notification Fan-out
       ├─ 当前 Edge 实例上的 WebSocket 连接
       ├─ 其他 Edge 实例的临时 fan-out 通道
       └─ 后台平台通知适配器（需用户授权）

员工端
  ├─ 首次进入/刷新/重连：GET /api/v1/tasks
  ├─ 前台：接收 task_changed 后再次拉取
  ├─ received/start/complete：HTTP Command + command_id
  └─ Command 结果：GET /api/v1/commands/{commandID}，推送只能作为提示
```

Edge 必须在 Projection 事务提交之后再发出 `task_changed`。如果进程在提交后、通知前崩溃，客户端会在下一次打开或重连时通过拉取恢复；因此不需要为了保证业务正确性而把 WebSocket 变成第二套可靠消息存储。

## 员工端实时协议原则

### 1. 连接与身份

- WebSocket 使用与普通 Edge API 相同的员工 JWT、`sid` 和 Edge audience 校验。
- 不使用 `X-Employee-Public-ID` 作为生产身份；开发 Header 仍只允许显式开发适配器。
- 不把长期 JWT 放在 URL 查询参数中；如果客户端平台不能在握手时安全传递 Authorization，应使用一次性短期连接票据。
- 每条连接绑定一个已认证的员工 Principal。服务端只向该连接发送该员工可见的提示。

### 2. 提示内容

首期只发送最小变化通知，例如：

```json
{
  "type": "task_changed",
  "notification_id": "uuid",
  "task_public_id": "uuid",
  "sync_version": 12,
  "reason": "assigned",
  "issued_at": "2026-09-02T08:00:00Z"
}
```

不在广播消息中放置不必要的员工、航班或任务敏感字段。客户端收到提示后调用任务读取接口；如果提示只包含任务 ID 和版本，服务端未来可以在不改变推送协议的情况下增加详情和增量读取。

### 3. 重连与恢复

- 客户端断线后使用带上限的指数退避重连，并避免多个页面重复建立连接。
- 连接建立成功后必须执行一次拉取；连接存活期间也不能跳过定期的完整校准。
- T0 先保留完整 `GET /api/v1/tasks` 作为恢复路径；增量接口必须另行定义稳定游标，不能直接把每个 Task 的 `sync_version` 当作全局游标。
- 若未来启用增量接口，响应必须包含 `next_cursor` 和 `reset_required`。游标失效或发现版本缺口时，客户端回退到完整快照。
- 提示不保证顺序、不保证恰好一次。客户端按任务 ID 和版本幂等处理，最终以拉取结果为准。

### 4. Command 状态

received/start/complete 仍然先通过 HTTP 将 Command 持久化到 Edge，并返回 `202 Accepted`；客户端使用稳定 `command_id` 和 `GET /api/v1/commands/{commandID}` 查看状态。WebSocket 可以发送 `command_changed` 作为刷新提示，但不得直接把 `pending` 或 `confirmed` 当成 Core 已完成的业务事实。`received` 只确认员工已收到通知，不是同意；员工没有拒绝任务命令。

## 多 Edge 副本与中间件边界

当 Edge 扩容为多个相同副本时：

```text
Gateway / Load Balancer
       ├─ Edge API 1 ─┐
       ├─ Edge API 2 ─┼─ shared Edge MySQL
       └─ Edge API N ─┘
```

- Gateway 负责 TLS、健康检查、限流和客户端请求分发；Worker 可通过内部服务地址或内部负载均衡访问同步接口。
- Edge 的 Projection、Command、Session、Inbox 必须保存在共享的 Edge MySQL 中，不能依赖某个副本的内存状态。
- 每个副本只维护自己持有的 WebSocket 连接。Core Event 被任一副本成功投影后，需要通过临时 fan-out 让其他副本上的对应员工连接获得提示。
- Redis Pub/Sub 可以作为低成本的 best-effort fan-out；Redis 不可用时，系统应降级为无实时提示但仍可通过 HTTP 拉取恢复。
- 如果未来需要高吞吐、跨区域、可回放的持久事件流，再评估 NATS JetStream、RabbitMQ 或 Kafka/Redpanda；这属于独立的中间件 ADR，不在 T0 默认范围内。

## T0 修改顺序

T0 表示实时任务体验和横向扩展前必须完成的顺序，不表示所有步骤已经实现。

### T0-0：冻结边界与契约

先冻结本 ADR 的职责边界：一个逻辑 Edge、统一任务读取、推送为提示、拉取为校准、Core/Edge Outbox/Inbox 不变。同步补齐 WebSocket 消息、订阅通知、重连、版本、错误和降级语义的 OpenAPI/协议文档。

验收：客户端、后端和测试不会把 WebSocket、微信通知或 HTTP 202 误认为业务最终确认；不会出现按任务类型拆 Edge 的接口或部署方案。

### T0-1：建立可观测性基线

在任何性能优化前记录：

- Core Outbox backlog 和 oldest age；
- Edge Inbox backlog、失败数和重试数；
- Core→Edge Projection lag；
- Worker 拉取、处理、确认的 p50/p95/p99 和 batch size；
- Edge `GET /api/v1/tasks`、Command status 的 p50/p95/p99；
- MySQL 查询耗时、锁等待、连接池使用率和慢查询；
- WebSocket 连接数、重连率、提示延迟、丢弃数和 fan-out 错误数。

验收：形成可重复的基线报告，并据此决定是否需要 Gateway、批量确认、更多 Worker 或持久 Broker；不能用“感觉点对点慢”替代测量。

### T0-2：先把拉取恢复契约做完整

- 保留 `GET /api/v1/tasks` 作为完整快照接口；继续按 JWT Principal 在 Edge 侧过滤员工任务。
- 明确快照时间、Projection 版本、空结果、任务取消/完成和 Projection lag 的客户端显示语义。
- 设计员工级变化序列或稳定增量 cursor；在游标未冻结前，使用完整快照恢复，不让客户端拼接时间戳。
- 与 `GET /api/v1/commands/{commandID}` 对齐，确保 Command 重试和状态刷新可恢复。

当前实现：`GET /api/v1/tasks` 返回 `sync_mode=full_snapshot`、`snapshot_at`、员工级持久化 `projection_revision`、`projection_lag_seconds`、`projection_lag_state`、`next_cursor=null` 和 `reset_required=false`。`projection_revision` 在 Edge Projection 新增或更高版本更新时递增，同版本幂等重放和过期版本不会递增；任务转派时原员工和新员工的 revision 都会递增。Projection lag 沿用 Edge 最近已应用事件的可观测口径，没有已应用事件时返回 `unknown`。

客户端语义：HTTP 200 且 `items=[]` 表示该员工当前没有任务，不是错误；取消和完成任务在快照中保留其终态，客户端以更新后的快照为准。启动、手动刷新、重连、收到未来的变化提示以及离线恢复都执行完整快照替换；本地缓存只改善体验，不能作为可靠事实。待处理 Command ID 独立保留，并通过 `GET /api/v1/commands/{commandID}` 查询和恢复状态。当前没有启用 `since_revision` 或增量 API，因此 `next_cursor` 保持 null，`reset_required` 保持 false。

验收：杀掉连接、丢弃一条提示或让客户端离线后，重新拉取可以得到正确任务状态；重复拉取无副作用；不使用单 Task `sync_version` 拼接员工全局游标。

### T0-3：加入通知抽象和提交后边界

- [x] 在 Edge 内增加 Notification/Fan-out Port，不让业务 Handler 直接操作连接。
- [x] Projection 事务提交后再通知；通知失败只记录指标和日志，不回滚 Core 事实或 Edge Projection。
- [x] 定义员工隔离、`notification_id`、任务 ID、版本和原因字段。
- [x] 首期使用单实例内存连接注册表；连接注册表不能保存业务事实。

当前实现位于 `internal/edge/application/notification`：`TaskChanged` 只发送 `task_public_id`、`sync_version`、`reason` 和 `issued_at` 等最小字段，员工 ID 只作为实例内路由键，不进入通知载荷。`InMemoryFanout` 按员工和连接 ID 隔离订阅，单个连接失败不会阻止其他连接尝试投递；连接注册表不持久化，重启、断线或丢失通知都通过下一次完整任务快照恢复。

Edge 内部事件入口先完成 `ApplyEvent` 投影事务，再为已成功且非重复的任务事件发布提示；重复 Event 不再次发布。通知构造或投递失败只产生结构化日志和 `flight_notification_delivery_total` 指标，仍返回事件已应用的 `202`，不改变可靠同步语义。

验收：Projection 写入成功但通知组件失败时，任务仍能通过 HTTP 拉取；同一提示重复发送不会产生业务副作用。

### T0-4：实现前台 WebSocket 提示

- [x] 使用 Edge JWT/sid 完成握手鉴权；浏览器通过员工 Bearer JWT 获取一次性短期 ticket，再以 `Sec-WebSocket-Protocol` 传递 ticket，不把凭据放入 URL。
- [x] 实现同源校验、`flight.realtime.v1` 协议协商、服务端 `ready`/`ping`、客户端 `pong`、空闲关闭、写超时和连接清理。
- [x] 推送 `task_changed` 作为 best-effort 刷新提示；员工 Web 客户端按 `notification_id` 去重，收到提示或重连成功后重新拉取完整 `GET /api/v1/tasks` 快照。
- [x] 实现客户端指数退避重连；ticket 获取遇到 401 时停止并回到认证恢复路径，WebSocket 不可用不阻塞 HTTP 任务读取和 Command status 查询。

代码级验收已覆盖一次性 ticket、JWT 员工隔离、协议握手、心跳回复、通知投递、重复提示去重、断线重连和空闲关闭；员工 Web 的真实登录、UTF-8 Projection 和浏览器 WebSocket `ready` 已由 Playwright E2E 验证。服务重启、网络分区和多副本连接 fan-out 仍留待 T0-5/T0-7；这些场景不能仅凭单元测试宣称已完成。

### T0-5：Gateway 与多 Edge/Worker 横向扩展

- 增加 Gateway/Load Balancer，移除多副本不能共用的固定宿主机端口暴露方式。
- 使用共享 Edge MySQL；确认 Session、Command、Projection、Inbox 在任意副本上可读写。
- 通过安全的数据库 claim/lease 让多个 Worker 并行处理，不削弱 Outbox/Inbox/Command 幂等。
- 多 Edge fan-out 使用临时通知适配器；临时通道故障时自动退化为 HTTP 拉取。

验收：两个 Edge 和两个 Worker 同时运行时，重复 Event/Command 只有一次业务效果；停止任一 Edge 后 Gateway、重连和拉取仍能恢复；停止 Redis 只影响实时提示，不影响事实同步。

### T0-6：后台平台通知适配器

- 为微信小程序订阅消息、企业微信或其他平台建立独立 Provider Port。
- 只有用户已授权、事件满足产品通知条件时才尝试发送。
- 消息发送失败进入重试/观测链路，但不阻塞 Core 事务和 Edge Projection。
- 通知内容只做提醒和跳转，不承载完整任务事实；用户进入页面后仍执行 HTTP 拉取。

验收：未授权、拒绝授权、平台不可用和重复发送都不会影响任务状态；通知点击后可以通过 Edge 读取到最新 Projection。

### T0-7：故障、容量与最终门禁

覆盖 Edge 重启、Worker 重启、Redis/临时 fan-out 不可用、WebSocket 断线、网络分区、重复/乱序 Event、重复 Command、数据库锁等待和大量员工同时连接。最终报告必须区分：

- 业务事实是否正确；
- Projection 是否最终收敛；
- 推送是否及时；
- 拉取是否可恢复；
- Gateway/副本是否达到测量目标。

没有真实指标或测试结果时，不得宣称达到“强实时”、HA、RPO 或 RTO 目标。

## 明确不做

- 不为不同任务类型创建不同 Edge 服务。
- 不让员工设备直接连接 Core 或 Core 数据库。
- 不让 WebSocket、Redis Pub/Sub 或微信订阅消息成为唯一可靠数据通道。
- 不为了实时体验删除 Outbox、Inbox、幂等、版本控制、Retry 或 Command status。
- T0 不拆 Core 的 Flight、Task、Personnel、Event、Rule 模块为微服务。
- T0 不默认引入持久消息 Broker；若指标证明需要，另行提交中间件选型和运维方案。
