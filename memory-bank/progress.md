# 项目进度

## 2026-09-03 无 OIDC 方案与员工数据库边界确认

- 用户确认当前没有 OIDC；管理端 SSO 采用企业微信浏览器 OAuth 直连，不需要开发管理端小程序。Core 管理 SSO Provider 白名单已放行 `wecom`，默认配置也以 `wecom` 为主；OIDC 适配器仅作为未来可选扩展，不是当前部署依赖。
- “新的员工数据库”按已冻结的 Core/Edge 架构实现为 Core MySQL 内的员工主数据/凭证模块，不新增第三套物理数据库。组织/人员事实使用 `operation_area`、`team`、`personnel`、`team_member`，账号使用 `employee_credential`，个人微信和企业微信绑定使用 `external_identity_binding`，管理端身份使用 `admin_identity`。
- 企业微信管理 Provider 已覆盖授权地址生成、服务端 access token 缓存、成员 `UserId` 解析和不接收企业外部联系人的约束；真实 CorpID/AgentID/Secret、HTTPS 回调/可信域名、Core `000005_admin_sso` migration、员工导入和 `admin_identity` 预置仍待部署联调。
- 本轮实际验证：Docker 前置通过；`go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all`、前端 `pnpm typecheck`、`pnpm lint`、`pnpm test`（5 文件/13 测试）和 `pnpm build`（admin-web、employee-web）均通过。未执行真实微信/企业微信平台调用，也未执行迁移、清库或破坏性 Volume 操作。

## 2026-09-02 真实身份接入骨架与最终边界

- 已实现个人微信/企业微信真实 Provider 适配器：Core 服务端可在配置开启后调用个人微信 `code2Session`、企业微信 `gettoken/getuserinfo`，并把外部身份映射到同一个 `Staff`；开发环境默认仍使用显式 `mock:<subject>`，真实密钥不进入客户端、日志或仓库。
- 已实现员工原生小程序壳：`app.json` 注册登录、任务、任务详情和账号页；个人微信使用 `wx.login` 一次性 code，企业微信支持 OAuth/H5 或容器注入 code；工号密码登录和绑定流程保持可用。
- 已实现管理端 SSO 前端 callback 适配与 Core 企业微信直连管理会话服务：生成并校验 state，回调只交换一次性 code，不把 Core JWT 放入 URL；Core 已有 `admin_identity`/`admin_sso_state`/`admin_session` 和 `/api/v1/admin/auth/sso/*` 路由。当前部署不依赖 OIDC，具体企业微信应用配置、管理员预置和生产联调仍待完成。
- 已补充 [`frontend/docs/identity-provider-integration.md`](../frontend/docs/identity-provider-integration.md) 和架构/前后端交接记录，明确个人微信需要原生小程序入口、企业微信可用 H5/OAuth、管理端 SSO 不需要小程序。
- 验证结果：Docker 前置检查在授权环境通过；`go test ./...`、前端 `pnpm typecheck`、`pnpm lint`、`pnpm test`（5 文件/13 测试）和 `pnpm build`（admin-web、employee-web）均通过。真实微信/企业微信和管理端 SSO 线上调用未执行，原因是尚未配置生产凭据、HTTPS 域名和管理员预置。

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

## 2026-09-01 分任务交接文件策略

- 已新增 `memory-bank/handoffs/README.md`，定义根 `HANDOFF.md` 总览与 `frontend.md`、`backend.md`、`infrastructure.md` 任务快照的职责边界。
- 已创建当前前端、后端和基础设施交接快照；任务文件采用滚动更新，`memory-bank/progress.md` 继续保存按时间追加的里程碑和临时结果。
- 已更新 `AGENTS.md`、`memory-bank/project-memory.md` 和根 `HANDOFF.md`：当用户说“我要结束当前这个对话”“我要退出当前对话”“准备开新线程”等明确结束会话语句时，先更新对应任务交接文件，再更新根索引，并按既有规则执行安全 GitHub 同步。
- 已明确并发规则：同时读取不会冲突；写入前必须重新读取目标文件、根索引和 `git diff`，发现其他会话修改时先合并事实，不覆盖未知修改。

## 2026-09-01 性能优先与动效限制

