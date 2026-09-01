# Architecture v2.0 基线

> 状态：Accepted / Frozen
> 冻结日期：2026-08-13
> 适用范围：本仓库的 Core、Edge、Worker、迁移、同步协议和部署边界

## 1. 最终架构定义

航空保障智能协同平台只服务单机场。最终工程形态是：

> 单仓库 + 内网 Core 模块化单体 + 云端 Edge 接入服务 + Core/Edge 独立数据库 + Core 唯一业务事实源 + Edge Projection + 可靠、幂等、可恢复的双向同步。

这不是微服务架构。`flight`、`task`、`personnel`、`event`、`rule` 等核心领域继续位于 Core 模块化单体内，不拆成独立服务。

Architecture Foundation 已完成容器级验收并冻结。BVS2-03 `Flight → Task → Candidate`、BVS2-04 `Leader Confirm` 已完成，BVS2-05 `Edge Projection → Employee Command` 代码已实现；真实双库/Compose 验收结果以最新 `memory-bank/progress.md` 为准。旧 B3/B4/B5 现场仍保持 legacy/paused，后续业务阶段必须继续遵守本文件的 Core/Edge、数据所有权和同步边界。

## 2. 单机场边界

系统组织范围固定为：

```text
单机场
└── OperationArea
    └── Team
        └── Personnel

Flight / Task / Event / Rule
```

禁止引入：`Tenant`、`tenant_id`、`Airport`、`airport_id`、airport scope、多机场隔离和机场切换功能。`AccessScope` 只支持 `global`、`area`、`team`、`assigned`、`self`。

## 3. 部署拓扑

```text
机场/公司内网
  管理端 Web -> Internal Gateway -> Core API -> Core MySQL
                                      |          -> optional Core Redis
                                      |          -> Transactional Outbox
                                      |          -> Core Sync Worker
                                      |
                              安全同步边界
                                      |
                         HTTPS / production mTLS
                                      |
公网/云端 Edge 区域
  微信小程序 -> Edge API -> Edge MySQL
                           -> optional Edge Redis
```

主要程序：

- `core-api`：内网管理端和核心业务入口，内部是模块化单体。
- `edge-api`：公网移动端入口，只访问 Edge DB 中的 Projection、Session、Command 等数据。
- `worker`：负责 Core Outbox 投递、Edge Command 拉取和重试；不承载 HTTP 业务路由。
- `migrate`：只执行指定数据库的版本化 SQL migration。

Core/Edge API 均尽量无状态。关键状态不得只保存在进程内存中。

## 4. Core 模块边界

Core 目标模块为 `iam`、`organization`、`flight`、`personnel`、`task`、`event`、`rule`、`notification`、`analytics`、`device`、`prediction`。本轮只实现 Foundation 所需骨架、端口和 Probe 所需最小存储，不生成无业务价值的空模块模板。

模块内可先采用 `handler.go`、`service.go`、`repository.go`、`model.go`、`dto.go`。Handler 不调用 GORM；Service 不依赖 `gin.Context`；Domain 不依赖 Gin、GORM、Redis。Repository 接口定义在使用它的业务侧，模块不得直接访问另一模块的数据表。

复杂跨模块用例由 Core Application 层编排，通过 Flight/Task/Personnel/Authorization 等 Port 协作。异步副作用优先由版本化事件表达。

## 5. Core 数据所有权

Core 是以下数据的唯一核心事实源：航班、人员、岗位、能力、人员状态、任务模板、任务实例、任务分配、事件、规则、审计和业务状态机。

Core Business Transaction 中需要同步的变化必须同时写入：

```text
业务数据 + audit_log + outbox_event
```

三者在同一个 Core MySQL transaction 内提交。Edge 不参与该事务，也不因为 Edge 暂时不可用而回滚核心业务。

## 6. Edge Projection 与数据库隔离

Core MySQL 和 Edge MySQL 是独立实例/容器、独立账号、独立 schema 和独立 migration。禁止 Edge API 连接 Core DB、Core API 直接写 Edge DB、跨库 JOIN 或复制完整 Core 表。

