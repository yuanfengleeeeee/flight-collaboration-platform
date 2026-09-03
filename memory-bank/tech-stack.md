# Architecture v2.0 技术栈

> 状态：Accepted / Frozen（架构基线）；技术实现状态更新于 2026-09-01

| 层 | 选择 | 边界 |
|---|---|---|
| 运行时 | Go 1.25.1 | `core-api`、`edge-api`、`worker`、`migrate` 共用单仓库模块；Core 仍是模块化单体 |
| HTTP | Gin 1.12.0 + 标准 `net/http` | Core、Edge、Sync 路由分离；业务 Service 不依赖 Gin |
| 持久化 | MySQL 8.0 + GORM 1.31.2/`database/sql` | Core/Edge 两个独立实例；事实数据和可靠同步记录只落 MySQL |
| Redis | `go-redis/v9` 9.21.0，可选 | cache、限流、短锁、在线状态和临时去重；不承担事实或可靠同步 |
| 配置 | Viper + YAML + `FLIGHT_` 环境覆盖 | 示例配置可提交；真实凭据只来自环境/未来 Secret Manager |
| 日志 | Zap 1.28.0 | 结构化字段，包含 request_id/trace_id；禁止敏感凭据 |
| 认证 | `golang-jwt/jwt/v5` 5.3.1 | JWT 只含身份/会话声明；Human/Machine Principal 分离；正式入口接线仍需收口 |
| 授权 | RBAC + 结构化 AccessScope | 角色 admin/manager/leader/staff；Scope 为 global/area/team/assigned/self |
| 迁移 | 版本化 SQL + 独立 migrate 命令 | `migrations/core/mysql` 与 `migrations/edge/mysql` 分开；禁止启动 AutoMigrate |
| 同步 | Transactional Outbox + Inbox + Command Store | at-least-once、幂等、重试、失败记录；暂不引入 Kafka/RabbitMQ |
| 测试 | Go unit/HTTP/integration test | Probe 默认可在内存 fake 运行；数据库测试显式配置，不清空用户数据 |
| 部署 | Docker Compose 本地 | `core-api`、`gateway`、`edge-api`、`edge-api-2`、`worker`、`core-mysql`、`edge-mysql`；Edge Redis 用于 best-effort fan-out，Core Redis 仍可选 |
| 前端 | `frontend/` 工作区目录骨架已创建 | `admin-web`/`employee-miniapp`/`employee-web` 及共享包目录已落地；React + TypeScript + Vite + pnpm workspace 为 `PROPOSED`，前端只能通过 API 访问数据 |
| 性能原则 | 先测量再优化；可靠同步优先于视觉效果 | 前端动效受性能预算约束；后端不削弱 Outbox/Inbox/Command、幂等和版本收敛 |

详细的前端动画限制、性能预算和后端性能待办见 `docs/performance-and-reliability-baseline.md`。当前性能指标均为待验证目标，不代表已经完成压测或生产验收。

## 未来 Port（本轮禁用）

`NotificationSender`、`PushSender`、`FlightDataSource`、`DeviceObservationPort`、`Predictor`、SSO/IdentityProvider、Transport/mTLS、Metrics/Tracing。当前使用 Noop/InApp/Disabled Adapter，不引入真实外部账号、硬件或模型。

## 非选择

本轮不引入微服务拆分、Kubernetes、Service Mesh、Kafka、RabbitMQ、分布式事务、ClickHouse、Spark、Flink、Model Registry、Feature Store、GPU Service 或完整监控平台。