- 已将“消息准确性、可靠同步、Core/Edge 状态收敛和故障可恢复性优先于视觉装饰”写入 `AGENTS.md`、`memory-bank/project-memory.md`、`memory-bank/architecture.md` 和前后端技术文档。
- 已新增 `docs/performance-and-reliability-baseline.md`，明确三个前端客户端的动效限制、性能优化要求、暂定 Web 指标和后端 API/数据库/Worker/同步性能待办。
- 已将性能与动效预算加入 `frontend/docs/architecture-freeze-gate.md` 和前端实施计划；当前没有前端可运行源码，未运行前端性能测试，文档中的指标均为待验证目标。

## 2026-09-01 F0 用户身份体验需求确认

- 用户已明确：员工首次进入小程序必须支持“工号 + 密码”登录；工号是平台登录账号，不以个人微信号或企业微信号替代。
- 个人微信和企业微信都可以绑定同一个员工账号；两类身份最终指向同一个 Core `Staff`，任一入口读取的是同一份 Edge Projection，数据更新允许存在短暂同步延迟但不能形成两套事实。
- 用户希望后续登录简单并尽量长时间保持登录；已匹配为已绑定入口快捷登录 + 短期 Access Token + 可刷新、可撤销的长期 Session，退出、改密、Staff 停用和管理员撤销必须能使会话失效。
- 以上需求已补入 `frontend/docs/identity-contract-proposal.md`，并加入 F0 冻结门槛；当前后端仍缺少密码认证、身份绑定、刷新/轮换、登出撤销和双入口可重复验收实现，不能标记为已完成。

## 2026-09-01 说明文档清理

- 已删除明确过时且不再作为实现依据的 `design-document.md` 和 `memory-bank/business-slice-001.md`。未跟踪的 `memory-bank/architecture-design.md` 已确认属于旧 v1 草案，但因其是用户未提交文件且未获得针对该具体文件的再次删除确认，本轮保留。
- 当前有效的架构、业务切片和技术基线分别以 `memory-bank/design-document.md`、`memory-bank/business-slice-v2-flight-task.md`、`docs/architecture/architecture-v2.md`、`tech-selection-report.md` 和 `tech-stack.md` 为准。
- 本次未删除旧业务代码、旧 migration、当前交接文件、ADR、前端 F0 文档、PRD 或性能可靠性基线；它们分别承担 legacy 说明、决策记录、当前交接、前端设计、产品背景或长期约束职责。
## 2026-09-01 BVS2-07 Employee Identity & Session

- Core now owns `employee_credential`, `external_identity_binding`, and one-time `identity_binding_ticket` records. Password verification uses bcrypt, bounded failed-attempt lockout, active Staff checks, provider subject uniqueness, client-bound ticket consumption, and an `identity.bind` audit record.
- Edge now exposes password login, provider exchange, binding completion, refresh rotation, current-session lookup, and logout. Edge stores only the minimum `mobile_session` record; refresh secrets are stored as SHA-256 hashes, access JWTs are short-lived and carry `sid`, and refresh replay revokes the replacement session.
- Core and Edge use separate audiences (`<configured audience>-core` and `<configured audience>-edge`). Core identity calls are protected by the configured internal identity key. Development provider verification accepts only explicit `mock:<subject>` codes; no real WeChat/WeCom integration or production secret was added.
- Added Core/Edge migration `000003`, service ports, HTTP handlers, OpenAPI contracts, and unit tests covering binding-required login, same-Staff provider exchange, ticket/client binding, rotation, refresh replay, logout, and session validation.
- Verification: Docker preflight passed using the installed Docker CLI path; Core and Edge `000003` migrations reached `applied` on isolated project `bvs206-closure`. `go test ./...`, `go build ./...`, and `scripts/verify.ps1 -Mode all` passed. Current-source live HTTP verification covered both provider bindings/exchange, one-time ticket and conflict rules, refresh rotation/replay revocation, logout, internal-key protection, and inactive-Staff fail-closed behavior. During live verification, the MySQL refresh-replay transaction rollback was found and fixed; temporary employees, sessions, and API processes were cleaned up. Real WeChat/WeCom provider integration remains pending.

## 2026-09-01 F0 页面设计完成

