# 航空保障智能协同平台

## Architecture v2.0 Foundation

本仓库当前基线为：单仓库、内网 Core 模块化单体、云端 Edge 接入服务、Core/Edge 独立 MySQL、Core 唯一事实源、Edge Projection、Outbox/Inbox 可靠双向同步。Architecture v2.0 Foundation 已于 2026-08-13 完成容器级验收，状态为 `ACCEPTED / FROZEN`。

项目现已进入 `Phase 2 — Business Implementation`：BVS2-01 业务设计已 `FROZEN`，BVS2-02 Core 数据与版本化 Migration、BVS2-03 `Flight → Task → Candidate`、BVS2-04 `Leader Confirm` 已完成，BVS2-05 `Edge Projection → Employee Command` 正在实施。详见 [Business Slice v2](memory-bank/business-slice-v2-flight-task.md)：Flight → Task → Personnel → Leader Confirm → Edge → Employee → Complete。旧 B3 现场不直接恢复；业务必须适应已冻结架构。规范见 [Architecture v2.0](docs/architecture/architecture-v2.md) 与 [ADR](docs/adr/)。

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
当前最新进度：BVS2-05 已完成代码与隔离数据库验证，BVS2-06 已完成首个 Task Cancel 功能步骤（Core 管理取消、事务历史/Audit/Outbox/幂等和 SQL 验证）；完整 Compose 闭环、正式认证和 Task CRUD 仍在后续实施。
## BVS2-06 current delivery

- Compose full-chain and physical Worker/Edge recovery acceptance has passed on isolated project `bvs206-closure`.
- Core and Edge business routes use Bearer JWT in normal configuration; TLS/mTLS is configurable for API listeners and Worker sync transport. Local Compose keeps TLS disabled and explicitly enables development actor-header compatibility.
- Core Task read CRUD is available at `GET /api/v1/tasks` and `GET /api/v1/tasks/{taskPublicID}` with RBAC/scope filtering. Task creation remains the transactional Flight Arrival flow; Confirm and Cancel are the lifecycle mutations. Manual generic create, arbitrary PATCH, and hard delete await a frozen product contract.
- See [Task CRUD contract](docs/task-crud-contract-v2.md), [Core OpenAPI](api/core/openapi.yaml), and [handoff](HANDOFF.md).
