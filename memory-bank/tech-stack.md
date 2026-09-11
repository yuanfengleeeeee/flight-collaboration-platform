# Architecture v2.0 技术栈

> 架构状态：`ACCEPTED / FROZEN`
> 技术实现状态：2026-09-07

| 领域 | 选择 | 边界 |
| --- | --- | --- |
| 运行时 | Go | `core-api`、`edge-api`、`worker`、`migrate` 共用单仓库模块；Core 是模块化单体 |
| HTTP | Gin + 标准 `net/http` | Core、Edge、集成路由分离；Service 不依赖 Gin |
| 持久化 | MySQL + GORM/`database/sql` | Core/Edge 独立实例；事实、Outbox/Inbox、Command 和 Projection 落 MySQL |
| 缓存/临时状态 | Redis，可选 | cache、限流、短锁、在线状态和 best-effort fan-out；不承担可靠消息 |
| 配置 | Viper + YAML + `FLIGHT_` 环境覆盖 | 真实凭据只来自环境或 Secret 管理，不提交仓库 |
| 日志/指标 | Zap + 内部 metrics | 结构化 request/trace 字段；不记录密码、完整 token 或密钥 |
| 认证 | JWT + Human/Machine Principal | Core/Edge audience 分离；真实平台身份由 Provider/SSO 配置提供 |
| 授权 | RBAC + 结构化 `AccessScope` | `admin/manager/leader/supervisor/staff`；Scope 为 global/area/team/assigned/self |
| 迁移 | 版本化 SQL + 独立 migrate 命令 | `migrations/core/mysql` 与 `migrations/edge/mysql` 分开；禁止启动 AutoMigrate |
| 可靠同步 | Transactional Outbox + Inbox + Command Store | at-least-once、幂等、版本收敛、Retry/Failed；不依赖 Redis |
| 前端 | React + TypeScript + Vite + pnpm workspace | `admin-web` 调 Core；三个员工客户端调 Edge |
| 原生小程序 | 个人微信/企业微信独立壳 | 独立登录、客户端标识和发布配置，共享 Staff/Edge 业务合同 |
| 本地部署 | Docker Compose | Core API、Gateway/Edge、Worker、Core MySQL、Edge MySQL，Redis 可选 |

## 外部 Port

- `internal/integration/flight.Provider`：航班外部源适配；标准化记录先写 `flight_source_inbox`，再由 Worker 应用。
- `NotificationSender`/`PushSender`：平台通知适配边界；真实微信/企业微信通知待配置。
- Identity Provider/SSO：员工 Provider 与管理端 SSO；真实凭据、域名和管理员预置待部署。
- Transport/mTLS、Metrics/Tracing：保留可配置实现；本地配置与生产安全参数分离。

## 技术边界

- Core 拥有航班、人员、岗位、能力、任务、Assignment、状态历史、审计和业务状态机；Edge 只拥有移动端 Projection、Session、Command、Inbox 和通知数据。
- 航班、任务和人员主数据不在前端或 Edge 复制为可写事实；前端只能通过 API 访问。
- 自动任务派发、三层握手、变更审批和异常处理属于 Core 业务服务，不能藏在 Handler 或浏览器状态中。
- 员工 `received/start/complete` 必须通过 Edge 持久化 Command；`received` 只表示收件，不代表同意或拒绝。
- 管理集合、Edge 历史和通知使用服务端分页；员工任务完整快照只用于员工级恢复。

## 性能与可靠性

先测量 API p95/p99、数据库查询、Worker 延迟、Outbox/Inbox 堆积和 Projection lag，再做连接池、索引、批处理或扩容优化。任何优化不得弱化 Outbox、Inbox、Command、幂等和版本保护。

前端动画只服务反馈和状态变化，默认使用短时 `transform`/`opacity`，支持 reduced motion；禁止持续高成本 WebGL/Canvas、全量列表 stagger、大面积 blur 和大型动效资源。

详细基线见 [`docs/performance-and-reliability-baseline.md`](../docs/performance-and-reliability-baseline.md)。当前指标仍是待验证目标，不代表已经完成生产压测。

## 明确不引入

当前不引入 Core 微服务拆分、Kafka/RabbitMQ、Kubernetes、Service Mesh、分布式事务、ClickHouse、Spark/Flink、真实 AI/硬件业务或多租户/多机场模型。
