# Architecture v2.0 Foundation 实施计划

> A1-A10 已于 2026-08-13 完成并验收；Architecture Foundation 状态为 Accepted / Frozen。本计划不包含真实 B3-B5 业务，后续业务实施需另行建立计划。

## A1 Repository Audit

核对 git root、branch、remote、status、diff、untracked、现有目录、migration、测试、配置和运行依赖；建立 KEEP/REFACTOR/REWRITE/REMOVE 清单，保留已有修改。

验证：审计结果与实际命令输出一致，旧 HANDOFF 的矛盾被记录。

## A2 Documentation Freeze

建立 `docs/architecture/architecture-v2.md`、8 个 Accepted ADR，并同步设计、技术栈、实施计划、进度、架构记录、AGENTS、HANDOFF、README；明确单机场、Core/Edge 数据所有权和 B3 暂停。

验证：规范文档之间没有共享数据库、AutoMigrate、多租户或微服务冲突；旧文档已明确过时/非规范。

## A3 Repository Restructure

建立 `cmd/core-api`、`cmd/edge-api`、`cmd/worker`、`internal/core`、`internal/edge`、`internal/integration`、`internal/platform`、`internal/shared`、`api/{core,edge,sync,schemas}` 和新部署目录。旧 B1/B2/B3 现场不覆盖，暂标为 legacy/paused。

验证：新入口的 import 方向清晰；Core/Edge/Integration 不共享业务表；`go list ./...` 可解析。

## A4 Dual Database Foundation

为 Core/Edge 分别建立 MySQL 配置、连接池、schema migration runner 和 `000001_*_foundation` up/down SQL；Compose 用两个容器隔离，Redis 只设 optional profile。

验证：Core/Edge migration 可分别在空库 status/up；服务启动不包含 `AutoMigrate`；不执行破坏性 down 或清理用户 volume。

## A5 Core / Edge Application

实现 Core API、Edge API、Worker 独立生命周期、配置、structured logging、request_id/trace_id、live/ready、优雅关闭和必要依赖检查。Core/Edge API 不依赖对方数据库。

验证：两个 API 可独立启动；live 不依赖数据库；ready 只把必要 MySQL 作为硬依赖，Redis 故障不让核心一致性失效。

## A6 Sync Infrastructure

实现 Event/Command Envelope、Outbox、Inbox、Command Store、幂等键、pending/retry/failed 状态、退避和错误记录；封装 Core→Edge Transport 与 Core 主动拉取 Edge Command。

验证：重复 Event/Command 只产生一次最终效果；Outbox 在 Worker 退出后可继续；Edge 不可用不回滚 Core 事务；无 Redis 仍可同步。

## A7 IAM Foundation

实现 Human/Machine Principal、四角色、`resource:action` Permission、结构化 AccessScope、IdentityProvider/Authenticator/Authorizer Port 和 Audit record。JWT 不保存完整权限列表。

验证：未认证/无权限/越权路径可测试；Scope 通过参数化过滤对象传递；设备身份不能伪装普通 staff。

## A8 Architecture Probe

建立 test-only/dev-only Core test event→Outbox→Worker→Edge Inbox→Projection→Edge API，以及 Edge command→Command Store→Core pull→Core Inbox→Core transaction→Outbox→Projection 的双向验证。

验证：Probe 不注册为生产公开业务接口；端到端测试可重复运行并检查公共 ID、版本、trace/correlation。

## A9 Failure Tests

覆盖重复 Event、重复 Command、Edge 暂不可用、重试、Worker 中途退出并重启、Redis 不可用、Projection 延迟和 failed 状态。

验证：每种故障有自动测试或可重复命令；文档只记录实际运行结果。

## A10 Final Verification（已完成）

运行 `gofmt`、`go test ./...`、`go build ./...`、`git diff --check`、`git status`；Docker 可用时运行 Compose config、双库 migration、Core/Edge health 和 Probe。不可用时记录真实错误。

验证：更新 `memory-bank/progress.md`、`memory-bank/architecture.md`、`HANDOFF.md`，不宣称未运行的检查通过。

实际结果：Docker Engine、Compose 配置、Core/Edge 双 MySQL migration、四个健康检查、双向容器 Probe、重复消息幂等、Worker 重启恢复、Edge 不可用重试恢复，以及 `go test ./...`、`go build ./...`、`git diff --check` 均已实际通过。v2 Foundation 现已冻结，不继续增加架构功能。

## Phase 2 Business Implementation