- 新增 `frontend/docs/page-design.md`，冻结三端页面路由、外壳导航、认证/绑定/账号安全、管理端任务工作台与详情、员工任务列表与详情、命令反馈和空错状态。
- 冻结“蓝调停机坪”视觉基线：航班条带 + 状态轨为统一识别元素；颜色、字体、间距、圆角、响应式断点、可访问性和 reduced-motion 规则已写明，且遵守同步准确性优先和高成本动效默认禁止的项目约束。
- `memory-bank/frontend-design-document.md` 状态更新为 `FROZEN / F0 PAGE DESIGN`；工程工具链、真实微信/企业微信 Provider、Command 客户端幂等/状态查询、环境域名和在线联调仍由 `frontend/docs/architecture-freeze-gate.md` 管理，未因页面设计完成而提前关闭。
- 根据当前工作区实际代码，校正身份文档为 BVS2-07 已完成本地 Mock Provider、工号密码、双身份绑定、Refresh 轮换和 Logout；真实第三方 Provider 仍未接入，但隔离 Docker 双库在线联调已在 BVS2-07 记录中完成。
- 本轮为设计和文档变更，未运行前端构建/测试或 Docker/数据库验证。

## 2026-09-01 F1 零依赖页面 Mock 预览

- 新增 `frontend/preview/index.html`、`preview.css`、`preview.js` 和 `preview/README.md`，不引入外部依赖，页面文件全部位于 `/frontend`。
- 预览已覆盖管理端任务工作台/详情、状态筛选、候选人确认/任务取消、员工端任务列表/详情、Accept/Complete 的 `pending → 已同步` 状态、账号绑定/登出，以及管理端/员工端登录页切换。
- 视觉实现遵循已冻结的“蓝调停机坪”基线：航班条带、状态轨、深色管理导航、移动任务卡、响应式断点、键盘焦点和 `prefers-reduced-motion`。
- 更新 `frontend/README.md`、`memory-bank/architecture.md`、`memory-bank/project-memory.md`、`memory-bank/frontend-design-document.md`、`memory-bank/frontend-implementation-plan.md`、`memory-bank/handoffs/frontend.md`、`docs/frontend-backend-handoff.md` 和根 `HANDOFF.md`，明确预览是 Mock，不代表真实 API/React/Vite 工程已完成。
- 环境检查：未发现 Node/npm/pnpm，因此未运行正式前端 lint、typecheck、unit、build、E2E；本轮静态检查确认预览文件齐全、无尾随空格、关键交互标记存在，并用 Python 本地 HTTP 服务确认 `index.html`、`preview.css`、`preview.js` 均返回 HTTP 200。按项目规则执行 Docker 前置检查，但因环境找不到 `docker.exe` 以退出码 1 失败，未进行项目级功能/集成/Compose 验证。

## 2026-09-01 F0 页面与权限信息架构重新讨论

- 用户反馈当前预览存在结构性问题：管理端、员工端、登录页不应在一个预览壳中作为正式页面；管理端工作区不能只有任务工作台；页面需要体现项目文档中的更多业务模块；当前 Demo 没有表达任务级权限。
- 新增待确认的权限核心：不同任务可归属不同队长，队长只查看自己管辖范围内的任务，主任可查看全部任务；需进一步确认“队长/主任”与现有 `leader/manager/admin` 的角色映射、Scope 字段、跨区域/跨团队边界、无权/无数据页面和后端授权合同。
- 已将 `frontend/docs/page-design.md`、`memory-bank/frontend-design-document.md`、架构基线和前端交接状态改为 `REOPENED / F0 INFORMATION ARCHITECTURE DISCUSSION`；原有 Mock 保留为探索稿，不再宣称页面或视觉已冻结。
- 后续重新设计顺序：先确认角色与任务可见范围，再梳理三端独立页面/路由，再从项目文档提取完整管理端工作区与首期范围，最后重新评审视觉美化和组件基线；确认前不继续扩展真实前端代码。

## 2026-09-02 F0 信息架构确认与 F1 预览重构

- 用户确认角色映射：主任=`manager`、队长=`leader`、员工=`staff`、系统管理员=`admin`；队长按照团队/区域自动获得任务，主任查看全部团队、区域、队长和任务。
- 用户确认管理端首期完整保留项目规划中的工作区；没有真实内容的模块先显示空态，不删除导航、不伪造业务数据。员工端增加通知、异常、历史和账号入口。
- 将探索预览拆分为页面地图、独立管理端登录、独立员工登录、独立管理端工作区和独立员工端工作区；管理端新增角色 Scope 视图、团队/区域上下文、完整分组导航和模块空态；员工端新增通知/异常/历史入口。
- 本轮保留“蓝调停机坪”作为待评审视觉方向，用户要求的后续美化单独列为修改项；当前不把视觉 Token 标记为最终冻结。
- 验证：按项目规则先执行 `scripts/ensure-docker.ps1`，但当前环境找不到 `docker.exe`，以退出码 1 失败；随后仅进行前端静态/本地 HTTP 资源检查，7 个 HTML/CSS/JS 入口均返回 HTTP 200，未进行项目级功能、数据库或 Compose 验证。
## 2026-09-01 Employee Command 幂等与状态查询

