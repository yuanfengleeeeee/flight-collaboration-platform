# 项目进度

## 2026-08-10 Architecture v2 重构启动

- A1 已按真实工作区完成：仓库根目录为 `C:/Users/yuanfengleeeeee/Desktop/flight-collaboration-platform`；当前分支为 `agent/foundation-and-handoff`；远程为 `origin`；旧 HANDOFF 记录的 `main` 分支和 Docker/Go 状态已过时。
- A1 发现工作区已有未提交修改：`internal/common/response.go`、`internal/model/event.go`、`internal/model/flight.go`、`internal/server/router.go`、`internal/server/router_test.go`，以及未跟踪的 `internal/module/event/`、`memory-bank/architecture-design.md` 和 `migrations/mysql/000003_task_instance_trigger_event_unique.*`。这些属于已有 B3 现场，本轮不覆盖、不回滚、不继续扩展。
- 旧结构为 `cmd/server` + 单一 `migrations/mysql` + `internal/model/store/server`；分类为：可复用的配置/日志/request ID/健康检查/SQL migrator 思路 KEEP，连接与入口 REFACTOR，旧业务模型/路由/migration REWRITE 或 legacy 保留，旧 B3 业务现场 PAUSED。
- A2 已冻结 `docs/architecture/architecture-v2.md` 和 ADR-001 至 ADR-008；同步更新了根 README、根设计/技术栈索引、`memory-bank/design-document.md`、`memory-bank/tech-stack.md`、`memory-bank/implementation-plan.md`。后续以 v2 文档为准。
- 当前未宣称 A3-A10 完成；下一步是建立新入口/新目录边界，继续保留上述工作区差异。

## 2026-08-10 A3-A4 实际进度

- A3 已完成：建立 `cmd/core-api`、`cmd/edge-api`、`cmd/worker`、`internal/core`、`internal/edge`、`internal/integration`、`internal/platform`、`internal/shared`、`api/core|edge|sync|schemas` 和部署/灾备文档边界；新增版本化 Event/Command envelope、public ID、配置、日志、request/trace middleware、security Port 和同步 Transport Port。
- A3 验证：使用仓库内临时 Go cache 执行 `go list ./...` 通过。默认 Go cache 先因宿主目录权限拒绝，未归因于代码；临时 cache 解决。
- A4 已完成代码/文件部分：新增 Core/Edge 独立 MySQL 连接和 SQL migrator；新增 `migrations/core/mysql/000001_core_foundation` 与 `migrations/edge/mysql/000001_edge_foundation`；新增独立 `architecture_probe_event`、Outbox、Audit、Core Inbox、Edge Projection、Command、Sync Inbox 等最小表；`cmd/migrate` 改为 `-target core|edge`，`down` 需要显式 `-allow-destructive`。
- A4 验证：`gofmt` 已执行；`go test ./internal/platform/mysql` 通过。`docker compose -f deployments/local/docker-compose.yml config` 未运行成功，原因是当前 PowerShell 找不到 `docker` 命令；未停止宿主机 MySQL，未执行 down-v 或 destructive migration。
- A5 已完成：新增 `core-api`、`edge-api`、`worker` 独立入口和 Core/Edge Server 生命周期；Core/Edge 只装配各自 MySQL/可选 Redis；两套 `/health/live`、`/health/ready` 和 `X-Request-ID`/`X-Trace-ID` middleware 已实现。
- A5 验证：`go test ./cmd/core-api ./cmd/edge-api ./cmd/worker ./internal/core/application ./internal/edge/application ./internal/platform/...` 通过；Core/Edge 无 MySQL 的 liveness/readiness HTTP 测试通过。尚未声称 Docker 容器启动通过。
- A6 已完成：实现 `internal/shared/event` 版本化 Event/Command Envelope；Core Memory/SQL Store 的 Transactional Outbox、Core Inbox、重试/失败状态；Edge Memory/SQL Store 的 Sync Inbox、最小 Task Projection、Command Store、重试/失败状态；HTTP Sync Transport 与 Edge `/internal/sync/v1/*` 路由。
- A7 已完成：实现 Human/Machine Principal、四角色 RBAC、`resource:action` 权限、结构化 Scope、JWT 只含身份/会话声明的 Authenticator，以及业务 Audit 写入 Port/校验。
- A8 已完成：新增 `internal/integration/sync/probe_test.go` 双向 Architecture Probe；Probe 使用 Memory Store，不注册真实业务路由。已验证重复 Event/Command 最终只执行一次、Edge API 可读 Projection、Command 处理会产生 Core Outbox。
- A8 当前验证：`go test ./internal/integration/sync ./internal/core/sync ./internal/edge/sync ./internal/edge/application` 通过。

## 2026-08-10 A9-A10 最终验证结果

