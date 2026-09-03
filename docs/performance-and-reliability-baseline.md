# 性能与同步可靠性基线

> 状态：项目长期约束与优化待办
>
> 更新时间：2026-09-01
>
> 适用范围：Core API、Edge API、Worker、同步链路，以及 `/frontend` 下的所有客户端

## 1. 总原则

本项目追求极致性能，但性能优化不能以牺牲同步准确性、消息可靠性、业务一致性或故障可恢复性为代价。优先级如下：

```text
消息不丢失/不产生错误副作用
        ↓
Core 事实、Edge Projection 和状态收敛准确
        ↓
同步延迟、接口延迟和吞吐量
        ↓
页面加载、交互响应和内存占用
        ↓
视觉装饰和非必要动画
```

必须持续保持：

- Core 是业务事实源，Edge 是最小 Projection/Command 边界；
- Outbox、Inbox、Command、Retry、Failed 和版本条件更新不能为了“更快”而删除；
- 不能用 Redis、浏览器缓存、前端离线队列或内存队列替代可靠存储；
- 优化不得把 `202 Accepted`、本地乐观状态或发送成功误报为业务最终成功；
- 所有性能结论都应有可复现的测量结果，不能只凭主观感受改写一致性规则。

## 2. 当前系统边界

| 组件 | 性能和可靠性职责 | 不允许做的事 |
|---|---|---|
| Core API | 在短事务内完成权限、状态、事实、Audit、Outbox 和业务幂等 | 不在数据库事务中等待网络投递；不让前端绕过 Core 修改事实 |
| Worker | 有界并发投递 Outbox、拉取 Edge Command、重试并记录失败 | 不无限创建 Goroutine；不因吞吐量删除重复/乱序保护 |
| Edge API | 快速读 Projection、持久化 Command、提供同步边界 | 不创建完整 Core 业务副本；不把本地 Projection 当最终事实 |
| 前端客户端 | 只渲染服务端状态，及时显示 pending、syncing、failed 和冲突 | 不连接数据库、Redis、Outbox、Inbox 或 `/internal/sync/v1/*` |
| Redis | 可选 cache、限流、短锁、在线状态和临时去重 | 不承担事实、Outbox、Inbox、Command 或故障恢复依据 |

## 3. 前端性能与动画限制

### 3.1 必须遵守的原则

- 管理端 `admin-web`、员工小程序 `employee-miniapp` 和员工网页版 `employee-web` 都必须把性能作为交互设计的一部分；三个客户端不能因为共享 UI 而引入不必要的动画依赖。
- Confirm、Cancel、Accept、Complete、Retry 等关键操作必须立即发起请求，不能等待入场动画、过场动画或特效结束。
- 按钮点击后立即锁定重复提交，显示真实的 `submitting`/`pending` 状态；不能用动画假装已经完成。
- 页面状态必须同时提供文字、图标或明确颜色语义；动画不能是唯一的同步状态提示。
- 必须支持 `prefers-reduced-motion: reduce`；减少动效时不能隐藏业务状态和错误信息。
- 默认优先使用 CSS `transform` 和 `opacity` 的短时过渡；避免 `transition: all`。
- 任何动画都必须说明目的：反馈、空间关系、状态变化或避免突兀切换。仅为“看起来炫”且频繁出现的动画不允许加入。

### 3.2 默认允许的轻量动画

以下是性能友好的默认范围，具体数值在前端工程初始化后通过设备测试校准：

| 场景 | 默认建议 |
|---|---|
| 按钮按下反馈 | `transform: scale(0.97)`，约 100–160ms |
| 小型 Popover/Tooltip | 只使用 `transform`/`opacity`，约 125–200ms；连续移动时可跳过动画 |
| Toast/状态徽标 | 只在首次出现或状态变化时过渡，约 120–200ms |
| Modal/Drawer | 约 150–250ms；不阻塞请求和数据刷新 |
| 列表刷新 | 默认不做整表 stagger；最多突出真正变化的行或状态徽标 |

UI 动画原则上不超过 300ms。高频操作、键盘操作、轮询刷新和实时状态变化应尽量即时更新。

### 3.3 默认禁止或需要专项论证的效果

以下效果在本项目默认禁止；确有业务价值时，必须提供设备测量、降级方案和评审记录：

