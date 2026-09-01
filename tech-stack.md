# 技术栈基线（Architecture v2）

> 状态：当前 v2 技术基线
> 更新时间：2026-09-01
> 规范来源：memory-bank/tech-stack.md、memory-bank/architecture.md、docs/architecture/architecture-v2.md、实际 Go 代码和 Migration

本文件是根目录索引。完整技术边界以 memory-bank/tech-stack.md 为准；本文件不保留早期候选方案。

## 1. 当前技术栈

| 层 | 选型 | 当前状态 |
| --- | --- | --- |
| 后端语言 | Go 1.25.1 | 已在 go.mod 和 v2 入口中使用 |
| HTTP | Gin 1.12.0 + 标准 net/http | Core、Edge 已使用 |
| ORM/数据库访问 | GORM 1.31.2 + database/sql | Core/Edge Repository 使用 |
| 数据库 | MySQL 8.0 | Core/Edge 独立实例和 schema |
| Migration | 版本化 SQL + cmd/migrate | Core/Edge 目录独立 |
| 缓存 | go-redis/v9 9.21.0，可选 | Redis 不承担可靠一致性 |
| 同步 | HTTP Transport + Outbox + Inbox + Command Store | at-least-once、幂等、重试、失败记录 |
| 配置 | Viper 1.21.0 + YAML + FLIGHT_ 环境覆盖 | 已在 v2 配置中使用 |
| 日志 | Zap 1.28.0 | 结构化日志、request_id、trace_id |
| 认证 | golang-jwt/jwt/v5 5.3.1 | 基础能力存在，正式入口接线待收口 |
| 授权 | Human/Machine Principal + RBAC + AccessScope | BVS2-04 已使用核心授权校验 |
| 测试 | Go unit、HTTP、Memory/显式 SQL integration test | 已建立测试基线 |
| 本地部署 | Docker Compose | Core、Edge、Worker 和双 MySQL |
| 前端 | `frontend/` 工作区目录骨架已创建 | React + TypeScript + Vite + pnpm workspace 为提议方案；源码、依赖和构建配置尚未实现 |

## 2. v2 程序入口

~~~text
cmd/core-api      Core API，Core 唯一业务事实源
cmd/edge-api      Edge API，Projection/Command/Inbox 接入
cmd/worker        Core Outbox 投递和 Edge Command 拉取
cmd/migrate       Core/Edge 独立 SQL Migration
~~~

Core 继续是模块化单体，不拆 flight、task、personnel、event、rule 微服务。

## 2.5 前后端目录边界

前端与 Go 后端在同一仓库中分目录维护。前端独立构建和部署，但不需要把现有 Go 代码移动到新的 `backend/` 目录；`cmd/`、`internal/`、`migrations/` 仍是后端的标准 Go 目录结构。

```text
frontend/
├── apps/
│   ├── admin-web/src/       # 管理端，只调用 Core API
│   └── employee-web/src/    # 员工端，只调用 Edge API
├── packages/
│   ├── contracts/src/
│   ├── api-client/src/
│   ├── auth/src/
│   ├── ui/src/
│   ├── task-domain/src/
│   └── mock/src/
├── e2e/
└── docs/
```

上述目录目前是前端工程骨架，不代表页面、API Client、认证或测试已经实现。浏览器不能直连 Core/Edge MySQL、Redis、Outbox、Inbox 或 Worker。

## 3. 数据库和数据所有权

- Core MySQL 保存航班、人员、岗位、能力、任务模板/实例/候选/分配、状态历史、Audit、Outbox 和业务幂等。
- Edge MySQL 保存移动端最小 Projection、Command、Session、Inbox 和 delivery。
- Core 与 Edge 不共库、不跨库 JOIN；Edge 不直连 Core DB，Core 不直写 Edge DB。
- 服务启动不执行 GORM AutoMigrate。
- 所有数据库结构变更必须通过 migrations/core/mysql 或 migrations/edge/mysql 的版本化 SQL 执行。
- Redis 只处理缓存、限流、短锁、在线状态和临时去重，不能替代 MySQL、Outbox 或 Inbox。

## 4. 代码分层边界

~~~text
internal/core          Core 领域、Application、MySQL Adapter、同步
internal/edge          Edge Application、Projection、Command、Inbox
internal/integration   Core ↔ Edge Transport 和 Worker
internal/platform      配置、MySQL、Redis、日志、健康、安全
internal/shared        Event/Command Envelope、公共 ID 和基础类型
~~~

约束：

- Handler 不直接调用 GORM。
- Service 不依赖 Gin。
- Domain 不依赖 Gin、GORM 或 Redis。
- 所有写操作必须经过后端权限、状态和事务校验。

## 5. 当前未纳入实现基线的能力

以下能力目前只有 Port、桩或设计边界，不代表已经实现：

- Web 管理端和移动端业务工程（目录骨架已创建，源码尚未实现）；
- 完整前端 API Client、认证和页面实现；
- 生产级 JWT Middleware、服务间认证和 mTLS；
- 真实推送、企业 IM、AI 模型、RFID/UWB；
- 完整指标平台、集中式日志和生产 HA/DR；
- 微服务拆分、Kubernetes、Kafka、RabbitMQ、分布式事务。

当前 Phase 2 已完成 BVS2-01 至 BVS2-04，下一项是 BVS2-05 Edge Projection → Employee Command。

## 6. 相关文档

- 技术选型说明：tech-selection-report.md
- 详细技术栈：memory-bank/tech-stack.md
- 架构记录：memory-bank/architecture.md
- 架构规范：docs/architecture/architecture-v2.md
- 前后端交接：docs/frontend-backend-handoff.md