- A9 已完成：Outbox 重试预算耗尽进入 `failed`；HTTP/Memory Edge Inbox 对重复 Event 幂等；Command Store 对重复 Command 幂等；Worker processing 状态可被重启实例重新领取；Redis disabled 不阻塞 readiness 或 Core foundation transaction。
- 全量 `go test ./...`：通过。执行时使用仓库内临时 `GOMODCACHE/GOCACHE`，因为宿主默认 Go cache 曾返回 Access denied；测试完成后临时目录已清理。
- 全量 `go build ./...`：通过；Core/Edge 独立入口启动探测：Core `live=200 ready=503`，Edge `live=200 ready=503`，符合无 MySQL 时 liveness/readiness 约定。
- `git diff --check`：通过，仅有 Git 的 LF→CRLF 工作区提示，无 whitespace error。
- `go run ./cmd/migrate -target core|edge -command status`：均未连接成功，Core 3310 和 Edge 3311 连接被拒绝；这是当前没有 Docker/MySQL 容器的真实环境阻塞，不是迁移解析错误。
- `docker compose -f deployments/local/docker-compose.yml config`：未运行成功，当前 PowerShell 找不到 `docker` 命令；因此未宣称 Compose、双 MySQL 实际启动、实际双库 migration 或容器内 Probe 通过。未执行 down-v、TRUNCATE、DROP DATABASE、migration down，也未停止宿主机 MySQL。
- Architecture Foundation 代码、协议、文档和自动化 Probe 已存在；Docker/真实 MySQL 相关验收需在 Docker CLI/服务可用后补跑。
- 最终收口前补充：Core/Edge Redis 配置已拆为各自 optional 配置；MySQL DSN 显式使用 UTC session 时区，migration 默认时间使用 `CURRENT_TIMESTAMP(6)`。修改后再次执行 `go test ./...` 与 `go build ./...`，均通过。

## 2026-08-06

- 完成仓库和工作区状态检查。
- 确认项目已有 PRD、设计文档、技术栈文档和 Go 原型骨架。
- 确认当前先建设基础架构，不编写核心业务逻辑。
- 创建 `memory-bank/`，建立设计基线、技术栈基线、架构记录和基础架构实施计划。
- 当前未完成基础层实现，也未宣称构建或测试通过。
- Step 1 工具链检查结果：Git 可用；Go 1.26.5 已通过官方校验并完成用户级安装；Docker Desktop 已安装；Docker Compose CLI v5.3.1 可用。
- 本次复核结果：`go version go1.26.5 windows/amd64` 成功；`docker info` 成功返回 Docker Server 29.6.2。此前 Docker 虚拟化阻塞记录已过时，当前可以继续项目基础验证。

## 2026-08-07

- Step 1 已完成：`go test ./...` 通过；`go build ./cmd/server` 通过。
- Compose 配置校验通过；发现宿主机已有 `mysqld` 占用 3306，未停止该进程。
- 将 MySQL 宿主机端口改为可通过 `MYSQL_HOST_PORT` 覆盖，默认仍为 3306；本机以 `3307:3306` 启动验证。
- MySQL 8.0.46 容器已启动，`mysqladmin ping`、数据库初始化查询均通过；Redis 容器已启动并返回 `PONG`。
- Step 2 已完成：已冻结模块边界、MySQL/Redis 职责、版本化 SQL 迁移、`FLIGHT_` 配置覆盖、JWT/RBAC 骨架、日志错误规范和测试分层，并同步到设计/技术/架构记录。
- Step 3 已完成：新增 `server.Application` 统一 HTTP 启动、监听错误、信号退出和依赖关闭；健康检查拆分为 liveness/readiness，数据库不可用时 readiness 返回 503；健康检查单元测试通过。
- Step 3 端到端验证通过：使用临时本地配置启动服务，readiness/live 均返回 200，MySQL/Redis 均显示 `ok`，验证进程已关闭。
- Step 4 已完成：新增 `migrations/mysql/000001_initial_schema.{up,down}.sql`、`internal/store/migrator.go` 和 `cmd/migrate`；Docker MySQL 上完成 `status → up → status → up → status`，15 张表和 `schema_migrations` 记录已验证。真实 `down` 未对现有数据卷执行，避免未经确认删除表。
- Step 5 已完成：Viper 支持 `FLIGHT_` 环境变量覆盖和配置校验；统一响应支持 request ID；新增 request ID 中间件并写入请求日志；配置、健康路由和端到端服务验证通过。
- Step 6 已完成：新增独立 JWT/权限包、认证登录处理器、Bearer 校验、角色/权限中间件和 `/api/v1/auth/me`；覆盖未认证 401、角色不匹配 403、issuer/算法/过期 token；未创建默认用户，避免把测试账号带入基础迁移。
- Step 7 已完成：Compose 增加 MySQL/Redis healthcheck；容器均为 `healthy`；`go test ./...`、`go build ./...` 通过；使用 `FLIGHT_MYSQL_PORT=3307` 启动服务后 readiness 返回 200，未认证 `/api/v1/auth/me` 返回 401；迁移状态保持 `initial_schema applied`。

## 基础层验收门（2026-08-07，legacy 历史记录）