- 全屏视频背景、粒子、WebGL、Canvas 持续绘制、复杂 3D、视差和鼠标跟随；
- 大面积 `filter: blur()`、`backdrop-filter`、持续变化的阴影或高成本混合效果；
- 动态渐变、无限循环装饰、自动轮播和与业务无关的持续运动；
- 大型 Lottie、复杂 SVG morph、整页转场和列表逐项入场动画；
- 在大量任务卡片、表格行或 Projection 高频刷新时，为每一条记录启动独立动画；
- 为了动画而修改 `width`、`height`、`margin`、`padding`、`top`、`left` 等会触发布局的属性；
- 无上限使用 `will-change`、`requestAnimationFrame` 或第三方动画库；
- 在小程序中高频、大对象 `setData` 或重复创建页面级定时器。

动画不是性能优化手段。加载中优先采用稳定的静态骨架、进度文字和真实请求状态，不用高频 shimmer 掩盖慢接口。

## 4. 前端性能优化要求

### 4.1 渲染和代码体积

- 按路由和功能拆分代码；管理端、员工 Web 和小程序不得加载彼此不需要的运行时依赖。
- 长列表必须分页；当列表规模达到需要评估的阈值时使用虚拟列表或窗口化渲染，不能一次渲染全部历史数据。
- 只请求页面需要的字段，避免把完整 Core 对象或完整历史记录复制到客户端。
- 共享包只共享 contracts、API client、认证适配器、UI 基础能力和展示状态映射；不通过共享包引入数据库或后端业务依赖。
- React 组件的 `memo`、缓存和状态提升必须由测量结果驱动，避免为了“优化”增加无效复杂度和内存占用。
- 小程序只更新发生变化的数据，避免大对象状态整体刷新；具体平台 API 规则在 F1 工具链冻结后验证。

### 4.2 请求、缓存和同步展示

- GET 查询使用请求去重、有限缓存、分页和取消过期请求；切换页面或筛选条件时应取消不再需要的请求。
- 不默认高频轮询。需要刷新时使用后端确认的间隔、退避和可见性策略；实时通道若未来引入，必须先明确断线重连、顺序、重复和降级语义。
- POST/Command 只有在后端支持稳定幂等键时才能自动重试；网络超时不能盲目生成新的业务 ID。
- 202 只显示“已提交/等待同步”；只有读取到 Core Event 收敛后的 Projection 或明确的 Core 结果，才显示最终业务状态。
- 本地缓存只能改善读取体验，不能作为可靠离线队列、最终状态或消息传递依据。

### 4.3 内存和生命周期

- 页面卸载、请求结束或组件销毁时，清理 timer、listener、observer、订阅和 AbortController。
- 不在全局状态中长期保存完整任务历史、大型响应、重复的 Projection 或敏感凭据。
- 不为每个列表项创建常驻定时器、动画循环或闭包引用；状态徽标和同步提示应随数据生命周期销毁。
- 性能回归需要同时看加载时间、主线程任务、帧率、JS 堆、网络大小和同步状态正确性，不能只看单一 FPS。

## 5. 后端性能优化待办

以下是后端未来待办，不代表已经实现。执行顺序必须先测量，再做针对性优化。

| 优先级 | 待办 | 验收重点 |
|---|---|---|
| P0 | 建立 Core API、Edge API、Worker 的 p50/p95/p99、错误率、Outbox/Inbox 延迟和 Projection lag 指标 | 能区分接口慢、数据库慢、Worker 慢、Edge 不可用和真实业务失败 |
| P0 | 保持 Outbox/Inbox/Command 的原子性、幂等、版本收敛和失败可重试 | 压测后仍不丢消息、不产生重复业务副作用，乱序事件不会覆盖新状态 |
| P1 | 审查 Core/Edge MySQL 索引、慢查询、`EXPLAIN`、N+1 查询和连接池参数 | Task 列表/详情、Inbox、Outbox、Command 查询有实际查询计划和边界测试 |
| P1 | 优化 Task 列表分页和 Projection 查询，必要时采用 keyset/cursor 分页 | 大数据量下不使用无界 `OFFSET` 或 `SELECT *`；权限 Scope 仍在查询边界生效 |
| P1 | 优化 Worker 投递：有界并发、批量拉取、连接复用、租约条件更新、退避和抖动 | Worker 重启、重复领取、网络失败和 Edge 恢复后仍能安全收敛 |
| P1 | 统一事件/Command payload 大小、超时、压缩和 HTTP keep-alive 策略 | 减少传输开销但不截断必要字段、不改变签名/版本/幂等语义 |
| P2 | 使用 Redis 做可失效热点读取缓存和限流优化 | 缓存失效或 Redis 不可用时，Core/Edge 仍以 MySQL 和同步记录正常工作 |
| P2 | 使用 Go pprof、trace 和分配分析定位 CPU、堆、锁竞争和 Goroutine 泄漏 | 先有基线和火焰图，再提交优化；不为微小收益引入难维护的代码 |
| P2 | 建立 API、双库同步、Worker 重启、Edge 中断和大数据量压测 | 同时验证吞吐、延迟、消息准确性、重复请求、乱序和恢复时间 |
| P2 | 把性能预算和同步延迟回归纳入 CI/发布门禁 | 代码、Migration、API 契约和前端构建变化都能发现性能回退 |

