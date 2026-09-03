# 后端任务交接

> 更新时间：2026-09-03
>
> 状态：BVS2-07、T0-1、T0-2、T0-3、T0-4 和 T0-5 代码已实现；真实个人微信/企业微信 Provider 与管理端企业微信 SSO 服务已加入并默认关闭，生产凭据/域名、员工主数据导入、管理员预置、代表性性能基线和双副本 Docker/网络故障验收仍待推进

## T0-5 当前快照

- Gateway 已通过 `deployments/local/gateway/nginx.conf` 转发 HTTP 和 WebSocket；Compose 提供 `edge-api`/`edge-api-2` 两个相同副本，宿主机只暴露 Gateway 8082，Worker 指向 Gateway。
- Core Outbox 与 Edge Command 使用 `lease_owner`/`lease_expires_at` 持久化租约；Worker ID 和租约时长可由 `FLIGHT_WORKER_ID`、`FLIGHT_SYNC_CLAIM_LEASE_SECONDS` 覆盖，旧 owner 确认会被拒绝。
- Edge realtime ticket 已使用共享 SQL 表并只保存 hash；Redis Pub/Sub 用于跨副本 best-effort `task_changed` fan-out，Redis 失败时 HTTP 快照和 projection revision 仍是恢复路径。
- 已运行：目标包测试、租约测试和 `go test ./...` 均通过（临时 GOCACHE）。未通过/未执行：`scripts/ensure-docker.ps1` 因找不到 `docker.exe` 失败，因此 Compose config、迁移应用、双副本和 Redis 故障注入不能宣称通过。
- 下一步：恢复 Docker CLI 后运行 `scripts/verify.ps1 -Mode all`，按顺序应用 Core 000004、Edge 000005/000006 migration，再做双 Edge、Gateway WebSocket、停一副本、Redis 停止和多 Worker 租约验收；不执行 `down -v`、DROP、TRUNCATE 或 migration down。

## 当前基线

- Core 仍是 Go 模块化单体，入口为 `cmd/core-api`；Edge 入口为 `cmd/edge-api`；异步同步和员工 Command 处理由 `cmd/worker` 完成；Migration 由 `cmd/migrate` 完成。
- Core 是航班、人员、Task、Assignment、状态历史、Audit、业务状态机和 Outbox 的唯一事实源。
- Edge 只保存员工端最小 Projection、Command、Session、Inbox 和 delivery；Core/Edge 使用独立 MySQL 和独立 Migration。
- 同步保留 Outbox、Inbox、Command、at-least-once、幂等、版本收敛、Retry 和 Failed 语义。

## 已完成范围

- Architecture Foundation A1-A10、BVS2-01 至 BVS2-07 已有对应代码、Migration、测试或隔离 Compose/HTTP 验收记录。
- Core 已实现 Arrival 创建语义、Task 列表/详情、Leader Confirm 和 Task Cancel。
- Edge Projection、员工 Accept/Complete Command、Core Worker 拉取、Core Inbox、Outbox 和状态回传链路已实现。
- Core/Edge 正常业务入口已接入 Bearer JWT；Worker Sync Transport 支持可配置 TLS/mTLS；开发 Actor Header 仅是显式非 release 适配器。
- BVS2-07 已接入 Core 凭证和外部身份绑定、一次性绑定 ticket、Edge Session、短期 JWT、Refresh 轮换/重放撤销、当前会话和 Logout；开发模式只接受显式 `mock:<subject>`，配置启用后 Core 可使用服务端个人微信 `code2Session` 和企业微信 `gettoken/getuserinfo` 适配器，平台密钥不下发客户端。
- T0-3 已增加 Edge Notification/Fan-out Port、员工隔离的单实例内存注册表和 `task_changed` 最小提示；Projection 成功提交后才触发，重复 Event 不重复通知，通知失败不回滚 Projection。
- T0-4 已增加 Edge 员工 WebSocket ticket/握手适配器和 `employee-web` 实时客户端；支持 JWT/sid、同源/协议校验、一次性 ticket、`ready`/`ping`/`pong`、空闲关闭、指数退避重连、通知去重和 HTTP 快照恢复。