- 已通过：工具链、Go 构建测试、Compose 配置、MySQL/Redis 健康、版本化迁移、服务启动/优雅关闭、liveness/readiness、统一响应/request ID、配置校验、JWT、角色/权限拒绝路径、基础部署说明。
- 有意保留：没有默认用户/种子账号；真实登录成功流程需要后续测试夹具或明确的初始化账号策略。
- 有意未执行：没有对现有 Docker 数据卷执行真实 `down`，因为该操作会删除表；迁移文件、状态追踪和回滚入口已存在，真实回滚应在隔离测试库中验证。
- 当前结论：基础架构可以进入第一个业务垂直切片，但在写业务代码前仍需重新读取 PRD/设计文档，并冻结首个切片的业务状态机和验收标准。
- 业务切片 001 已冻结：航班到达后的任务确认闭环；已明确状态机、幂等、硬约束、角色权限、验收标准和数据库缺口。当前仍未开始业务代码，下一步是业务 Step B1 数据结构迁移。
- 业务 Step B1 已完成：新增班组/成员、任务候选、通知、审计模型；扩展事件幂等字段、任务模板触发类型、任务实例版本/班组、分配确认字段；`000002_task_confirmation_foundation` 已在 Docker MySQL 应用，重复迁移通过；全量测试和构建通过。
- 业务 Step B2 已完成：新增 `internal/testsupport` 场景夹具，覆盖合格员工、跨班组员工、能力不足员工和忙碌员工；支持显式 `Persist`/按主键反向 `Cleanup`，不使用 `TRUNCATE`；默认测试为内存单元测试，不接触开发数据库；全量测试和构建通过。
- 历史下一步计划：业务 Step B3，实现 `flight_arrived` 事件幂等处理和任务生成；该计划已被 Architecture v2 Foundation 重构取代，未在本轮执行。

## 2026-08-10 A10 重新验证补充（历史状态，已由 2026-08-13 验收记录覆盖）

- Docker Desktop 已确认正在运行：CLI 位于 `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`；Docker Engine `29.6.2`、Compose `v5.3.1`。`docker compose -f deployments/local/docker-compose.yml config` 已通过。
- 新增 `.dockerignore` 排除约 1GB 的本地临时 Go cache/MySQL 数据。三个 v2 镜像 `local-core-api`、`local-edge-api`、`local-worker` 已实际构建成功。
- Compose 创建 v2 network、Core/Edge volumes 和容器，但 MySQL 启动阶段 Docker Linux Engine 返回 API 500，随后容器列表/inspect 和 `docker version/info/ps` 超时。没有执行 `down -v`、删除 volume、migration down、TRUNCATE、DROP DATABASE，也没有触碰旧原型 `flight-mysql`/`flight-redis`。
- 使用本机 MySQL 8.0.35 在临时目录启动两个独立实例 3310/3311，完成 Core/Edge 独立 migration `pending -> up -> applied`；真实 Core API、Edge API、Worker 进程 readiness 均为 200，MySQL 为 ok，Redis 缺失仍为 unavailable，未阻断核心运行。
- `FLIGHT_RUN_DB_PROBE=1 go test ./internal/integration/sync -run TestArchitectureProbeAgainstRunningServices -count=1 -v` 已在临时双库/真实进程上通过，验证 SQL Core transaction、Outbox、HTTP Edge Inbox/Projection、Edge Command、Worker pull、Core Inbox、回传 Projection 和 API 读取。代码随后补充 SQL failed record/retry 与 command attempts/退避处理；更新后的 opt-in SQL Probe 尚未在 Docker Engine 恢复后重跑。
- 最新 `go test ./...`、`go build ./...`、`git diff --check` 均通过。A10 的剩余阻塞仅为 Docker Engine 恢复后的容器健康、容器 migration 和容器 Probe；在此之前不进入 B3/B4/B5。

## 2026-08-13 A10 Docker 容器级最终验收

- Docker Desktop 已恢复并验证：Docker `29.6.2`、Compose `v5.3.1`、context `docker-desktop`；`docker info` 和 `docker compose -p architecture-v2-final -f deployments/local/docker-compose.yml config --quiet` 均通过。
- 独立 Compose project `architecture-v2-final` 已实际运行 `core-mysql`、`edge-mysql`、`core-api`、`edge-api`、`worker`。Core/Edge MySQL 均为 healthy，映射端口为 3310/3311；旧项目和旧 volume 未被删除。
- Core 与 Edge migration 分别完成 `pending -> up -> applied`，服务启动不调用 AutoMigrate。
- Core/Edge 的 live/ready 四个端点均为 HTTP 200；ready 细节为 `mysql=ok`、`redis=unavailable`。
- 容器环境 `TestArchitectureProbeAgainstRunningServices` 通过，覆盖 Core -> Outbox -> Edge Inbox -> Projection 与 Edge -> Command -> Core Inbox -> Outbox -> Projection，并验证重复 Event、重复 Command、失败记录和重试。
- 故障注入通过：停止 Worker 后重启，Pending Outbox 被继续处理；停止 Edge API 后恢复，Core Outbox 经 retry/backoff 成功补发。
- 最终命令结果：`go test ./...`、`go build ./...`、`git diff --check` 均通过；v2 相关 48 个 Go 文件 `gofmt` 检查通过。全仓旧 legacy 文件存在既有格式差异，本轮未为此改写旧现场。
- A1-A10 全部完成，Architecture v2.0 Foundation 状态更新为 `ACCEPTED / FROZEN`。B3/B4/B5 真实业务仍未在本轮实现；后续另开业务阶段。