> 状态：BVS2-01 FROZEN / BVS2-02 COMPLETED / BVS2-03 COMPLETED / BVS2-04 COMPLETED / BVS2-05 COMPLETED / BVS2-06 COMPLETED（当前冻结范围）
> 当前后端边界：BVS2-06 冻结范围已完成；手工 Task 创建、任意 PATCH、硬删除和完整生产身份体系仍需单独冻结契约。
> 业务实现必须适应 Architecture v2.0，不得为业务便利修改已冻结的 Core/Edge、双数据库、Outbox/Inbox、Principal、RBAC/Scope 或 Migration 边界。

业务切片的唯一当前入口为 [`memory-bank/business-slice-v2-flight-task.md`](business-slice-v2-flight-task.md)。旧 B3 业务切片材料、旧 B3 目录和旧业务路由均为 legacy/paused，不得直接恢复。

Phase 2 的执行顺序为：

1. `BVS2-01` 业务设计冻结：`FROZEN`。六组设计交付物和 BVS2-AT-01 至 BVS2-AT-30 已冻结；本步骤只改文档，不写业务代码。
2. `BVS2-02` Core 数据与迁移：`COMPLETED`。已建立已冻结业务事实、状态历史和业务幂等结果记录；Core 容器数据库已执行 `000002_core_business_slice_v2`，Edge 仍只有独立 Foundation migration。
3. `BVS2-03` Flight → Task → Candidate：`COMPLETED`。已实现 Flight 到达 Application Use Case、任务生成幂等、模板快照、Candidate 确定性筛选、同事务 Audit/Outbox 和 Core API 入口；内存单元测试与隔离 Docker Core MySQL 集成测试均已覆盖。
4. `BVS2-04` Leader Confirm：`COMPLETED`。已实现 Human Principal + Area/Team Scope、任务版本与候选复核、Personnel reserved、Assignment、状态历史、Audit、`task.assigned.v1` Outbox 和 `confirmation_id` 业务幂等；内存与隔离 Core MySQL 验收均已覆盖。
5. `BVS2-05` Edge Projection → Employee Command：`COMPLETED`。代码、Edge migration、隔离数据库验证和员工命令状态机均已完成。
6. `BVS2-06` 完整闭环验收：`COMPLETED`（当前冻结范围）。已完成 Compose 双向闭环、JWT/mTLS 入口、Task 查询/生命周期操作以及重复、越权、非法状态和恢复路径验证。

当前 `BVS2-01` 至 `BVS2-06` 均已完成当前冻结范围；后续转入前端 F0 设计冻结和产品化待办收敛。

## BVS2-04 Leader Confirm（2026-08-31）

- 已实现 `ConfirmationService` 与独立 `ConfirmationRepository` Port；确认命令不扩大 BVS2-03 Arrival 的既有接口，Core Service 不依赖 Gin/GORM。
- 成功路径在一个 Core MySQL transaction 内完成：锁定并校验 Task `awaiting_confirmation + expected_task_version`，复核 Candidate 与当前 Personnel 的 Team/Area、岗位、能力、启用、`idle` 和计划时间冲突，调用保留状态变更，创建 `confirmed` Assignment，选中一个 Candidate、拒绝其他候选，将 Task 变为 `assigned`，写入 Task/Assignment/Personnel History、Audit 与 `task.assigned.v1` Outbox。
- `confirmation_id` 是业务幂等键；同 ID 同内容重放原结果且不重复副作用，不同内容返回 `confirmation_id_conflict`。任务已分配、版本过期、权限范围不符和候选人失效均有稳定结果码；候选人失效会在同一事务内标记 `invalidated`，不会生成半成品 Assignment。
- Core API 新增 `POST /api/v1/tasks/{taskPublicID}/confirm`。当前 v2 内部入口支持由认证中间件注入 Principal，也保留 `X-Actor-*` 测试适配；正式 JWT 中间件接线仍属于后续入口收敛，不改变 Application 层 Human Principal 校验。
- 验证结果：`go test ./...`、`go build ./...`、`git diff --check` 通过；临时内存卷 Core MySQL 完成 Core migration 后，`FLIGHT_RUN_BVS2_DB_TEST=1 go test ./internal/core/adapter/mysql -run TestFlightTaskSQLAgainstDocker -count=1 -v` 通过。原 `local_core_mysql_data` 数据卷检测为 InnoDB 损坏，未执行删除或重置；集成验证使用可清理的内存临时容器。

## BVS2-05 Edge Projection → Employee Command（2026-09-01，代码与隔离数据库验证已完成）