Edge 只保存移动端最小数据，例如：

```text
task_projection
notification_projection
mobile_command
mobile_session
sync_inbox
delivery_log
```

`task_projection` 只允许包含移动端执行所需字段：`public_id`、`employee_public_id`、`flight_display_no`、`task_name`、`area_name`、`planned_at`、`status`、`message`、`sync_version`、`updated_at`。完整航班敏感信息、完整人员档案和完整事件数据不得同步到 Edge。

## 7. 同步协议

跨系统事件使用版本化 Event Envelope：

```text
event_id
event_type
schema_version
aggregate_type
aggregate_id
occurred_at
producer
correlation_id
trace_id
payload
```

事件类型必须带版本，例如 `task.assigned.v1`、`task.completed.v1`。不兼容修改必须生成 `.v2`，不得复用旧版本号。

Edge → Core Command 使用：

```text
command_id
command_type
schema_version
actor_public_id
aggregate_id
occurred_at
trace_id
payload
```

同步语义是 `at-least-once delivery + idempotent consumption`。`event_id` 和 `command_id` 由接收方 Inbox 唯一约束保护最终业务效果。网络层不承诺 exactly-once。

Core → Edge：Core Outbox Worker 发送事件，Edge Inbox 先幂等落库，再更新 Projection。Edge 不可用时 Outbox 保持 pending/retry，业务事务已成功。

Edge → Core：Edge 先持久化 `mobile_command`，Core Worker 主动拉取 pending command，写 Core Inbox 后交给 Core Application Service 做权限和状态机校验。Core 处理结果再通过 Outbox 更新 Edge Projection。生产传输预留 HTTPS、mTLS 和 Machine Identity；本地可使用普通 HTTP。业务代码依赖 Transport Port，不依赖网络拓扑。

## 8. 身份、安全与审计

身份分为两类：

- Human Principal：User、Role、Permission、AccessScope。
- Machine Principal：Device、ServiceAccount、IntegrationClient。设备不是普通用户角色。

当前角色固定为 `admin`、`manager`、`leader`、`staff`。权限采用 `resource:action`，例如 `flight:read`、`task:assign`、`task:complete`、`event:handle`、`analytics:read`。

Scope 使用结构化对象，不允许拼接任意 SQL：

```go
type AccessScope struct {
    Global  bool
    AreaIDs []uint64
    TeamIDs []uint64
    UserID  uint64
}
```

JWT 只携带 `sub`、`sid`、`iat`、`exp`、`iss`、`aud` 等身份/会话声明，不保存完整权限列表。认证通过 `IdentityProvider`、`Authenticator`、`Authorizer`、`Principal` 抽象与 Domain 解耦；本地允许开发密钥，生产预留 RS256/ES256、AD/LDAP/OIDC/SSO。

关键写操作写入业务 Audit，至少包含 `actor_type`、`actor_id`、`action`、`resource_type`、`resource_id`、`result`、`request_id`、`trace_id`、`source_ip`、`occurred_at`。日志不得包含密码、JWT secret、完整 token 或其他敏感凭据。

## 9. ID、时间与迁移

- 数据库内部主键：`BIGINT UNSIGNED AUTO_INCREMENT`。
- 对外同步/API/审计 ID：UUIDv7 `public_id`。
- 航班号、员工号等业务编号不作为数据库主键。
- 数据库统一保存 UTC；API 使用 ISO 8601；单机场时区由配置维护。
- 重要状态保留历史，例如 `flight_status_history`、`personnel_status_history`、`task_status_history`、`event_handle_log`。
- migration 目录严格分为 `migrations/core/mysql` 和 `migrations/edge/mysql`，服务启动禁止 GORM `AutoMigrate`。
- 基础版本从各自的 `000001_*_foundation` 开始；旧 B1/B2 migration 不作为 v2 兼容约束。

## 10. Redis、集成 Port 与预留能力

Redis 只用于 cache、rate limit、短期锁、短期去重、在线状态和临时状态；可靠同步不依赖 Redis，Redis 故障不能破坏 Core 事务和 Outbox。