## 2026-08-13 Phase 2 业务阶段切换（历史状态，已由 2026-08-28 BVS2-01 冻结记录覆盖）

- 用户已正式确认 Architecture v2.0 Foundation 为 `ACCEPTED / FROZEN`；架构阶段结束，不再为“更完善”继续扩展底层基础设施。
- 项目状态切换为 `Phase 2 — Business Implementation: READY`。
- 下一项正式任务改为 `Business Slice v2 — Flight → Task → Personnel → Leader Confirm → Edge → Employee → Complete`。
- 新切片入口为 `memory-bank/business-slice-v2-flight-task.md`；旧 `business-slice-001.md`、旧 B3 代码现场和旧路由明确保持 `legacy/paused`，不直接恢复。
- 当前仅登记并冻结业务切片入口，尚未开始业务代码；下一步是 `BVS2-01` 业务设计冻结。设计通过前不进入 Core 业务迁移或 Handler/Service 实现。
- 新业务必须复用并遵守已验收边界：Core 唯一事实源、Edge 最小 Projection、Command 经 Core 校验、Outbox/Inbox、幂等、Retry、RBAC/Scope、Human/Machine Principal 和 Audit。

## 2026-08-28 BVS2-01 业务设计冻结

- 按用户确认的 Phase 2 顺序完成 BVS2-01；本轮只更新业务设计和项目交接文档，没有写入真实 Flight/Task/Personnel/Assignment 代码、业务 Migration、OpenAPI 或 Docker 变更。
- 冻结六组交付物：业务规则、状态机、RBAC + Data Scope 权限、Core/Edge Event 与 Command 契约、异常/补偿、可执行验收场景。
- 核心决策已固定：Flight 只提供事实和触发条件；Task 独立拥有生命周期；Candidate 不等于 Assignment；Personnel 状态只能由 Personnel Application/Domain 通过 Port 变更；员工 Accept/Complete 只能走 Edge Command；Core 是最终业务事实源。
- 幂等已区分传输层和业务层：`event_id`、`source_event_id`、`generation_key=flight_public_id + trigger_type`、`confirmation_id`、`cancellation_id`、`command_id` 和 Projection `sync_version` 各自有唯一职责；相同 ID 重放返回原结果，不同 ID 但目标已完成返回 `already_*`；内容冲突返回专用 conflict 结果。
- 取消、人员失效、迟到 Command、乱序 Projection、Edge 不可用、Worker 重启、事务原子性和 Redis 缺失均已写入规则；验收清单为 BVS2-AT-01 至 BVS2-AT-30，后续转为 BVS2-03 至 BVS2-06 的自动化测试。
- 在 BVS2-01 冻结记录产生时，BVS2-02 标记为 `READY`，当时需补齐 Core 业务事实/历史表和可重放的 Command 结果持久化；该项已由下方最新记录完成。Edge 仍不得创建完整 Core 业务副本，旧 B3/B4/B5 继续 `legacy/paused`。
- 本轮未重新运行 Docker/数据库验证；此前 Architecture Foundation 的真实容器验收结果仍以 2026-08-13 记录为准。文档一致性检查已通过；本轮 `go test ./...` 通过，`go build ./...` 返回成功但 Go 尝试写入用户级 stat cache 时出现 Access denied 非阻塞提示，`git diff --check` 通过。

## 2026-08-28 BVS2-02 Core 业务表与版本化 Migration

- BVS2-02 已完成：新增 `migrations/core/mysql/000002_core_business_slice_v2.up.sql` 与对应 down 文件；没有修改 Edge migration，也没有开始 BVS2-03 及后续真实业务代码/API。
- Migration 按 BVS2-01 冻结表设计建立 14 张 Core 表：`operation_area`、`team`、`personnel`、`team_member`、`flight`、`flight_status_history`、`task_template`、`task_instance`、`task_status_history`、`task_candidate`、`task_assignment`、`task_assignment_status_history`、`personnel_status_history`、`business_idempotency_record`。既有 `audit_log`、`outbox_event`、`core_inbox` 保持在 Foundation migration 中。
- Docker Engine 已恢复；复用已有 `architecture-v2-final` Compose 项目和独立卷，未删除/重建卷，未停止旧 `flight-mysql`（3307）或 `flight-redis`（6379）。Core/Edge MySQL 均 `healthy`，Core/Edge API 与 Worker 均 `Up`。
- Docker 实际迁移结果：Core `status` 为 `000001 applied`、`000002 applied`；Edge `status` 为 `000001 applied`；重复执行 Core `up` 成功且状态不变。Core 14 张 BVS2-02 表、20 个外键、`uk_task_instance_generation_key` 和 `uk_business_idempotency_operation_key` 均存在；Core/Edge 互不包含对方业务表，`tenant_id`/`airport_id` 列数量均为 0。
- Docker 健康检查：Core/Edge `/health/live` 与 `/health/ready` 均返回 200；readiness 为 `mysql=ok`、`redis=unavailable`，符合 Redis optional 约定。`docker compose ... config` 已通过。
- 本阶段继续遵守非破坏性边界：未执行 `docker compose down -v`、数据库清空、DROP/TRUNCATE 或 migration down。下一步为 `BVS2-03 Flight → Task → Candidate`。