- 员工快捷 Accept/Complete 现在要求客户端提交稳定 `command_id`；同一逻辑命令重试时忽略 delivery trace/occurred_at 元数据，Edge 返回原命令状态和 `duplicate=true`；同 ID 不同业务内容返回 `409 command_id_conflict`。
- 新增 `GET /api/v1/commands/{commandID}`，按员工身份校验归属，公共状态固定映射为 `pending/syncing/confirmed/failed`；返回 attempts、可选 next_attempt_at、created_at、updated_at 和安全的 `error_code`，不泄露原始后端错误。
- Edge Memory/MySQL Store 增加 `FindCommand`、`updated_at` 和并发插入后的内容冲突检查；Core Memory/SQL/业务 MySQL Inbox 统一复用共享逻辑命令等价判断。
- 已运行 `go test ./...`，全量通过。Docker 前置检查在受保护 Docker CLI 路径下通过；下一步为使用当前源码对隔离双库执行 Edge SQL/HTTP 幂等与状态查询验收。
## 2026-09-01 Employee Command 验收完成

- 已在隔离 Compose 项目 `bvs206-closure` 的 Edge MySQL 上使用当前源码完成 HTTP/SQL 验收：首次员工 Accept 返回 `duplicate=false`，同一 `command_id` 重试返回 `duplicate=true`；状态查询返回 `pending` 与时间字段；跨员工查询返回 403；同 ID 不同业务内容返回 409。
- 临时 Edge API 进程、测试命令记录均已清理；既有 `flight-*` 服务和隔离 Compose 服务未停止或修改。Docker 前置检查、`go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all`、Edge OpenAPI YAML 解析和 `git diff --check` 均已通过。

## 2026-09-02 T0 员工任务实时交付方案入文档

- 根据实时任务交付讨论，确认当前只有一个逻辑 Edge，Edge 不按任务类型拆分；多实例方向是相同 Edge 副本 + Gateway + 共享 Edge MySQL。
- 确认员工端采用“推送提示 + 拉取校准”：`GET /api/v1/tasks` 仍是任务恢复和最终读取入口；未来前台增加 WebSocket `task_changed`，后台再接入经授权的平台通知，均不得替代 Projection、Outbox/Inbox 或 Command status。
- 新增 `docs/adr/ADR-009-employee-task-realtime-delivery.md`，并将 T0 顺序同步到 `memory-bank/design-document.md`、`memory-bank/implementation-plan.md`、`memory-bank/architecture.md` 和 `memory-bank/project-memory.md`。
- T0 顺序为：契约冻结、指标基线、拉取恢复契约、Notification Port、前台 WebSocket、Gateway 与多 Edge/Worker、后台平台通知、故障/容量门禁。持久消息 Broker 不作为默认 T0 依赖。
- 本轮只修改文档，未实现 WebSocket、Gateway、多 Edge 或平台推送；因此没有新增功能验证结果，也没有执行 Docker/Compose 验证。

## 2026-09-02 F2 前端真实 API 业务闭环源码落地

- 在 `/frontend` 创建正式 pnpm workspace、TypeScript strict、Vite、Vitest、ESLint 配置，以及 `admin-web`、`employee-web`、`employee-miniapp` 和共享 contracts/api-client/auth/task-domain/ui/mock 包。
- `admin-web` 已接入 Core Task List、Task Detail、Confirm、Cancel，任务 Scope 由 Core JWT/显式开发 Actor 决定，前端不本地扩大权限范围；Core 当前没有公开管理用户登录签发路由，页面保留 JWT/SSO 可替换适配点。
- `employee-web` 已接入 Edge 工号密码登录、长期会话恢复（Access/Refresh/Me/Logout）、本人 Projection、Accept/Complete、客户端稳定 command ID 和 `GET /api/v1/commands/{commandID}` 状态轮询；202 只展示 pending/syncing，不直接改写业务状态。
- `employee-miniapp` 已加入原生 `wx.request`、微信存储和共享 `SessionManager` 适配层；真实 AppID、通信域名、Provider code 回调和页面注册仍待平台配置。
- 新增 `frontend/docs/api-integration.md`，说明本地 Vite `/core-api`/`/edge-api` 代理、认证限制、API 边界和待接入项；静态 Mock 继续保留在 `frontend/preview/`，不与正式源码混用。
- 当前环境未发现 Node/npm/pnpm，正式依赖安装、前端 lint/typecheck/unit/build/E2E 尚未运行。代码变更后已执行 `git diff --check`，未发现 whitespace error；本轮尚未执行需要 Docker 前置的后端/数据库/Compose 联调。