- Edge `TaskProjection` 新增 `assignment_public_id` 与 `business_status` 语义；`task.assigned.v1`、`task.accepted.v1`、`task.completed.v1`、`task.cancelled.v1` 均按 Core 最小快照投影。
- Edge Projection 按 `sync_version` 收敛：旧版本忽略、同版本同内容幂等、同版本不同内容返回 `projection_version_conflict`；新增 Edge `000002_edge_business_slice_v2` migration 保存 Assignment ID。
- Edge 新增 `POST /api/v1/tasks/{taskPublicID}/accept` 与 `/complete`，只从 `X-Employee-Public-ID` 生成命令 Actor，先持久化 Command 再异步交给 Core Worker。
- Core 新增员工命令处理：校验本人 Assignment、`task:accept`/`task:complete`、状态和 Projection 版本；Accept/Complete 在同一 Core MySQL transaction 内更新 Task、Assignment、Personnel，写三类 History、Audit 和对应 Outbox Event。
- Worker 新增可插拔 CommandProcessor；生产 Core MySQL 使用业务 Repository 的同事务 Core Inbox，Foundation Probe 仍保留。业务拒绝标记为终态失败，不进入无意义重试。
- Core/Edge 的 Command 幂等比较使用 JSON 语义等价判断，兼容 MySQL JSON 列自动规范化对象键顺序；同 command ID 的不同业务内容仍返回冲突。
- 已实际通过定向 Go 测试：`go test ./internal/edge/... ./internal/core/application/... ./internal/core/adapter/mysql ./internal/core/module/iam ./internal/integration/sync ./cmd/worker/...`。
- 已在隔离临时 Edge MySQL 应用两个 migration，并通过 `FLIGHT_RUN_BVS2_EDGE_DB_TEST=1` Projection 版本收敛/冲突 SQL 测试；Edge `000001`、`000002` 均为 `applied`。
- 已在隔离临时 Core MySQL 应用 `000001`、`000002` 后通过 `FLIGHT_RUN_BVS2_DB_TEST=1 go test ./internal/core/adapter/mysql -run TestFlightTaskSQLAgainstDocker -count=1 -v`，覆盖 Accept→Complete 原子状态转换、Core Inbox 幂等、三类 History、Audit 与 Outbox。
- `powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all` 已通过：Docker 前置、`go test ./...`、`go build ./...`、Compose config 和 `git diff --check` 均通过。临时容器已停止并清理；原有数据卷未删除/重置。

## Cross-cutting Project Memory and Docker Preflight（已完成，2026-08-31）

- 新增 `memory-bank/project-memory.md`，保存跨会话长期约束；`progress.md` 和 `HANDOFF.md` 继续保存执行进度和交接状态。
- 新增 `scripts/ensure-docker.ps1`，在功能、集成、数据库或 Compose 验证前检查 Docker Engine；未就绪时启动 Docker Desktop 并等待就绪。
- 新增 `scripts/verify.ps1`，统一执行 Docker 前置检查，以及 Go 测试、构建、Compose 配置和 `git diff --check` 等分项验证。
- 该机制不自动启动项目容器、不执行 Migration、不删除 Volume、不清空数据库；这些动作仍需按项目规则显式执行。
## BVS2-06 Task Cancel（2026-09-01，首个闭环已完成）

- 已实现 Core 管理取消用例：POST /api/v1/tasks/{taskPublicID}/cancel，输入 cancellation_id、expected_task_version、reason。
- Human admin/manager/leader 受 task:cancel 与 Task Team/Area Scope 约束；leader 只能在员工 Accept 前取消，staff 和 machine 不可执行。
- awaiting_confirmation 会在同一 Core transaction 内使全部 proposed Candidate 失效；assigned/in_progress 会同步取消 Assignment 并释放 Personnel；completed/cancelled 不产生新的业务状态副作用。
- 事务同时写 Task/Assignment/Personnel History、Audit、业务幂等记录和 task.cancelled.v1 Outbox；同 cancellation_id 重放返回原结果，不同内容返回 cancellation_id_conflict。长取消 ID 的事件 CorrelationID 使用稳定派生 UUID 以兼容现有 CHAR(36) schema。
- 已通过取消服务单元测试、Core/HTTP 相关包回归测试，以及独立 Docker Core MySQL migration 后的 TestTaskCancellationSQLAgainstDocker。

当前正式任务仍为 BVS2-06：还需完成完整 Compose 服务闭环、正式 JWT/mTLS 接线、Task CRUD 和全链路异常恢复验收。
## 2026-09-01 BVS2-06 implementation update