## 2026-08-31 BVS2-03 Flight → Task → Candidate

- BVS2-03 已完成：新增 Core `flight`、`personnel`、`task` 领域模型，跨模块 Application Service、Core MySQL Repository 和 `POST /api/v1/flights/{flightPublicID}/arrival` 入口。没有恢复旧 B3，也没有实现 Leader Confirm、Assignment、Edge Projection 或员工 Command。
- 到达用例严格实现冻结规则：仅允许 `scheduled → arrived` 触发；按启用的最高模板版本生成 `awaiting_confirmation` Task；Candidate 与 Assignment 分离；候选需要当前主 Team/Area、启用、岗位匹配、全部能力、`idle` 且计划时间无活动 Assignment 冲突，并按 `last_state_changed_at ASC, public_id ASC` 稳定排序。
- Flight 更新、`flight_status_history`、Task、`task_status_history`、Candidate 快照、业务幂等记录、Audit 和 `task.generated.v1` Outbox 由同一个 Core MySQL transaction 提交。重复 `source_event_id` 返回原结果且不产生重复事实；来源内容冲突返回 `source_event_id_conflict`；非法 Flight 状态的拒绝结果可安全重放。
- 新增测试覆盖正常生成、候选过滤/排序、重复来源事件、来源内容冲突、同 generation key 重放、无启用模板、非法状态和稳定 HTTP 响应。`go test ./...` 已实际通过；隔离 Docker Core MySQL (`127.0.0.1:3310`) 的 opt-in SQL 测试也已通过，验证了真实表写入、候选查询、Audit/Outbox/幂等记录和重复请求不重复写入。
- Docker Desktop Engine 已恢复，Compose 配置检查通过，Core/Edge MySQL 使用已有独立卷并均为 `healthy`。本轮尝试重建新应用镜像时，Docker Hub 拉取 `golang:1.25` / `debian:bookworm-slim` 返回 `EOF`；因此没有把旧应用镜像冒充为包含 BVS2-03 的容器验证结果。该环境层问题不影响已通过的 Go 与真实 Core SQL 验证。
- 本轮没有执行 `docker compose down -v`、DROP、TRUNCATE、破坏性 migration down，也没有停止或修改旧 `flight-mysql`/`flight-redis`。下一项为 `BVS2-04 Leader Confirm`。

## 2026-08-31 项目长期记忆与 Docker 验证前置

- 新增 `memory-bank/project-memory.md`，保存跨会话长期约束；在 `AGENTS.md` 中要求开始新任务或写代码前读取该文件。
- 新增 `scripts/ensure-docker.ps1`：检测 Docker Engine；未就绪时自动启动 Docker Desktop 并等待 Engine 就绪。脚本不会自动启动项目容器、执行 Migration、删除 Volume 或清理数据库。
- 新增 `scripts/verify.ps1`：所有验证模式先调用 Docker 前置脚本，再执行 Go 测试、构建、Compose 配置或服务状态检查。
- 实际验证：Docker Desktop 原本未运行时，`ensure-docker.ps1` 成功启动 Docker Desktop，Docker Engine 就绪；`verify.ps1 -Mode all` 的 `go test ./...`、`go build ./...`、Compose 配置检查和 `git diff --check` 均通过。
- 本次只增加长期记忆、验证脚本和文档规则，没有执行破坏性数据库操作，也没有提交、推送或切换分支。

## 2026-09-01 BVS2-05 Edge Projection → Employee Command（最新状态）

- 本轮继续 BVS2-05，未恢复旧 B3/B4/B5。Edge 已能把 Core 的 `task.assigned.v1`、`task.accepted.v1`、`task.completed.v1`、`task.cancelled.v1` 转成最小 Task Projection，并按 `sync_version` 处理乱序、重复和同版本冲突。
- Edge 新增 `assignment_public_id` 字段及 `migrations/edge/mysql/000002_edge_business_slice_v2`；新增员工 `accept`/`complete` HTTP 命令入口，Actor 只取自 `X-Employee-Public-ID`。
- Core 新增 `employee_accept_task.v1` / `employee_complete_task.v1` 处理器和 MySQL 业务命令事务：本人 Assignment、RBAC、状态、版本校验通过后，同事务更新 Task/Assignment/Personnel，写三类 History、Audit 与 `task.accepted.v1` / `task.completed.v1` Outbox。
- Worker 新增 CommandProcessor 边界；有 Core MySQL 时使用业务 Repository 的同库 Core Inbox 事务，Foundation Probe 仍由原 Core sync store 处理。
- 修复 MySQL JSON 列对象键顺序导致的同 command ID 重放冲突误判；Command 比较改为 JSON 语义等价，同时保留不同业务内容冲突。
- 已在隔离临时 Edge MySQL 应用两个 migration，并通过 `FLIGHT_RUN_BVS2_EDGE_DB_TEST=1` Projection 版本收敛/冲突 SQL 测试；已在隔离临时 Core MySQL 应用两个 migration，并通过 `FLIGHT_RUN_BVS2_DB_TEST=1 go test ./internal/core/adapter/mysql -run TestFlightTaskSQLAgainstDocker -count=1 -v`，覆盖 Accept→Complete 原子状态、Core Inbox 幂等、History、Audit 与 Outbox。
- `powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all` 已通过，包含 Docker 前置、全仓 Go 测试、构建、Compose config 和 `git diff --check`。本轮临时容器已停止并清理；原有 `local_core_mysql_data` 未删除或重置。
- 当前下一步：进入 BVS2-06，补充完整 Compose 服务闭环、正式认证接线、Task Cancel/CRUD 和全链路异常恢复验收。