## 2026-09-02 T0-1 可观测性实现完成

- 新增无强制第三方依赖的 Prometheus 文本指标 Registry，接入 Core API、Edge API 和 Worker 的 `/metrics`；Worker 指标服务仅监听 `/metrics` 路径。
- 新增 Core Outbox 与 Edge Command/Inbox 的持久化 backlog、failed count、oldest age 和 Projection lag 统计；新增 HTTP、数据库连接池和 Worker 投递/处理/确认延迟指标。
- 新增 observability、Core/Edge sync store、Worker metrics 测试；未改变可靠同步、幂等、版本收敛、Retry 和 Failed 语义。
- 验证事实：Docker 前置检查通过；受影响测试、`go build ./...`、运行时 Core/Edge/Worker metrics smoke 和 `scripts/verify.ps1 -Mode all` 通过。运行时 smoke 使用临时无数据库写入配置，未启动或修改现有业务 Compose 服务。
- 当前未宣称代表性业务负载下的性能基线；p50/p95/p99 报告待固定业务负载和数据规模后生成。下一步是 T0-2 拉取恢复契约。

## 2026-09-02 F2 前端源码收尾核查

- 补充了员工命令、管理端 Confirm/Cancel 的客户端幂等键复用：网络失败后的同一逻辑重试复用原 ID，业务内容变化或 409 冲突时重新建立新操作；客户端 ID 采用 UUID 形态以符合 Edge/Core OpenAPI。
- API Client 增加对已提前取消请求的处理；小程序请求显式声明 JSON Content-Type；正式 Web 不再跳转旧版预览页面。
- 已执行 `scripts/ensure-docker.ps1`，但当前环境找不到 `docker.exe`，以退出码 1 失败；Node、npm、pnpm、tsc、deno 均未发现，因此未运行前端依赖安装、lint、typecheck、unit、build、E2E 或在线联调。
- 源码级 JSON 配置核查通过，`git diff --check` 通过（仅有 LF/CRLF 格式提示），未发现正式源码中的 `require()`、`transition: all`、旧版预览跳转或内部同步 API 调用。

## 2026-09-02 Docker 环境恢复后的统一验证

- 用户确认 Docker Desktop 已打开；提升权限检查确认 Docker Engine 就绪，路径为 `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`。
- 已运行 `scripts/verify.ps1 -Mode all` 并通过：`go test ./...`、`go build ./...`、Docker Compose config 和 `git diff --check`。
- 已运行只读 `scripts/verify.ps1 -Mode compose-ps` 并通过；当前没有运行中的项目 Compose 服务，未自动启动、停止或清理任何容器。
- 前端 Node/npm/pnpm/tsc 仍未发现，因此前端依赖安装、lint、typecheck、unit、build、E2E 和浏览器联调仍未运行。

## 2026-09-02 前端 Node 环境配置与构建验证

- 用户授权下载并配置前端环境。Node.js 官方 Windows x64 v24.20.0 MSI 下载后因 Windows Installer 返回 1603 未采用；随后使用同版本官方 Windows x64 压缩包并校验 SHA-256，安装到当前用户目录 `AppData\Local\Programs\nodejs`。
- npm 11.19.0 和项目声明的 pnpm 9.15.0 已安装；Node 目录及 npm 用户目录已写入当前用户 PATH。前端依赖安装成功，生成 `frontend/pnpm-lock.yaml`。
- `pnpm lint`、`pnpm typecheck`、`pnpm test`、`pnpm build` 全部通过；单元测试为 1 个测试文件、3 个测试，admin-web 和 employee-web 生产构建成功。
- 本轮先修复了会话初始化的 TypeScript 循环推断问题，再通过全部前端检查。浏览器实际 API 联调、完整 E2E、小程序平台构建和真实微信/企业微信 Provider 仍待完成。

## 2026-09-02 T0-2 拉取恢复契约实现

