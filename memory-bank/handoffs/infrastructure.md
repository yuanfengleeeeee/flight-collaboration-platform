# 基础设施任务交接

> 更新时间：2026-09-08
> 状态：Architecture Foundation 已冻结；`flight-dev` 资源已完成收口，生产化和正式故障门禁仍待完成。

## 当前拓扑

- 本地开发基线另提供固定 `flight-dev`/`flight-test` Compose 项目：前后端和 Gateway 集中在一个 `app` 容器，Core/Edge MySQL 与 Edge Redis 保持独立并挂载 named Volume。
- `scripts/dangerous-test-up.ps1` 为危险/版本测试创建新的 `flight-danger-*` 项目和全新 named Volume；日常/普通测试不会因为每次执行而新建项目。
- 本地 Compose 包含 Core API、Edge API、Worker、Core MySQL、Edge MySQL；Redis 为可选依赖。
- Core/Edge MySQL 完全独立，分别使用 `migrations/core/mysql` 和 `migrations/edge/mysql`。
- Worker 只连接 Core DB，通过 Edge HTTP Transport 投递和拉取，不直连 Edge DB。
- Gateway/多副本能力属于后续正式环境验收项；Redis 只作缓存、短锁、在线状态和 best-effort fan-out。

## 当前业务基础设施要求

- 航班源先写 Core `flight_source_inbox`，Worker 异步应用；Provider 健康状态为 `fresh/stale/fallback/failed`。
- Core Outbox → Worker → Edge Inbox/Projection 是可靠链路，必须保留幂等、重试、失败和版本收敛。
- 员工实时 socket、管理 SSE 和 Redis fan-out 都只是提示；断线通过服务端分页/完整快照恢复。
- migration 文件存在不代表已应用；使用 `go run ./cmd/migrate -target core|edge -command status` 查看真实状态。

## 待完成基础设施门禁

1. 按真实航班接口资料完成 Provider 凭据/字段配置、定时同步、告警渠道和对账。
2. 完成 Core/Edge API、Gateway、Worker 运行时隔离验收，验证失败重试、fallback、断线、重复消息和恢复；Core `000010`–`000014`、Edge `000007` 已在迁移专用隔离项目应用并完成 schema 复核。
3. 完成 Gateway、双 Edge、多 Worker lease、网络分区、服务重启和 Redis 不可用场景。
4. 形成 API、MySQL、Worker、Outbox/Inbox、Projection 的 p50/p95/p99、堆积和恢复时间报告。
5. 修复并验证标准镜像获取、CI 缓存和可重复构建流程；临时 workaround 不算正式 Compose 验收。

## 2026-09-08 flight-dev 资源收口

- `flight-dev` 当前保留 4 个容器：`app`、Core MySQL、Edge MySQL、Edge Redis；容器当前均为停止状态，数据 Volume 未删除。
- 当前仅保留 3 个 Volume：`flight-dev_core_mysql_data`、`flight-dev_edge_mysql_data`、`flight-dev_edge_redis_data`。此前其他项目的 26 个 Volume 已按明确授权删除，旧测试/验收数据库数据不可恢复。
- `docker system df` 复核：3 个镜像约 `1.456GB`、3 个 Volume 约 `430.3MB`、BuildKit 缓存 `0B`；容器可写层约 `122.9kB`。
- 重新启动固定开发环境使用 `scripts/dev-up.ps1`；危险/版本测试仍使用 `scripts/dangerous-test-up.ps1` 创建隔离项目和新 Volume。

## 验证纪律

任何 Compose、数据库或集成验证前先运行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

推荐入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

此前文档修订阶段没有应用 migration、清理数据库或停止已有服务；后续迁移专项和 Docker 资源清理均按独立项目、精确名称执行。不得默认执行 `docker compose down -v`、DROP、TRUNCATE 或 migration down。

## 2026-09-08 Docker 前置修复

- 宿主 Docker Desktop 当前为 running，Client/Server `29.6.2`，Context 为 `desktop-linux`，Compose 为 `v5.3.1`，WSL2 `docker-desktop` 为 Running。
- Docker Desktop `AutoStart` 保持关闭；项目验证需要 Engine 时由 `scripts/ensure-docker.ps1` 按需启动，普通 Windows 登录不会启动 Docker Desktop。
- `scripts/ensure-docker.ps1` 已改为优先使用 `docker version` 和 `docker desktop start --detach`，并增加注册表安装路径发现与分层诊断。
- 修改后 `scripts/verify.ps1 -Mode all -DockerTimeoutSeconds 30` 实际通过，退出码为 `0`。
- 涉及 Compose/数据库验证时要使用可访问 Docker Engine 的执行上下文；未执行迁移回滚或 `down -v`。
- 已新增项目级 `.codex/rules/docker.rules`，仅放行 `scripts/ensure-docker.ps1` 和 `scripts/verify.ps1` 的 PowerShell 入口在宿主权限上下文执行；Docker Desktop 仍保持按需启动。规则加载需重启 Codex。

## 2026-09-08 flight-dev 启动尝试

- Docker 前置检查通过；`scripts/dev-up.ps1` 的 `flight-dev-app` 构建因 Docker Hub 拉取 `node:22-bookworm-slim`、`nginx:1.27-alpine`、`golang:1.25` 返回 EOF 失败。
- 已启动固定项目的 Core MySQL、Edge MySQL、Edge Redis，三者均 healthy，并创建 `flight-dev_core_mysql_data`、`flight-dev_edge_mysql_data`、`flight-dev_edge_redis_data` named Volume。
- `app` 尚未启动，未执行 migration/seed；未清理其他项目或 Volume。