## 尚未完成或待确认

- 员工主数据导入/管理接口、管理员 `admin_identity` 预置/生命周期和密钥管理；企业微信管理会话签发代码已接入，真实微信/企业微信 Provider 仍需注入凭据、配置域名并完成线上联调。
- 代表性业务负载下的性能基线、API/数据库/Worker/同步链路压测和性能回归门禁。
- 手工 `POST /tasks`、任意 PATCH 和硬删除的业务语义；当前 Task 由 Arrival 创建，使用 GET 读侧和 Confirm/Cancel 生命周期。
- OpenAPI 与实际 JWT、错误 Envelope、字段、版本和状态的进一步最终收敛。

## 后端性能待办

- 建立 API p50/p95/p99、数据库查询耗时、Outbox/Inbox 堆积、Worker 投递延迟、Command 延迟和 Projection lag 指标。
- 审查索引、慢查询、`EXPLAIN`、N+1、Task 分页、连接池和事务长度。
- 优化 Worker 的有界并发、批量拉取、连接复用、租约条件更新、退避和抖动，但不能削弱重复/乱序保护。
- 使用 pprof/trace 分析 CPU、堆、锁竞争、分配和 Goroutine 泄漏；先测量再做局部优化。
- 建立 API、双库同步、Worker 重启、Edge 中断和大数据量压测；详细清单见 [`docs/performance-and-reliability-baseline.md`](../../docs/performance-and-reliability-baseline.md)。

## 真实验证记录

- 已有 `go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all`、Core SQL、双库 SQL/HTTP/Worker Probe、JWT 401 和 Worker/Edge 故障恢复等真实验证记录。
- 正常 `docker compose up --build` 仍可能受 Docker Hub 基础镜像元数据 `EOF` 影响；已有隔离环境使用当前交叉编译二进制完成代码闭环验证，不能把 workaround 写成标准镜像构建已通过。
- 新的后端验证前必须先运行 `scripts/ensure-docker.ps1`；没有本次命令输出时不得重复宣称通过。
- BVS2-07 已在隔离项目 `bvs206-closure` 的 Core/Edge MySQL（宿主端口 3330/3331）上完成 `000003` migration 和当前源码 HTTP 联调；临时员工、会话和 API 进程均已清理，既有 `flight-*` 服务未触碰。验证覆盖绑定、双 Provider exchange、ticket 单次消费、冲突/未映射、refresh 重放撤销、logout、共享密钥和 Staff 停用 fail-closed。

## 下一步

1. 进入 T0-5，增加 Gateway 与两个或以上相同 Edge 副本，确认共享 Edge MySQL 和多 Worker claim/lease。
2. 增加跨副本 best-effort fan-out；Redis 只作为临时提示通道，故障时继续依赖 HTTP 快照恢复。
3. 在产品提供真实 Provider、域名和凭据并导入员工/管理员主数据后，完成个人微信/企业微信生产接线与企业微信管理端 SSO 联调。
4. 按固定数据规模生成 p50/p95/p99 基线，再决定连接池、批量确认、索引或持久 Broker。

## 2026-09-02 T0-3 通知抽象和提交后边界实现完成