- 保留 Edge 员工 JWT Principal 过滤的完整 `GET /api/v1/tasks`，响应增加 `snapshot_at`、`sync_mode=full_snapshot`、员工级 `projection_revision`、Projection lag 状态、`next_cursor` 和 `reset_required`。
- 新增 Edge MySQL `000004_employee_projection_cursor` 及 Memory/SQL Store 支持；Projection 新增或高版本更新推进员工 revision，同版本/过期重放不推进，转派同时推进原员工和新员工。
- 冻结客户端语义：HTTP 200 空数组是合法空结果；取消/完成使用 Projection 终态；启动、刷新、重连、推送提示后的校准和离线恢复重新拉取完整快照；Command 通过稳定 ID 和公共 status API 恢复；当前不启用增量 cursor。
- 验证事实：已先运行 `scripts/ensure-docker.ps1`；受影响测试通过，随后 `scripts/verify.ps1 -Mode all` 通过，包含 `go test ./...`、`go build ./...`、Compose config 和 `git diff --check`。000004 migration 未自动应用，未执行破坏性数据库操作。

## 2026-09-02 T0-3 通知抽象和提交后边界实现

- 新增 `internal/edge/application/notification`：提供员工隔离的 `TaskChanged`、`Publisher`、`Sink` 和单实例 `InMemoryFanout`；员工 ID 仅用于内部路由，不进入通知 wire payload；单连接失败会继续尝试其他连接。
- Edge `/internal/sync/v1/events` 先执行 `ApplyEvent`，只有投影成功且 Event 非重复时才构造并发布任务变化提示；通知失败只记录结构化日志和 `flight_notification_delivery_total`，不回滚 Projection、不改变事件已应用的 `202`。
- 连接注册表是内存 best-effort 状态，不承担可靠消息存储；断线、重启、丢失和跨副本未命中由下一次 `GET /api/v1/tasks` 完整快照与 `projection_revision` 恢复。
- 定向验证已通过：`go test ./internal/edge/application/notification ./internal/edge/application ./internal/platform/observability`；初次测试发现并修复了测试夹具对任务列表顺序的错误假设。文档同步后最终 `scripts/verify.ps1 -Mode all` 也已通过，包含全量 Go 测试、构建、Compose config 和 `git diff --check`。

## 2026-09-02 T0-4 员工 WebSocket 提示实现

- Edge 新增 `POST /api/v1/realtime/ticket` 和 `GET /api/v1/ws`：ticket 由员工 Bearer JWT 签发，30 秒内单次消费，浏览器通过 `Sec-WebSocket-Protocol` 传递；服务端只协商 `flight.realtime.v1`，不把 ticket 放入 URL 或响应协议。
- `internal/edge/application/realtime` 已接入 Notification Port，完成同源和协议校验、员工 Principal 绑定、`ready`/`ping`/`pong`、读写超时、空闲关闭、连接清理和 WebSocket 指标。通知失败/连接丢失不影响 Projection，完整 HTTP 快照仍是恢复路径。
- `employee-web` 新增 `RealtimeClient`，自动获取 ticket、指数退避重连、回复心跳、按 `notification_id` 有界去重，并在连接成功、重连和 `task_changed` 后重新拉取任务快照；ticket 401 停止重连并进入认证恢复路径。
- Go 定向测试已通过：`go test ./internal/edge/application/realtime ./internal/edge/application ./internal/platform/httpauth ./internal/platform/observability`。前端 `typecheck`、源码 `lint`、`test`（2 个测试文件、5 个测试）和 `build`（admin-web、employee-web）已通过；首次 lint 失败是并行 build 生成的 `dist` 被递归扫描，已将 ESLint 忽略规则修正为 `**/dist/**`/`**/node_modules/**` 后复核通过。
- 真实浏览器联调、服务重启/网络分区和多副本 fan-out 仍未验收；文档更新后的 `scripts/verify.ps1 -Mode all` 已通过。下一步进入 T0-5 Gateway 与共享 Edge 多副本。

## 2026-09-02 F2 真实后端联调与 UTF-8 编码收敛