- Compose full-chain and physical recovery acceptance is complete on the isolated `bvs206-closure` project; Docker Hub base-image EOF remains an environment limitation for normal `up --build`.
- Formal JWT/mTLS entry wiring is complete for Core management routes, Edge employee routes, and Worker-to-Edge transport. Local Compose deliberately keeps TLS disabled and development actor headers explicitly enabled.
- Task CRUD is delivered within the frozen semantics: Core list/detail reads with RBAC/scope, Arrival as create, Confirm/Cancel as lifecycle mutations. Generic manual create, arbitrary PATCH, and hard delete remain blocked until their business contract is frozen.

## 2026-09-01 BVS2-07 Employee Identity & Session

- Implemented Core credential verification with bcrypt, failed-attempt lockout, active Staff checks, external identity uniqueness, client-bound one-time binding tickets, and binding audit records in migration `000003_employee_identity`.
- Implemented Edge password login, provider exchange, binding completion, refresh rotation, current-session lookup, logout, and session-backed JWT resolution in migration `000003_employee_session`.
- Added separate Core/Edge JWT audiences, service-to-service identity HTTP contract, local mock provider, OpenAPI updates, and unit coverage for first binding, same-Staff provider exchange, ticket client binding, refresh replay, logout, and session validation.
- Code-level verification passed with `go build ./...` and targeted identity/migration/security tests. Docker preflight failed because `docker.exe` is unavailable, so live migration, Compose, and MySQL integration results remain unverified.

## T0 员工任务实时交付与横向扩展实施顺序（2026-09-02）

> 状态：T0-1、T0-2、T0-3、T0-4 协议与首个员工 Web 客户端已实现；当前系统仍只有一个逻辑 Edge，WebSocket 只是提示通道，多副本和平台通知待实施。
>
> 详细决策：[`docs/adr/ADR-009-employee-task-realtime-delivery.md`](../docs/adr/ADR-009-employee-task-realtime-delivery.md)。

### T0-0 契约和边界冻结（已完成）

- [x] 冻结“一个逻辑 Edge + 相同副本”的拓扑，不按任务类型拆 Edge。
- [x] 冻结 `GET /api/v1/tasks` 是员工任务快照读取入口；WebSocket/平台通知只负责提示。
- [x] 冻结 `task_changed`、`command_changed`、重连、版本、降级和错误语义。
- [x] 保持 Core 唯一事实源、Edge Projection、Outbox/Inbox、Command 幂等和 at-least-once 语义不变。

### T0-1 性能与可靠性基线（采集实现完成，运行基线待真实负载）

- [x] 采集 Core Outbox、Edge Command/Inbox backlog 和 oldest age。
- [x] 采集 Edge/Core HTTP 请求、数据库连接池和 Worker Outbox/Command 拉取、处理、确认延迟直方图。
- [ ] 运行代表性业务负载并生成 p50/p95/p99 基线报告，再决定是否先做批量拉取/确认、连接池、索引、Gateway、多 Worker 或持久 Broker。

### T0-2 拉取恢复契约

- [x] 保留员工 JWT Principal 过滤的完整 `GET /api/v1/tasks` 快照。
- [x] 明确 Projection lag、空结果、取消/完成、刷新和离线恢复的客户端语义。
- [x] 增加持久化员工级 `projection_revision`；当前仍使用完整快照，不把单 Task `sync_version` 当全局 cursor。
- [x] 与 `GET /api/v1/commands/{commandID}` 对齐，确保 Command 重试和状态刷新可恢复。

当前契约返回 `sync_mode=full_snapshot`、`snapshot_at`、员工级 `projection_revision`、Projection lag 状态、`next_cursor=null` 和 `reset_required=false`。空任务是 HTTP 200 的空数组；取消/完成保持在最新 Projection 中；启动、刷新、重连和离线恢复都重新拉取快照，未把本地缓存或离线队列当作可靠事实。增量 API 留待后续冻结稳定 cursor 后再实现。

### T0-3 通知抽象和提交后触发

- [x] 增加 Notification/Fan-out Port，Handler 不直接操作连接。
- [x] 仅在 Edge Projection 事务提交后发通知；通知失败不得回滚业务事实。
- [x] 定义员工隔离、notification ID、task ID、版本和原因字段；通知不携带不必要敏感数据。
- [x] 首期使用单实例内存连接注册表，但不把内存状态当业务事实。