- 新增 `internal/edge/application/notification`：`TaskChanged`、`Publisher`、`Sink` 和 `InMemoryFanout`；连接按员工公共 ID 和连接 ID 注册，员工之间不会互收提示，单个连接失败会继续投递其他连接。
- Edge 内部事件入口在 `ApplyEvent` 返回成功后才触发任务通知；重复 Event 不重复 fan-out。通知构造/投递失败只写结构化日志和 `flight_notification_delivery_total`，仍保持事件已应用的 `202`。
- 通知载荷不包含员工 ID，只包含通知 ID、任务 ID、版本、原因和时间；内存 fan-out 不持久化，可靠恢复依赖完整任务快照和员工级 `projection_revision`。
- 已先通过 Docker 前置检查；定向测试 `go test ./internal/edge/application/notification ./internal/edge/application ./internal/platform/observability` 和最终 `scripts/verify.ps1 -Mode all` 均通过，后者包含全量 Go 测试、构建、Compose config 和 `git diff --check`。

## 2026-09-02 T0-4 员工 WebSocket 提示实现完成

- Edge 新增 `POST /api/v1/realtime/ticket` 与 `GET /api/v1/ws`。员工 JWT 通过 ticket 接口换取 30 秒一次性不透明 ticket，浏览器把 ticket 作为 WebSocket 子协议候选值发送；服务端只回显 `flight.realtime.v1`，不接受 URL 凭据。
- `internal/edge/application/realtime` 已完成同源/协议握手、员工 Principal 绑定、`ready`/`ping`/`pong`、读写超时、空闲关闭、连接清理和 WebSocket 指标，并通过 T0-3 Notification Port 投递 `task_changed`。
- `employee-web` 已完成 ticket 获取、WebSocket 建连、心跳回复、指数退避重连和 notification ID 有界去重；连接成功、重连和通知均触发完整任务快照刷新，HTTP 仍是最终读取和恢复路径。
- 已执行 Docker 前置检查。Go 定向测试、前端 `typecheck`、源码 `lint`、`test`（2 个测试文件、5 个测试）和 `build` 均通过；源码 lint 还修正了递归忽略 `**/dist/**`/`**/node_modules/**`，避免并行构建产物被扫描。
- 真实浏览器联调、服务重启/网络分区和多副本连接 fan-out 尚未验收；文档更新后的 `scripts/verify.ps1 -Mode all` 已通过。
## Current snapshot: 2026-09-01 Command contract closure

- Completed the client-owned `command_id` contract for employee Accept/Complete. Replays with the same logical command are idempotent even when delivery trace/timestamp metadata changes; business-content reuse returns `409 command_id_conflict`.
- Added public `GET /api/v1/commands/{commandID}` with employee ownership checks and the client-safe `pending/syncing/confirmed/failed` status mapping. It returns attempts, retry time when applicable, timestamps, and only a generic failure code.
- Updated Edge Memory/MySQL stores, shared command equivalence, Core Inbox comparisons, Edge OpenAPI, and unit/HTTP tests. The next backend-facing handoff is for frontend API-client integration; real WeChat/WeCom provider integration and Task lifecycle contract decisions remain separate.

## T0 实时任务交付决策（2026-09-02）

- 当前 Edge 仍是一个逻辑服务，不按任务类型拆分；横向扩展时使用相同副本、Gateway 和共享 Edge MySQL。
- 员工任务继续以 `GET /api/v1/tasks` 拉取为最终读取和恢复路径。前台 WebSocket `task_changed` 已作为提示接入，后台平台通知仍待实施，提示只能触发再次拉取。
- Core Outbox → Worker → Edge Inbox/Projection 的可靠链路不变；Projection 提交后才允许尽力而为通知，Redis fan-out 只能是临时提示通道。
- 后端下一项 T0 顺序记录在 [`docs/adr/ADR-009-employee-task-realtime-delivery.md`](../../docs/adr/ADR-009-employee-task-realtime-delivery.md)：T0-1 至 T0-4 已完成，下一步是多副本 Gateway/Worker、平台通知和故障容量门禁。
- T0-3 的 Notification Port 和 T0-4 的单实例 WebSocket 已实现；Gateway、多副本、真实微信/企业微信通知以及真实网络故障场景仍未实现或验收。

## 2026-09-02 T0-1 可观测性实现完成