## 2026-08-31 BVS2-04 Leader Confirm（历史记录）

- BVS2-04 已完成：新增 `ConfirmationService`、Confirmation Repository Port、Core MySQL 事务适配和 `POST /api/v1/tasks/{taskPublicID}/confirm`；没有恢复旧 B3，也没有实现 Edge Projection 或员工 Command。
- 成功确认严格要求 Human Principal、`task:assign` 权限以及同时满足 Task Team/Area Scope；锁定并校验 Task 版本，复核当前 Candidate/Personnel，原子化执行 Personnel `idle → reserved`、创建 `confirmed` Assignment、Candidate `selected/rejected`、Task `awaiting_confirmation → assigned`，并写入 Task/Assignment/Personnel History、Audit 和 `task.assigned.v1` Outbox。
- `confirmation_id` 写入 Assignment 和业务幂等记录；同 ID 同内容返回首次结果且无重复副作用，不同内容返回 `confirmation_id_conflict`。已分配 Task、过期版本、越权和候选人当前失效均返回稳定错误码；候选人失效不会创建半成品 Assignment。
- `api/core/openapi.yaml` 已同步确认请求/响应和错误码；当前 Core v2 HTTP 内部入口可接收认证中间件注入的 Principal，并保留 `X-Actor-*` 测试适配。正式 JWT middleware 接线留在后续入口收敛，不改变 Application 层授权边界。
- 已验证：定向确认单元测试、全仓 `go test ./...`、`go build ./...`、`git diff --check` 均通过；在临时内存卷 Core MySQL 完成 Core migrations 后，`FLIGHT_RUN_BVS2_DB_TEST=1 go test ./internal/core/adapter/mysql -run TestFlightTaskSQLAgainstDocker -count=1 -v` 通过，真实验证覆盖成功确认、历史、Assignment、人员状态、Audit、Outbox 和幂等重放。
- 验证期间发现原有 `local_core_mysql_data` InnoDB 数据卷损坏；未执行删除、重置或 destructive migration，使用了可清理的临时内存卷。该损坏卷仍保持未修改状态。
- 本段为 BVS2-04 完成时的历史交接；当前状态以本文件上方的 BVS2-05 最新状态为准，BVS2-06 完整闭环和旧 B3/B4/B5 继续按边界推进。

## 2026-09-01 前端工程设计启动

- 已重新读取项目长期记忆、Architecture v2.0、当前架构记录、设计文档、实施计划、BVS2 业务切片、前后端交接文档和当前工作区状态；确认仓库尚无前端工程目录。
- 已新增 `memory-bank/frontend-design-document.md`、`memory-bank/frontend-tech-stack.md` 和 `memory-bank/frontend-implementation-plan.md`，建立 Mock-first 的 `admin-web` / `employee-web` 双应用方案。
- 设计遵循 Core/Edge 数据边界：管理端只面向 Core，员工端只面向 Edge Projection/Command；浏览器不调用内部同步接口、不直连数据库，202 Command 结果不被前端直接当作最终业务状态。
- 当前前端设计仍为 `DRAFT/PROPOSED`。正式 JWT/OIDC、Core 任务列表/详情/创建/取消契约、Edge 员工认证、客户端 Command 幂等键/状态查询、员工端最终形态和视觉品牌需要产品/后端确认。
- 本轮未创建前端代码，未运行前端构建/测试；这是文档设计阶段，不得将前端工程写成已实现或已验证。

## 2026-09-01 前端工作区目录骨架

- 已创建 `frontend/` 工作区目录骨架：`apps/admin-web`、`apps/employee-web`、`packages/contracts`、`api-client`、`auth`、`ui`、`task-domain`、`mock`、`e2e` 和 `docs`。
- 已新增 `frontend/README.md` 和各源码目录占位文件；未创建前端业务源码、`package.json`、lockfile、构建配置或可运行页面。
- 已同步更新 `tech-selection-report.md`、`tech-stack.md`、`memory-bank/tech-stack.md`、`memory-bank/architecture.md`、前端设计/技术栈/实施计划和 `docs/frontend-backend-handoff.md`，明确目录已落地但前端实现仍为 `DRAFT/PROPOSED`。
- 已补充 `frontend` 的构建产物和依赖目录忽略规则；本轮未运行前端构建或测试，Go/Docker 验证结果沿用上方已有真实记录。
## 2026-09-01 BVS2-06 Task Cancel（首个功能步骤完成）

