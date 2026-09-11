# 航空客运地面代理协同平台

## Architecture v2.0 Foundation

本仓库当前基线为：单仓库、内网 Core 模块化单体、云端 Edge 接入服务、Core/Edge 独立 MySQL、Core 唯一事实源、Edge Projection、Outbox/Inbox 可靠双向同步。Architecture v2.0 Foundation 已于 2026-08-13 完成容器级验收，状态为 `ACCEPTED / FROZEN`。

项目现已进入 `Phase 2 — Business Implementation` 收口阶段：BVS2-01 至 BVS2-07、T0-1 至 T0-5 的当前代码切片已落地，后续重点是生产配置、故障/性能门禁和正式验收。当前业务流程为“外部航班同步 → `pending_dispatch` 自动派发 → Core/Edge 三层握手 → 员工 `received` 收件 → `start` 执行 → `complete` 完成”；员工没有拒绝任务的业务动作，冲突和异常必须提交变更申请并由值班经理/管理员审批。详见 [当前业务流程基线](docs/business-process-v2.md) 和 [Business Slice v2](memory-bank/business-slice-v2-flight-task.md)。旧 B3 现场不直接恢复；业务必须适应已冻结架构。规范见 [Architecture v2.0](docs/architecture/architecture-v2.md) 与 [ADR](docs/adr/)。

## 入口

```text
cmd/core-api   内网 Core API
cmd/edge-api   云端 Edge API
cmd/worker     Core Outbox 投递与 Edge Command 拉取
cmd/migrate    指定 Core/Edge 数据库迁移
```

## 常用检查

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

统一验证入口会先检测 Docker Engine；如果 Docker Desktop 未运行，会自动启动并等待就绪，再执行 Go 测试、构建、Compose 配置和差异检查。也可以按需执行 `-Mode test`、`-Mode build`、`-Mode compose-config` 或 `-Mode compose-ps`。

Docker 可用时：

```powershell
docker compose -f deployments/local/docker-compose.yml config
docker compose -f deployments/local/docker-compose.yml up -d
```

Core/Edge 健康检查均为 `/health/live` 和 `/health/ready`。Core/Edge migration 分别位于 `migrations/core/mysql` 和 `migrations/edge/mysql`；服务启动不执行 GORM `AutoMigrate`。

不要执行 `docker compose down -v`、`TRUNCATE`、`DROP DATABASE` 或未经确认的 destructive migration；本机已有 MySQL 时使用可配置宿主机端口，不停止已有服务。
当前最新进度：任务自动派发、收件回执、超时重派、任务变更申请/经理审批、航班源预同步与 fallback、岗位/能力字典 CRUD、管理端角色 Scope 和服务端分页均已写入当前代码/契约；真实航班 Provider、真实平台凭据/域名、生产告警、多副本故障门禁、性能基线和正式主数据仍待验收。任务不是通用手工 CRUD：航班事实由外部源进入，任务由到达事件和启用模板生成。
## Current business delivery

- 自动派发、三层握手、收件超时重派、任务变更审批、航班源预同步/fallback、岗位/能力字典 CRUD、管理 Scope 和服务端分页已进入当前代码/契约。
- Core 与 Edge 业务路由在正常配置下使用 Bearer JWT；TLS/mTLS 可配置，Local Compose 保持开发环境的显式 actor-header 兼容开关。
- Core 任务查询使用 `GET /api/v1/tasks` 和 `GET /api/v1/tasks/{taskPublicID}`，按 RBAC/Scope 过滤；任务只能由外部航班到达 + 启用模板生成，不提供通用手工创建、任意 PATCH 或硬删除。
- 员工任务操作为 `received`、`start`、`complete`；`received` 仅收件，不是同意或拒绝。
- See [Task CRUD contract](docs/task-crud-contract-v2.md), [Core OpenAPI](api/core/openapi.yaml), and [handoff](HANDOFF.md).
