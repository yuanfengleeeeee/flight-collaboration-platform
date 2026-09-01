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

业务切片的唯一当前入口为 [`memory-bank/business-slice-v2-flight-task.md`](business-slice-v2-flight-task.md)。旧 `business-slice-001.md`、旧 B3 目录和旧业务路由均为 legacy/paused，不得直接恢复。

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
