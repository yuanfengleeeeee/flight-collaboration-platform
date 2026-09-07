# 基础设施任务交接

> 更新时间：2026-09-07
> 状态：Architecture Foundation 已冻结；业务代码已进入自动派发/可靠同步收口，生产化和正式故障门禁仍待完成。

## 当前拓扑

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
2. 在隔离环境应用待部署 Core `000010`–`000013`、Edge `000007`，验证失败重试、fallback、断线、重复消息和恢复。
3. 完成 Gateway、双 Edge、多 Worker lease、网络分区、服务重启和 Redis 不可用场景。
4. 形成 API、MySQL、Worker、Outbox/Inbox、Projection 的 p50/p95/p99、堆积和恢复时间报告。
5. 修复并验证标准镜像获取、CI 缓存和可重复构建流程；临时 workaround 不算正式 Compose 验收。

## 验证纪律

任何 Compose、数据库或集成验证前先运行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

推荐入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

文档修订阶段没有应用 migration、清理数据库或停止已有服务；结束会话时按规则同步已核对的项目修改。不得默认执行 `docker compose down -v`、DROP、TRUNCATE 或 migration down。