后端优化明确不包括：为了追求吞吐而移除 Outbox/Inbox、取消数据库幂等约束、依赖 Redis 作为可靠队列、把 Core 拆成未经验证的微服务，或让前端直连数据库。

## 6. 性能验收门槛

当前前端工程尚未完成，以下是待 F1/F8 验证的目标，不是当前已通过结果：

- Web 关键交互目标为 INP ≤ 200ms、LCP ≤ 2.5s、CLS ≤ 0.1；实际需要在代表性设备和真实数据规模上测量。
- 关键操作的请求不能被动画延迟；动画只影响视觉，不改变提交、确认、重试和刷新时序。
- 关键页面在常规负载下以 60 FPS 为目标；出现持续掉帧时优先移除非必要特效，而不是降低同步刷新频率。
- Task 列表、状态刷新、Command pending/confirmed/failed 和版本冲突场景必须同时完成性能与正确性验收。
- 后端验收必须记录 API p95/p99、数据库查询耗时、Outbox→Edge 和 Command→Core→Edge 的端到端延迟、失败重试次数和最终收敛结果。
- 任何性能测试如果访问真实后端、数据库或 Compose，必须先执行 `scripts/ensure-docker.ps1`；未运行的测试不能写成已通过。

## 7. 关联文档

- [前端工程设计基线](../memory-bank/frontend-design-document.md)
- [前端技术栈与工程结构](../memory-bank/frontend-tech-stack.md)
- [前端实施计划](../memory-bank/frontend-implementation-plan.md)
- [前后端交接文档](frontend-backend-handoff.md)
- [Architecture v2.0 架构记录](../memory-bank/architecture.md)

## 8. T0-1 可观测性实现状态（2026-09-02）

T0-1 的采集代码已经接入，但当前没有把一次短暂的本地运行结果写成性能结论。运行负载、数据库状态和容器资源不同，必须用同一套请求量、数据规模和环境再次采集后，才能确定 p50/p95/p99 目标和瓶颈。

### 8.1 指标入口

- Core API：`GET /metrics`，提供 HTTP 请求和 Core 数据库连接池指标。
- Edge API：`GET /metrics`，提供 HTTP 请求、Edge 数据库连接池、待处理 Command 和失败 Inbox 指标。
- Worker：`FLIGHT_WORKER_METRICS_ADDR`，默认监听 `:9090`，提供 Outbox、Command 拉取/处理/确认和 Core 数据库连接池指标；本地 Compose 使用 `WORKER_METRICS_HOST_PORT` 暴露宿主机端口。

指标接口应只在内网或受保护的观测网络暴露，不应成为员工端公共 API。指标标签只使用组件、HTTP 方法、路由模板、状态码、队列和操作结果，不包含员工 ID、Command ID、原始 URL 或敏感错误文本。

### 8.2 当前采集的核心指标

| 指标 | 说明 |
|---|---|
| `flight_http_requests_total` | 按组件、方法、路由模板和状态码统计请求数 |
| `flight_http_request_duration_seconds` | HTTP 延迟直方图，可由监控系统计算 p50/p95/p99 |
| `flight_sync_queue_pending_items` | Core Outbox、Edge Command 的待处理数，以及 Edge Inbox 失败数 |
| `flight_sync_queue_oldest_age_seconds` | 各持久同步队列最老待处理记录年龄 |
| `flight_worker_outbox_delivery_duration_seconds` | Core Outbox 到 Edge 的投递延迟直方图 |
| `flight_worker_command_processing_duration_seconds` | Core Command 处理延迟直方图 |
| `flight_worker_command_ack_duration_seconds` | Worker 确认 Edge Command 的延迟直方图 |
| `flight_db_*` | `database/sql` 连接数、使用中连接、空闲连接和等待压力 |
| `flight_notification_delivery_total` | Edge best-effort 任务提示按 `delivered`、`no_subscribers`、`failed` 结果统计 |
| `flight_websocket_connections` | 当前 Edge 实例的活跃员工 WebSocket 连接数 |
| `flight_websocket_connections_total` | 成功建立的 WebSocket 连接数 |
| `flight_websocket_disconnects_total` | 按 `read_error`、`write_error`、`idle_timeout`、`server_shutdown` 统计断开原因 |
| `flight_websocket_heartbeat_total` | 按服务端 ping、客户端 pong/ping 统计应用层心跳 |
| `flight_websocket_notification_delivery_seconds` | Projection 提示签发到当前连接写出的延迟直方图 |