- 按项目规则先执行 Docker 前置检查；隔离 Compose 项目 `frontend-live` 的 Core/Edge MySQL 保持健康，既有 `flight-mysql`、`flight-redis` 未触碰。当前源码 Core API `:8081`、Edge API `:8082`、Worker 已完成真实到达生成、主任确认、Outbox/Worker 投递、Edge Projection、员工工号密码登录、Refresh/Me、Accept、Complete、Command status 和重复命令闭环。
- 员工 Web `http://localhost:4175/login` 已用隔离测试账号完成真实浏览器登录，并读取真实 Edge Projection。登录页静态中文和任务列表动态中文均显示正常；本次先发现隔离 fixture 中中文已被错误写成 `è”...`，随后使用明确 UTF-8 原始字节修复 Core/Edge 隔离测试数据，未改变业务结构或删除数据。
- 诊断确认 Edge API 响应头为 `application/json; charset=utf-8`，MySQL session/连接和表为 `utf8mb4`；Windows 终端曾出现的乱码与 API 数据污染是两个独立问题。Web `ApiClient`、微信小程序 `wx.request`、Edge→Core Identity HTTP、Worker→Edge Sync HTTP 和测试请求头已统一显式使用 `application/json; charset=utf-8`。
- 新增 `frontend/packages/api-client/src/client.test.ts` 覆盖中文 JSON 请求/响应；前端 `pnpm lint`、`pnpm typecheck`、`pnpm test`（3 文件/6 测试）、`pnpm build` 通过；Go `go test ./...`、`go build ./...`、`git diff --check` 通过。完整 Playwright E2E、真实微信/企业微信 Provider、管理端 SSO、WebSocket 真实握手和性能门禁仍待后续阶段。

## 2026-09-02 员工 Web Playwright 与 WebSocket 浏览器验收

- 新增 `frontend/playwright.config.ts` 与 `frontend/e2e/employee-web.spec.ts`；账号密码只从 `E2E_EMPLOYEE_NO`、`E2E_EMPLOYEE_PASSWORD` 环境变量读取，测试默认使用 `chrome` 通道，不把临时凭据写入仓库。
- E2E 首次只验证登录和 Projection 时通过；加入 WebSocket `ready` 断言后发现 Vite 代理 `changeOrigin=true` 破坏 Edge 的同源 Host/Origin 校验。将员工 Web `/edge-api` 代理设为 `ws=true`、`changeOrigin=false`，并按 UTF-8 解码 Playwright WebSocket 帧。
- 按项目规则先执行 `scripts/ensure-docker.ps1`；最终 `pnpm e2e` 通过（1 个场景，真实员工登录、UTF-8 中文 Projection、浏览器 WebSocket `ready`）。Edge 指标确认握手 `accepted=1`，连接断开由测试上下文结束产生的读错误，不影响握手验收。
- 当前仍未完成 WebSocket 服务重启/网络分区、多副本 fan-out、异常恢复和 Command 生命周期 E2E；标准 Compose 镜像构建仍受 Docker Hub 基础镜像 EOF 影响，当前验证使用隔离数据库容器加当前源码进程。

## 2026-09-02 员工端会话恢复、连接状态与视觉回归

- `employee-web` 顶部连接提示改为由 `RealtimeClient` 的真实状态驱动，覆盖 connecting/open/retrying/closed/unauthorized；ticket 401 时清理员工会话并回到登录页，不再静态显示“已连接 Edge”。
- 新增 `packages/auth/src/index.test.ts`，覆盖 Access Token 失效后的 Refresh 恢复、Refresh 失效后的安全清理和 binding ticket 与正式会话隔离；前端单元测试现为 4 个文件、9 个测试。
- Playwright 员工 Web 场景增加页面刷新校验，验证 localStorage 会话恢复后仍能读取中文 Edge Projection；本轮 `pnpm e2e` 通过 1 个场景。
- 首轮视觉回归已用本机 Chrome 检查员工登录页和真实任务页，收口了连接状态色、航班识别条、任务卡层次、触控尺寸和管理端表格/操作卡细节；最终品牌视觉仍待产品评审。
- 按规则重新执行 Docker 前置检查并通过；`pnpm lint`、`pnpm typecheck`、`pnpm test`、`pnpm build` 和 `git diff --check` 均通过。完整 401/403/404/409/503、Command 生命周期、服务重启/网络分区、多副本 fan-out、真实平台 Provider 和管理端 SSO 仍待后续验收。
## 2026-09-02 T0-5 Gateway 与多副本实现