实现：`internal/edge/application/notification` 提供 `TaskChanged`、`Publisher`、`Sink` 和员工隔离的 `InMemoryFanout`。Edge 内部事件路由仅在 `ApplyEvent` 成功且 Event 非重复后发布；重复 Event 不重复通知，通知失败只记录日志/指标并保持 `202`。连接丢失、进程重启和提示丢失由 `GET /api/v1/tasks` 完整快照恢复。

### T0-4 前台 WebSocket 提示

- [x] 使用 Edge JWT/sid 完成握手鉴权、心跳、空闲关闭和指数退避重连。
- [x] 推送变化提示，客户端收到后拉取并按版本校准；WebSocket 断开时继续使用 HTTP 快照恢复。
- [x] 代码级覆盖重复提示、员工隔离、断线重连、空闲关闭和提示丢失语义，不把提示当成业务确认；真实浏览器、服务重启、网络分区和多副本场景留待 T0-5/T0-7。

实现：Edge 新增 `POST /api/v1/realtime/ticket` 和 `GET /api/v1/ws`。ticket 是 30 秒一次性不透明凭据，只能通过员工 Bearer JWT 获取，并以 `flight.realtime.ticket.{ticket}` 子协议候选值完成浏览器握手；服务端只选择 `flight.realtime.v1`，不回显 ticket。`internal/edge/application/realtime` 管理连接生命周期、同源校验、`ready`/`ping`/`pong`、空闲关闭、写超时和连接清理。`employee-web` 的 `RealtimeClient` 在连接成功、重连和 `task_changed` 后触发完整快照刷新，通知 ID 有界去重，ticket 401 停止重连并进入认证恢复路径。

### T0-5 Gateway 与多副本

- [x] 增加 Gateway/Load Balancer，支持两个相同 Edge 副本；Edge 副本不再占用固定宿主机端口，Gateway 负责 HTTP/WebSocket Upgrade 转发。
- [x] 确认所有 Edge 副本共享 Edge MySQL，Session/Command/Projection/Inbox 不依赖进程内权威状态。
- [x] 支持多个 Worker 的数据库 claim/lease 和幂等处理；lease owner 贯穿 Pull/Ack，过期租约可重新领取，旧确认不会覆盖新 owner。
- [x] 对多副本连接增加 Redis Pub/Sub 临时 fan-out；Redis 只作为 best-effort 提示，Redis 故障时仍回退 HTTP 全量快照。
- [x] realtime ticket 改为共享 Edge SQL 存储并只保存 hash，任意 Edge 副本均可安全消费单次 ticket。

### T0-6 后台平台通知

- [ ] 为微信小程序订阅消息、企业微信等建立独立 Provider Port，先完成授权、模板和失败语义，再接入真实凭据。
- [ ] 平台通知只负责后台提醒和跳转；点击后仍从 Edge 拉取最新 Projection。
- [ ] 平台不可用、未授权和重复通知不得阻塞 Core/Edge 可靠同步。

### T0-7 故障与容量门禁

- [ ] 验证 Edge/Worker/Redis/fan-out/网络分区/WebSocket 断线/重复乱序消息/多副本故障。
- [ ] 分别报告业务事实正确性、Projection 收敛、推送延迟、拉取恢复和副本可用性。
- [ ] 只有在真实指标和可重复测试满足目标后，才评估 NATS/RabbitMQ/Kafka 等持久 Broker；不以“强实时”作为未经测量的结论。

### T0 明确不做

- [ ] 不按不同任务类型创建不同 Edge。
- [ ] 不让 WebSocket、Redis 或平台通知替代 SQL Projection、Outbox、Inbox 和 Command status。
- [ ] 不在 T0 拆分 Core 模块为微服务，不默认引入持久消息 Broker 或 Service Mesh。
## 2026-09-03 P0 management capability slice

- [x] Add Core-owned admin SSO state/session/identity contracts, OIDC and explicit development Provider adapters, stable handlers, role/scope restoration, and revocable session checks.
- [x] Add Core personnel and assignment read APIs with RBAC, principal-derived Team/Area/User scope, pagination limits, OpenAPI, MySQL adapters, and frontend client contracts.
- [x] Add Core migration `000005_admin_sso`, ADR-011, and unit coverage for one-time state consumption, session replay/role/scope behavior, and management authorization.
- [ ] Connect the selected production identity source, provision `admin_identity`, apply the migration to an isolated environment, and perform live SSO/HTTP/SQL acceptance.
- [ ] Complete remaining management modules and the 503/restart/network-partition/multi-replica/performance gates.

Verification: Docker preflight and `scripts/verify.ps1 -Mode all` passed on 2026-09-03 using an ignored project-local Go build cache. No migration or destructive database operation was executed.