- 已完成 Core Task Cancel：新增 CancellationService、CancellationRepository/MySQL transaction、取消 HTTP handler 和 Core server/cmd 装配。
- 已完成权限边界：task:cancel 仅授予 admin/manager/leader；Human + Team/Area Scope 必须同时满足；leader 仅允许取消 awaiting_confirmation/assigned，staff、machine 和越权请求拒绝。
- 已完成状态机：awaiting 取消使 proposed Candidate 全部 invalidated；assigned 取消执行 Assignment confirmed → cancelled、Personnel reserved → idle；in_progress 取消执行 Assignment accepted → cancelled、Personnel busy → idle；completed/cancelled 只返回稳定终态结果。
- 已完成一致性：Task/Assignment/Personnel History、Audit、task.cancelled.v1 Outbox、业务幂等记录同事务提交；同 ID 重放无重复副作用，冲突 ID 返回 cancellation_id_conflict；长取消 ID 的 CorrelationID 兼容现有 CHAR(36)。
- 已验证：Docker Engine 前置脚本成功；取消单元及受影响 Core 包测试通过；独立临时 Core MySQL 完成现有 migrations 后，TestTaskCancellationSQLAgainstDocker 通过。临时容器待本轮结束清理，已有项目容器/数据未触碰。

下一步：继续 BVS2-06 的完整 Compose 服务闭环和恢复验收，再处理正式 JWT/mTLS、Task CRUD 与其余异常场景。

## 2026-09-01 BVS2-06 Compose 闭环与恢复验收

- 使用独立 Compose project `bvs206-closure`、宿主机端口 3330/3331 和独立 Core/Edge volume 启动当前 Core API、Edge API、Worker 与双 MySQL；现有 `flight-mysql`/`flight-redis` 未停止或修改。
- `scripts/ensure-docker.ps1` 在 Compose、migration、Probe 和故障注入前均成功确认 Docker Engine 就绪。Core/Edge migration 均从 pending 应用为 `000001`、`000002 applied`；Core/Edge `/health/live` 与 `/health/ready` 全部 HTTP 200，ready 为 `mysql=ok`、`redis=unavailable`。
- 当前代码的真实 SQL/HTTP Probe `TestArchitectureProbeAgainstRunningServices` 通过，覆盖双库 Outbox/Inbox、Edge Projection、Command 幂等、失败记录/重试和 Edge API 读取。
- 物理恢复验收通过：停止 Worker 后写入的消息在 Worker 重启后继续投递；停止 Edge API 时 Core Outbox 进入 retry，Edge 恢复后 Worker 成功补投，Probe 最终通过。
- 故障场景暴露并修复 MySQL `DATETIME(6)` 重放比较问题：MySQL 会对微秒进行四舍五入，Core Inbox 之前用截断比较导致同一 `command_id` 被误报 `command id was reused with different content`；新增共享 `EqualPersistedTime`，Core/Edge 写入和比较统一使用持久化精度。
- Docker Hub 对 `golang:1.25`/`debian:bookworm-slim` 的元数据请求仍返回 `EOF`，所以 `docker compose up --build` 环境路径未通过；为避免旧镜像冒充当前代码，本轮交叉编译当前二进制并装入本地缓存运行时镜像完成了当前代码的容器闭环。临时验证镜像/文件待本阶段最终清理。

当前下一步：BVS2-06 正式 JWT/mTLS 接线与鉴权拒绝路径，随后实现已冻结范围内的 Task 查询/详情/生命周期管理 CRUD。

## 2026-09-01 BVS2-06 JWT/mTLS 正式入口

- 新增 `internal/platform/httpauth` JWT HTTP 适配层；Core Confirm/Cancel 与 Edge 员工 Task/Command API 在生产装配中强制 Bearer JWT，合法 Token 注入 `security.Principal`，缺失/无效 Token 返回 401。
- `X-Actor-*`/`X-Employee-Public-ID` 仅在 `allow_dev_actor_headers=true` 且非 release 模式时作为显式本地调试适配；release 配置会拒绝该开关。Edge Command 还校验命令 Actor 与认证 Principal 一致。
- JWT 继续不携带权限列表；解析支持签名 Token 中的可选角色与结构化 Scope 属性，最终权限仍由 Core IAM Authorizer 和业务 Service 校验。
- 新增可配置 TLS/mTLS：Core/Edge Server 支持证书、CA、最小 TLS 版本和 `RequireClientCert`；Worker Sync Transport 支持服务端 CA 校验、客户端证书和私钥。默认本地 HTTP/调试配置保持关闭。
- 已通过 JWT middleware、TLS 证书加载、Core/Edge server、Transport 与同步回归测试；当前隔离 Compose 已实测匿名 401、开发适配器和双向 Probe。

当前下一步：实现 Core Task 查询/详情以及与既有 Arrival/Confirm/Cancel 生命周期兼容的 CRUD API，保持 Core 事实源和 Scope 校验。

## 2026-09-01 前端双端平台选型分析