- 已增加 `deployments/local/gateway/nginx.conf`、`gateway`、`edge-api-2`；Edge 宿主机固定端口移至 Gateway，Worker 同步地址改为 `http://gateway`，两个 Edge 使用同一 Edge MySQL。
- Core `outbox_event` 与 Edge `mobile_command` 增加持久化 `lease_owner`/`lease_expires_at` migration；Worker、HTTP Transport、Edge pending/ack 路由已贯穿 owner 和租约时长。
- realtime ticket 增加共享 Edge SQL 存储（仅保存 hash、一次性消费），Redis Pub/Sub 增加跨 Edge best-effort fan-out；Redis/通知失败不影响 Projection 和 HTTP 快照恢复。
- 新增 Core/Edge 租约测试；本轮 `go test ./...` 在使用临时 `GOCACHE` 后通过。按规则执行 `scripts/ensure-docker.ps1`，但当前环境找不到 `docker.exe`，所以 Compose 配置、迁移应用、双副本/Redis 故障注入尚未验证，不能写成已通过。

## 2026-09-02 F3 员工任务生命周期、角色 Scope 与刷新恢复 E2E

- 先执行 `scripts/ensure-docker.ps1`，确认 Docker Engine 就绪；在隔离 Compose 项目 `frontend-live` 上新增一条 scheduled 测试航班 `FL-LIVE-002`，通过真实 Core 到达接口生成任务、主任 Confirm、Worker Outbox/Inbox 同步到 Edge。既有 `FL-LIVE-001` 和 `flight-*` 容器未清理。
- `employee-web` 新增 Command 收据存储：发起 Accept/Complete 前先保存 `command_id`、Assignment、期望 Projection 版本和操作类型；收到 202 后保存状态，刷新后按同一 ID 查询；confirmed/failed/409 后分别清理或重新建立，不把 localStorage 当可靠消息队列。
- Command 状态查询遇到临时网络/503 时最多自动重试 15 次；重试耗尽仍保留收据，刷新页面可以继续恢复查询，不把异常误报为业务完成。
- 新增 `frontend/e2e/employee-task-lifecycle.spec.ts`、`frontend/e2e/core-task-scope.spec.ts` 和 `frontend/e2e/edge-error-contract.spec.ts`。真实隔离 E2E 已通过：基础员工登录/中文 Projection/WebSocket ready/刷新恢复、主任全局/正确 Scope 队长/错误 Scope 404、Accept→Command confirmed→Complete→刷新恢复已完成、Edge 401/403/404/409 错误契约。
- `frontend/packages/api-client/src/client.test.ts` 增加 401 回调和非 JSON 503 稳定错误映射；新增 `TaskCommandStore` 2 个单元测试。前端 `pnpm test` 通过（5 个文件/13 个测试），`pnpm lint`、`pnpm typecheck`、`pnpm build` 通过。
- 首次并行 E2E 因两条同名任务使基础用例标题断言触发 Playwright strict mode 失败，已改为按航班号限定任务卡；修复后基础场景单独复跑通过。该首次失败保留为过程事实，未伪称全套一次性通过。
- 仍待：可重复创建的 CI 隔离夹具、503 浏览器级故障注入、服务重启/网络分区、多副本 fan-out/容量门禁、真实微信/企业微信 Provider、管理端 SSO 和最终品牌视觉评审。
## 2026-09-03 P0 管理端身份、会话与管理查询代码落地

- Core 新增管理端 SSO/会话边界：`admin_identity`、一次性 `admin_sso_state`、可撤销 `admin_session`，以及 OIDC authorization-code / 显式 development Provider 适配器。State 使用 redirect allow-list、HttpOnly cookie、哈希保存和 MySQL 一次性消费；Provider token 不下发浏览器；每次管理请求从 Core session 恢复当前角色与 Team/Area/User scope。
- 新增 Core `GET /api/v1/personnel` 和 `GET /api/v1/assignments` 只读接口，Service 强制执行 RBAC 和 principal-derived scope，MySQL adapter 负责参数化查询；补齐 `api/core/openapi.yaml` 与前端 contracts/API client 类型。
- 新增 Core `migrations/core/mysql/000005_admin_sso.up.sql`/`.down.sql`、`docs/adr/ADR-011-admin-sso-and-management-read-model.md`，并增加 adminauth/adminquery 单元测试。
- 本轮 Docker 前置检查成功，CLI 为 `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`。第一次全量测试因默认 Go cache 权限被拒绝退出 1，切换到被 `.gitignore` 忽略的 `.tmp\\gocache-handoff` 后 `go test ./...` 与 `go build ./...` 均成功。
- 最终 `scripts/verify.ps1 -Mode all` 通过，包含全量 Go 测试、构建、Compose config 和 `git diff --check`。Core `000005_admin_sso` 尚未应用，真实 OIDC/IAM、管理员预置、管理端完整业务 UI、双副本故障和性能门禁仍待后续。