### 8.3 T0-1 未完成门禁

- [ ] 在隔离环境执行固定数量的任务读取、Command 和双向同步负载。
- [ ] 保存包含请求量、数据规模、批量大小、轮询间隔、连接池和容器版本的基线报告。
- [ ] 对比 API 延迟、数据库等待、Worker 延迟、队列 backlog 和 Projection lag，确认下一步优化对象。
- [ ] 在完成基线前，不宣称 HTTP 点对点链路、单 Worker 或数据库已经是确定瓶颈，也不据此强制引入持久消息 Broker。

### 8.4 T0-2 拉取恢复契约

- `GET /api/v1/tasks` 是按 JWT Principal 过滤的完整 Edge Projection 快照；响应包含 `snapshot_at` 和员工级 `projection_revision`。
- `projection_revision` 是持久化的员工级变化序列，不能用单 Task `sync_version` 作为员工全局游标；当前 `next_cursor` 固定为 null，尚未开放增量读取。
- `projection_lag_state=unknown` 表示 Edge 尚无可用的已应用事件时间基准，不等于零延迟；`known` 时客户端只展示观测值，不把它当业务确认。
- 启动、刷新、重连和离线恢复统一重新拉取完整快照；空数组是合法空结果，取消/完成以快照中的终态为准。Command 恢复必须继续使用稳定 `command_id` 和公共 Command status API。
- Edge MySQL 需要按顺序应用 `migrations/edge/mysql/000004_employee_projection_cursor`；代码不会启动时自动建表或调用 AutoMigrate。

### 8.5 T0-3 通知抽象和提交后边界

- Edge 内部事件先完成 Inbox/Projection 的 `ApplyEvent`，只有成功且非重复的任务事件才发布 `task_changed` 提示；通知失败不回滚 Projection，也不改变事件已应用的响应。
- `flight_notification_delivery_total{component="edge",result="delivered|no_subscribers|failed"}` 只用于观测提示投递结果，不是可靠队列深度，也不代表客户端已刷新到最终状态。
- 单实例内存 fan-out 只保存当前连接注册表；进程重启、连接断开、跨副本未命中和提示丢失，均由员工下一次完整 `GET /api/v1/tasks` 快照和 `projection_revision` 恢复。
- T0-3 的定向测试和全量 `scripts/verify.ps1 -Mode all` 已通过；WebSocket 鉴权/心跳/重连和多副本临时 fan-out 尚未实现，不能据此宣称强实时或多副本交付已验收。

### 8.6 T0-4 WebSocket 提示实现

- Edge 已提供员工 ticket 接口和 WebSocket 入口；ticket 只在短时间内有效且单次消费，握手使用 Edge JWT/sid 解析出的员工身份、同源 Origin 和 `flight.realtime.v1`。
- 连接使用应用层 `ready`/`ping`/`pong`，并设置读写超时、空闲关闭和连接清理；指标只使用组件、结果、断开原因和方向等低基数标签，不包含员工 ID、任务 ID、连接 ID 或 ticket。
- `task_changed` 的延迟指标只反映 best-effort 提示写出时间，不代表客户端已完成 HTTP 快照刷新，也不代表业务状态已确认。Projection、Outbox/Inbox 和 Command status 仍是可靠链路。
- 当前代码级测试覆盖连接建立、员工隔离、提示投递、心跳、重复提示去重、退避重连和空闲关闭；真实浏览器网络、服务重启、网络分区、连接数容量和多副本 fan-out 基线仍待 T0-5/T0-7。
### 8.7 T0-5 Gateway 与多副本

- Gateway 只负责转发和 WebSocket Upgrade；真实性能验收仍需分别记录 Gateway、Edge API 以及 Worker 的 p50/p95/p99，不得把 Gateway 的平均值当作端到端结果。
- 多 Worker 验收必须记录 lease claim 冲突、租约过期恢复、ack 拒绝和 Outbox/Command backlog；租约不能通过缩短 TTL 换取吞吐而破坏处理安全。
- Redis fan-out 只记录提示发布失败与重连状态；Redis 停止时应验证 Projection、HTTP 全量快照、projection revision 和可靠同步仍然收敛。
- 双 Edge 验收必须记录 Gateway 后端分布、WebSocket 重连、单副本停止后的恢复时间和重复/乱序提示；Docker CLI 恢复前这些指标均为待测目标。