- Core API、Edge API 和 Worker 已提供内部 `/metrics` 端点；指标覆盖 HTTP 请求/延迟、数据库连接池压力、Core Outbox、Edge Command/Inbox backlog 与 oldest age、Projection lag，以及 Worker 投递/处理/确认延迟直方图。
- Core/Edge 队列指标从各自持久化 Store 读取；Worker 负责 Core Outbox backlog 和同步操作指标。实现未改变 Outbox、Inbox、Command、幂等、版本收敛、Retry 或 Failed 语义。
- Docker 前置检查、受影响 Go 测试、`go build ./...`、运行时指标 smoke 和 `scripts/verify.ps1 -Mode all` 均已通过。运行时 smoke 使用无数据库写入的临时进程，确认三个服务的指标端点和稳定标签可抓取。
- 代表性业务负载下的 p50/p95/p99 数值报告仍未生成；在固定数据规模、批量大小、轮询间隔和连接池配置后再执行，不把空跑结果作为性能结论。

### 下一步

1. 进入 T0-3，增加提交后 Notification/Fan-out Port，保持通知失败不回滚 Projection。
2. 按固定数据规模、批量大小、轮询间隔和连接池生成 p50/p95/p99 报告，再决定批量确认、索引、Gateway 或多 Worker。
3. 在产品提供真实 Provider、域名和凭据后，替换本地 Mock Provider 并完成生产接线。

## 2026-09-02 T0-2 拉取恢复契约实现完成

- `GET /api/v1/tasks` 保留员工 JWT Principal 过滤的完整快照，新增 `snapshot_at`、`sync_mode`、员工级 `projection_revision`、Projection lag 状态、`next_cursor` 和 `reset_required`；OpenAPI 与前端 contracts 已同步。
- Edge Memory/SQL Store 增加员工 projection cursor；000004 migration 创建 `employee_projection_cursor`，Projection 新增/高版本更新推进 revision，同版本重放和过期版本不会推进，任务转派同时推进两侧员工。
- 空结果、取消/完成、刷新、重连、离线恢复和 Command 状态恢复语义已写入 ADR；当前不启用增量 API，`next_cursor=null`、`reset_required=false`。
- 本轮已执行 Docker 前置检查；受影响测试和最终 `scripts/verify.ps1 -Mode all` 均通过，包含 `go test ./...`、`go build ./...`、Compose config 和 `git diff --check`。000004 migration 未自动应用，未执行破坏性数据库操作。
## Current session handoff (2026-09-03)

- Core management SSO/session code, direct Enterprise WeChat/development provider adapters, Core migration `000005_admin_sso`, personnel/assignment read APIs, OpenAPI and frontend client contracts, and ADR-011 are present in the workspace.
- Authorization remains Core-owned: direct Enterprise WeChat authentication must resolve to a pre-provisioned active `admin_identity`; current role and scope are restored from the Core session. Production credentials, callback domains, employee import, and admin provisioning remain pending. OIDC is not required for this deployment.
- Docker Engine preflight succeeded in this session. The new migration has not been applied, and no destructive database or volume operation was run.
- Full Go test/build verification was not rerun after the final handoff-only edits; the next session must run `scripts/ensure-docker.ps1` first, then the complete checks and isolated management HTTP/SQL verification.
## Verification update (2026-09-03)

- Docker preflight succeeded with the project-local Docker CLI override; the CLI resolved to `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`.
- The first full test attempt was blocked by restricted access to the default Go build cache. Re-running with ignored project-local `.tmp\gocache-handoff` succeeded: `go test ./...` passed and `go build ./...` exited 0.
- `scripts/verify.ps1 -Mode all` passed after that rerun, covering Docker preflight, full Go tests, full Go build, Compose config, and `git diff --check`.
- This does not constitute live migration, production OIDC, or dual-replica failure acceptance: Core `000005_admin_sso` remains unapplied and no Compose startup or destructive database operation was performed.