- 用户已确认首期同时建设管理端和员工端；当前 `frontend/` 已有目录骨架，前端源码、依赖清单、构建配置和页面仍未实现。
- 平台建议已收敛为：`admin-web` 使用桌面优先的响应式 Web；`employee-web` 使用移动端优先的响应式 Web，并可渐进增强为 PWA；微信小程序暂作为后续第二客户端，不在首期默认创建。
- 该建议基于当前使用方式：管理端需要高信息密度、表格/分栏、Candidate 对比和 Confirm；员工端主要是手机上的 Projection 查看、Accept/Complete 和异步同步状态提示。当前员工流程没有已经确认的微信原生能力硬需求。
- PWA 可复用当前 React/TypeScript/Vite、contracts、API client、task-domain、Mock 和 E2E 模型，部署迭代更快；其离线能力只用于资源和有限读场景，不能替代 Edge 持久化 Command。
- 微信小程序的价值主要在微信入口、扫码/分享等生态能力，但会增加独立运行时、身份映射、通信域名/HTTPS、审核发布和兼容性维护成本；如现场确认必须从微信进入，再新增独立 `apps/employee-miniapp`，只复用共享契约和业务域。
- 本轮仅更新设计和交接文档，未运行前端构建/测试；响应式 Web/PWA 与微信小程序的最终冻结仍属于 F0 待确认项。

## 2026-09-01 员工端双微信入口调整

- 根据员工不愿新增软件或网页的实际痛点，员工端平台建议从响应式 Web/PWA 调整为首期微信小程序。
- 员工端只建设一套小程序，同时支持个人微信和企业微信入口；两类外部身份必须映射到同一个 Staff，不创建两套员工账号或两套任务事实。
- 个人微信/企业微信差异只存在于入口、登录态和身份适配层；任务列表、任务详情、Accept/Complete、202 pending 和 Projection 收敛共用同一套业务模型与 Edge API 边界。
- 管理端继续采用桌面优先响应式 Web；`employee-web` 保留为正式可用网页版和备用入口。`employee-miniapp` 小程序业务壳层与员工网页版壳层在后续 F1 初始化，具体小程序技术方案仍待确认。
- 本轮仅同步平台设计和交接文档，未创建小程序源码，未运行前端构建/测试；最终认证、身份绑定、通信域名和 Command 客户端幂等键仍需 F0/F2 冻结。
## 2026-09-01 BVS2-06 Task CRUD 读侧闭环

- Core 新增 `GET /api/v1/tasks` 与 `GET /api/v1/tasks/{taskPublicID}`，通过独立 Query Port 和 MySQL 参数化查询返回任务、Candidate 快照及当前 Assignment。
- 读接口执行 `task:read`、Human Principal 和 Team/Area/Assigned Scope；分页、状态和 Flight 过滤均有边界校验，越权任务不会通过 Repository 查询返回。
- 本切片的 create 仍是 Flight Arrival 事务，delete-like 生命周期仍是带 History/Audit/Outbox 的 Task Cancel；未实现尚未冻结语义的手工 POST/PATCH/硬删除。
- 已通过 Task Query 单元/HTTP 测试、`TestFlightTaskSQLAgainstDocker`（真实 Core MySQL）和重建后 Compose HTTP smoke/双向 Probe。

## 2026-09-01 F0 前端架构冻结审计启动

- 已完成当前前端目录、Core/Edge 路由、OpenAPI、前端设计文档和交接文档的基线审计。
- 新增 `frontend/docs/architecture-freeze-gate.md`，将 F0 拆成客户端边界、身份映射、Command 幂等/状态查询、Task 生命周期、OpenAPI、一致性、工具链、Mock 数据、域名和视觉基线等可验收门槛。
- 当前已确认：`admin-web → Core API`；`employee-miniapp → Edge API`；`employee-web → Edge API`；所有前端运行时内容位于 `/frontend`；员工小程序同时支持个人微信和企业微信；两类身份必须映射到同一个 Core `Staff`。
- 当前未冻结：双微信及员工 Web 登录接线、外部身份绑定/失效规则、客户端 Command 幂等键、公共 Command 状态查询、Task 生命周期最终矩阵、OpenAPI 与 JWT 现状的完全对齐、前端工具链、Mock/测试账号和环境域名。
- 本轮只完成文档和边界收敛，未创建真实业务页面、前端构建配置或生产认证实现；不把未运行的前端验证写成已通过。

## 2026-09-01 F0 身份合同提案

- 已审计当前身份基础：Core/Edge 可验证 Bearer JWT 并注入 `security.Principal`；外部身份 Provider、Staff 绑定、生产登录换取和 Principal Resolver 尚未接线；Edge 的 `mobile_session` 表当前没有对应登录用例。
- 新增 `frontend/docs/identity-contract-proposal.md`，提出个人微信、企业微信、员工 Web 和管理端统一进入平台会话的边界：外部身份不作为业务主键，员工会话最终映射到同一个 `Staff`，Core/Edge 使用客户端隔离的 audience。
- 提案暂不修改 migration、JWT 实现或第三方配置；当前仍等待平台主体/AppID、员工 Web 登录方式、首次绑定/换绑责任、Token 签发与撤销策略确认。