所有第三方边界位于 `internal/integration/`。通知只依赖 `NotificationSender`/`PushSender` Port，当前使用 Noop/InApp Adapter；WeCom、短信、外部航班系统、RFID/UWB、AI Prediction、SSO 和消息中间件只保留稳定 Port 或 Disabled Adapter，不接入真实生产系统。

## 11. 健康、可观测性与降级

Core/Edge 都提供：

```text
/health/live   # 只代表进程存活
/health/ready  # 检查必要依赖；不暴露密码和内部堆栈
```

基础可观测性必须有 structured logging、`request_id`、`trace_id`、健康检查和同步状态日志。Prometheus、Grafana、Alertmanager、OpenTelemetry 只保留接口/配置点。

降级规则：

- Core/Edge 断网：Core 继续写核心业务；Edge 只读已同步 Projection，Command 暂存 pending，恢复后补偿。
- Edge 故障：Core 和管理端继续运行，移动端暂不可用。
- Core 故障：Edge 继续查看已同步数据，新操作显示 pending。
- Redis 故障：核心业务继续，缓存/在线状态性能下降。

## 12. 高可用与灾备目标

生产目标（当前开发环境不宣称达到）：Core API ≥2、Edge API ≥2、各自 MySQL Primary + Replica，Full Backup + Binlog + PITR。

目标值：Core API 单实例 RTO ≤1 分钟；数据库节点故障 RTO ≤10 分钟；核心数据 RPO ≤5 分钟；严重区域故障 RTO ≤60 分钟。当前只实现无状态代码边界、配置边界和恢复流程文档，不建设 Kubernetes、数据库自动 Failover 或灾备中心。

## 13. 当前业务实现边界

BVS2-03 的业务入口为 Core API `POST /api/v1/flights/{flightPublicID}/arrival`。该用例只接收 Core 管辖的 Flight 到达事实，在 Core 单事务内完成 `scheduled → arrived`、Task 生成、Candidate 快照、状态历史、Audit、业务幂等记录和 `task.generated.v1` Outbox。Candidate 不是 Assignment，且该阶段不会向 Edge 创建 Task Projection；只有后续确认分配事件才可以进入 Edge 移动端链路。

实现位置固定为：

```text
internal/core/application/flighttask
    -> Core application use case and HTTP translation
internal/core/adapter/mysql
    -> Core-only GORM repository and transaction mapping
api/core/openapi.yaml
    -> Core arrival contract
```

候选筛选只通过 Application Port 读取 Personnel 事实，Repository 负责参数化 Team/Area、主成员、岗位、能力、状态、启用和计划时间冲突过滤；Handler 不访问 GORM，Edge 不保存完整 Core 业务事实。

## 14. 本阶段非目标

禁止本轮引入 Kafka、RabbitMQ、Kubernetes、Service Mesh、分布式事务、复杂 CQRS、Event Sourcing、ClickHouse、Spark、Flink、真实 AI、硬件业务、真实推送、多租户、多机场或 B3-B5 真实业务流程。

## 15. 验证切片

Architecture Probe 仅用于测试/开发：

```text
Core test event -> Core transaction -> Outbox -> Worker
-> Edge Sync API -> Edge Inbox -> task_projection -> Edge API read

Edge test command -> Command Store -> Core Worker pull -> Core Inbox
-> Core application -> Core transaction -> Outbox -> Edge projection update
```

必须验证重复 Event、重复 Command、Edge 临时不可用、Worker 重启、Redis 不可用。Probe 不作为生产公开业务功能。

## 16. Foundation 最终验收状态

2026-08-13，独立 Compose project `architecture-v2-final` 已完成容器级验证：Core/Edge 双 MySQL 独立 migration、Core/Edge live/ready、双向 SQL/HTTP Probe、重复消息幂等、failed/retry、Worker 重启恢复和 Edge 暂时不可用后的补偿均通过。

因此本架构基线状态为 `ACCEPTED / FROZEN`。本地验收不代表生产 HA、PITR、mTLS、负载均衡或 RPO/RTO 目标已经达成；这些仍按第 12 节作为生产目标保留。
