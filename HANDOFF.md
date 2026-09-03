# Task Handoff

> 更新时间：2026-09-03
> 任务：项目级并行交接总览
> 说明：本文顶部是当前全局索引；下方历史章节保留旧交接证据，不作为当前状态的唯一来源。各任务的最新快照位于 `memory-bank/handoffs/`。

## 当前全局交接索引（2026-09-03）

`HANDOFF.md` 是项目级总览，不再承载前端、后端和基础设施的全部详细过程。新会话应先读取对应任务快照，再按任务需要读取 `memory-bank/progress.md` 的相关里程碑。

| 范围 | 当前状态 | 最新交接文件 |
|---|---|---|
| 前端 | F3 员工任务生命周期、Core 角色 Scope、Command 收据刷新恢复和 UTF-8 错误契约已完成；真实 Provider 适配器、原生小程序页面壳和管理端企业微信 SSO callback 已加入；生产凭据/域名、员工/管理员主数据预置、故障注入和最终品牌评审仍待完成 | [`memory-bank/handoffs/frontend.md`](memory-bank/handoffs/frontend.md) |
| 后端 | BVS2-07、T0-1、T0-2、T0-3、T0-4、T0-5 已有代码/迁移；真实个人微信/企业微信 Provider 与管理端企业微信 SSO 服务已加入并默认关闭，Go 测试通过；生产凭据/域名、员工/管理员主数据预置和双副本 Compose/迁移/故障验收仍待补 | [`memory-bank/handoffs/backend.md`](memory-bank/handoffs/backend.md) |
| 基础设施 | Foundation/BVS2-06 隔离闭环已有验证；标准镜像构建和性能门禁仍需推进 | [`memory-bank/handoffs/infrastructure.md`](memory-bank/handoffs/infrastructure.md) |

### 交接更新规则

- 用户表达“我要结束当前这个对话”“我要退出当前对话”“准备开新线程”等明确结束会话语义时，先更新本次任务对应的交接快照，再更新本索引。
- 交接快照采用滚动更新，不无限追加重复内容；历史过程追加到 `memory-bank/progress.md`，过长后按阶段归档。
- 同时进行多个任务时，各会话只更新自己负责的任务文件；跨范围内容分别同步，不覆盖其他会话的未提交修改。
- 修改前必须重新读取目标交接文件、当前索引和 `git diff`；发现并发修改时先合并事实。测试或构建未运行时，必须明确写成“未运行”。

### 2026-09-02 F2 真实员工端联调与 UTF-8 收敛

- 隔离 Compose 项目 `frontend-live` 的 Core/Edge MySQL 使用当前源码 Core API、Edge API 和 Worker 完成到达生成、主任确认、Projection 同步、员工工号密码登录、Refresh/Me、Accept、Complete、Command 状态和重复命令验证；既有 `flight-*` 容器未触碰。
- 员工 Web 浏览器已实际登录 `http://localhost:4175/login` 并读取 Edge Projection；任务中文在修复隔离测试数据后正常显示。Edge JSON 响应头为 `application/json; charset=utf-8`，MySQL 连接/表为 `utf8mb4`。
- Web/API Client、微信小程序 `wx.request`、Edge→Core Identity HTTP、Worker→Edge Sync HTTP 的 JSON 请求头已统一声明 UTF-8；前端新增中文请求/响应测试，员工 Web Playwright E2E 已通过。个人微信/企业微信真实平台联调、员工/管理员主数据预置、完整异常场景 E2E 和视觉评审仍未完成。

### 2026-09-01 文档清理结果

- 已删除明确过时且不再作为实现依据的旧 B3 业务切片说明和根目录旧设计草案；当前以 `memory-bank/`、`docs/architecture/`、`docs/adr/` 和前端 F0 文档为准。未跟踪的 `memory-bank/architecture-design.md` 仍保留，删除需用户对该具体文件再次确认。
- 旧 `cmd/server`、`internal/model`、`internal/store`、`internal/module` 及 `migrations/mysql` 代码/迁移现场仍保留为 legacy/paused，未删除业务代码或数据库迁移。

## 当前目标（已完成）

冻结并实现：单仓库、内网 Core 模块化单体、云端 Edge 接入服务、Core/Edge 独立数据库、Core 唯一业务事实源、Edge Projection、可靠双向同步、安全/IAM 基础、可观测性、HA/DR 扩展边界和 Architecture Probe。

Foundation 容器级验收已完成并冻结；本轮没有开始 B3/B4/B5 真实航班、任务、人员、事件业务实现。

## A1 审计结果

- 仓库根目录：`C:/Users/yuanfengleeeeee/Desktop/flight-collaboration-platform`
- 当前分支：`agent/foundation-and-handoff`
- 远程：`origin` 指向项目 GitHub 仓库；本轮不 push、不 commit、不创建 PR。
- 工作区在本轮开始前已有未提交修改：`internal/common/response.go`、`internal/model/event.go`、`internal/model/flight.go`、`internal/server/router.go`、`internal/server/router_test.go`。
- 工作区在本轮开始前已有未跟踪内容：`internal/module/event/`、`memory-bank/architecture-design.md`、`migrations/mysql/000003_task_instance_trigger_event_unique.up.sql`、`migrations/mysql/000003_task_instance_trigger_event_unique.down.sql`。
- 上述 B3 现场属于已有修改，本轮保留、不回滚、不覆盖、不继续扩展。旧 `cmd/server`、`internal/model`、`internal/store`、`migrations/mysql` 作为 legacy/paused 现场；新 v2 入口不依赖其业务路由。
- 预先存在的未跟踪 `memory-bank/architecture-design.md` 是旧 v1 架构草案；本轮因保护用户未提交文件而未编辑，已在 `memory-bank/architecture.md` 标为 legacy/paused，不能作为 v2 规范。

## A2 已完成

- 新增 `docs/architecture/architecture-v2.md`。
- 新增 `docs/adr/ADR-001` 至 `ADR-008`，全部标记 Accepted。
- 同步 `AGENTS.md`、`README.md`、根 `design-document.md`/`tech-stack.md` 索引、`memory-bank/design-document.md`、`memory-bank/tech-stack.md`、`memory-bank/implementation-plan.md`、`memory-bank/architecture.md`、`memory-bank/progress.md`。
- 已冻结单机场边界、Core/Edge 数据所有权、独立数据库、Event/Command 版本化、Outbox/Inbox、Human/Machine Principal、RBAC + Scope、Redis 和 HA/DR 目标。

## 当前结果（2026-08-10 Docker 恢复前历史记录）

- A3-A9 已有实际代码和测试：新目录/入口、双 migration schema、Core/Edge health、Outbox/Inbox/Command Store、Retry/Failed、IAM、RBAC/Scope、Audit、双向 Probe 和故障测试均已存在。
- `go test ./...`：通过；`go build ./...`：通过；Core/Edge 启动探测：`live=200 ready=503`。
- `git diff --check`：通过（只有 LF→CRLF 提示）。
- 最终收口已再次确认 Core/Edge Redis 配置独立 optional、MySQL DSN 使用 UTC session 时区；修改后 `go test ./...` 和 `go build ./...` 仍通过。
- Docker/真实 MySQL 仍是环境阻塞：PowerShell 找不到 `docker`；Core migration status 连接 127.0.0.1:3310 被拒绝；Edge migration status 连接 127.0.0.1:3311 被拒绝。因此不能声称 Compose、实际双库 migration 或容器内 Probe 通过。
- 临时 `.tmp` Go cache/二进制已删除；未执行 destructive 操作，未停止宿主机 MySQL。

## 下一次继续（已由最终验收关闭）

1. Docker CLI/服务可用后，仅运行 `docker compose -f deployments/local/docker-compose.yml config` 和 `up`，确认 Core/Edge 两个 MySQL healthy。
2. 分别运行 `go run ./cmd/migrate -target core -config configs/config.v2.yaml -command up` 与 Edge 等价命令，记录 status。
3. 运行容器/HTTP 版双向 Probe，并将真实结果追加到 `memory-bank/progress.md`。
4. 在上述验证补齐前，不开始 B3/B4/B5 真实业务。

## 验证纪律

本交接不把旧 HANDOFF 中的 Go/Docker 结果视为当前结果。当前任务后续必须重新运行：

```text
gofmt
go test ./...
go build ./...
git diff --check
git status
```

## 2026-08-10 A10 重新验证补充（历史状态，已由 2026-08-13 验收记录覆盖）

- Docker Desktop 已确认正在运行：用户目录下的 CLI 为 `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`；Docker Engine `29.6.2`、Compose `v5.3.1`，`docker compose -f deployments/local/docker-compose.yml config` 已通过。
- 本轮新增 `.dockerignore`，排除约 1GB 的本地临时 Go cache/MySQL 数据，避免 Docker 构建上下文污染。
- Docker 三个 v2 镜像（`local-core-api`、`local-edge-api`、`local-worker`）已实际构建成功；Compose 创建了 `local_default`、`local_core_mysql_data`、`local_edge_mysql_data` 和 v2 容器。
- 容器启动阶段 Docker Desktop Linux Engine 返回 API 500：`containers/.../json` 和后续 `containers/json` 请求失败，之后 `docker version/info/ps` 超时。未执行 `docker compose down -v`、volume 删除、migration down、TRUNCATE、DROP DATABASE，也未修改旧原型的 `flight-mysql`（3307）和 `flight-redis`（6379）。因此 Docker 容器健康、容器内 migration 和容器内 Probe 仍不能标记为通过；不擅自重启 Docker Desktop。
- 为完成可重复的真实数据库验证，曾使用本机 MySQL 8.0.35 在仓库临时目录创建两个独立实例（3310/3311），不触碰宿主机 3306。Core/Edge migration 分别执行 `pending -> up -> applied`；Core API、Edge API、Worker 真实进程运行时，两个 readiness 均为 200，细节为 `mysql=ok`、`redis=unavailable`。
- `FLIGHT_RUN_DB_PROBE=1 go test ./internal/integration/sync -run TestArchitectureProbeAgainstRunningServices -count=1 -v` 在上述临时双库和真实进程上通过（Core SQL transaction -> Outbox -> HTTP Edge Inbox/Projection，以及 Edge HTTP Command -> Worker pull -> Core Inbox -> Outbox -> Projection）。随后新增的 SQL failed-record/retry 断言已由 Memory/全量 Go 测试覆盖；更新后的 opt-in SQL Probe 尚未在 Docker Engine 恢复后重跑。
- 最新代码回归：`go test ./...` 通过，`go build ./...` 通过，`git diff --check` 通过。临时运行目录已删除并加入 `.gitignore`。A10 只剩 Docker Engine 恢复后的容器级补跑，B3/B4/B5 仍未开始。

Docker 可用时再运行 `deployments/local` Compose、双库 migration、Core/Edge health 和 Probe；Docker 不可用则记录真实阻塞。不得执行 `docker compose down -v`、`TRUNCATE`、`DROP DATABASE`、破坏性 migration down 或停止本机已有 MySQL。

## 2026-08-13 A10 Docker 容器级最终验收（最终状态）

Architecture v2.0 Foundation 已完成最终容器级验收，状态为：

```text
Architecture v2.0 Foundation: ACCEPTED / FROZEN
```

- Docker Desktop 已恢复：Docker `29.6.2`、Compose `v5.3.1`、context `docker-desktop`；`docker info` 和 Compose 配置校验通过。
- 使用独立 Compose project `architecture-v2-final` 启动 `core-mysql`、`edge-mysql`、`core-api`、`edge-api`、`worker`。Core/Edge 使用新建且相互独立的 MySQL 容器和 volume；旧 `local_*` volume、旧 `flight-mysql`、旧 `flight-redis` 均未删除或修改。
- Core migration 与 Edge migration 分别执行 `pending -> up -> applied`，没有跨库连接或共享 migration。
- Core/Edge `/health/live` 与 `/health/ready` 全部返回 200；readiness 为 `mysql=ok`、`redis=unavailable`，符合 Redis optional 约定。
- 容器 SQL/HTTP Architecture Probe 通过：Core transaction -> Outbox -> Worker -> Edge Inbox -> Projection -> Edge API，以及 Edge Command -> Core pull -> Core Inbox -> Core transaction -> Outbox -> Edge Projection。
- 重复 Event、重复 Command、Inbox failed record/retry 均通过；短暂停止 Worker 后重启，Pending Outbox 恢复处理；短暂停止 Edge API 后恢复，Outbox 进入重试并成功补发。
- 最终质量检查：`go test ./...` 通过，`go build ./...` 通过，`git diff --check` 通过；v2 相关 48 个 Go 文件 `gofmt` 检查通过。旧 legacy 文件的既有格式差异未被本轮擅自重写。

本轮没有开始 B3/B4/B5 真实业务，也没有引入 Kafka、Kubernetes、微服务拆分或其他架构复杂度。后续可以在本冻结基线上另开业务实施阶段；本交接不宣称任何生产 HA/RPO/RTO 目标已经在开发环境达成。验收 Compose 容器和 volume 保持运行，供复核使用。

## Phase 2 当前交接（2026-08-28，历史记录；已由下方 2026-08-31 交接覆盖）

项目已正式从 Foundation 验收切换到：

```text
Phase 1  Architecture Foundation      ACCEPTED / FROZEN
Phase 2  BVS2-01 业务设计              FROZEN
Phase 2  BVS2-02 Core 数据与 Migration COMPLETED
Phase 2  BVS2-03 Flight → Task → Candidate COMPLETED
```

`Business Slice v2 — Flight → Task → Personnel → Leader Confirm → Edge → Employee → Complete` 的 BVS2-01 设计入口为 `memory-bank/business-slice-v2-flight-task.md`，已完成冻结；BVS2-02 数据基线和 BVS2-03 Flight → Task → Candidate 已完成，下一项正式任务是 `BVS2-04 Leader Confirm`。

BVS2-01 已冻结六组交付物：业务规则、Flight/Task/Candidate/Assignment/Personnel/Projection/Command 状态机、RBAC + Scope 权限、Core/Edge Event/Command 契约、异常与补偿、BVS2-AT-01 至 BVS2-AT-30 可执行验收场景。已明确 Flight 只做触发事实、Candidate 与 Assignment 分离、Personnel 由自身 Application/Domain 持有状态、员工 Accept/Complete 必须是 Edge Command，以及同 ID/不同 ID 的幂等和内容冲突语义。

BVS2-02 已新增 `migrations/core/mysql/000002_core_business_slice_v2.up.sql` 及对应 down 文件，建立 14 张 Core 业务事实、组织前置、状态历史和业务幂等结果表。Core Docker DB 的 `schema_migrations` 为 `1,2`，重复执行 `up` 后仍为 `1,2`；Edge Docker DB 保持独立且只有 `000001 edge_foundation`。BVS2-03 已在此数据基线上实现 Flight/Task/Candidate Core 业务用例和 API；Assignment、Edge Projection 和员工 Command 仍未实现，也未恢复旧 B3 现场。

后续实现必须让业务适应已冻结架构：Core 是唯一事实源，Edge 只保存最小 Projection/Command，员工操作经 Core Worker 和 Core 状态机处理；所有关键 Core 写入继续与 Audit/Outbox 保持同一事务。

本段 Docker 结果是 2026-08-28 的历史记录：当时 Core/Edge MySQL、API 与 Worker 均完成 Foundation 验证。最新 BVS2-03 的 Docker 状态和实际命令结果以下方 2026-08-31 交接为准。

## Phase 2 BVS2-03 最新交接（2026-08-31）

项目状态：

```text
Phase 1  Architecture Foundation      ACCEPTED / FROZEN
Phase 2  BVS2-01 业务设计              FROZEN
Phase 2  BVS2-02 Core 数据与 Migration COMPLETED
Phase 2  BVS2-03 Flight → Task → Candidate COMPLETED
Phase 2  BVS2-04 Leader Confirm         READY
```

BVS2-03 已在 `internal/core/application/flighttask` 和 `internal/core/adapter/mysql` 实现：Core 接收 Flight 到达事实，仅允许 `scheduled → arrived`；在同一事务中更新 Flight 与状态历史、解析最高启用模板、创建 `awaiting_confirmation` Task、生成 Candidate 快照，并写入业务幂等记录、Audit 和 `task.generated.v1` Outbox。候选规则和排序遵循冻结的 Team/Area、主成员、岗位、能力、idle、启用和计划时间冲突约束；Candidate 仍不等于 Assignment。

Core API 新增 `POST /api/v1/flights/{flightPublicID}/arrival`，并同步更新 `api/core/openapi.yaml`。新增测试覆盖正常生成、候选过滤/排序、重复 source event、来源内容冲突、同 generation key 重放、无模板、非法状态和稳定 HTTP 错误；非法状态的拒绝结果重放仍返回原业务错误。

验证结果：

- `go test ./...` 实际通过；隔离 Docker Core MySQL `127.0.0.1:3310` 的 `FLIGHT_RUN_BVS2_DB_TEST=1 go test ./internal/core/adapter/mysql -run TestFlightTaskSQLAgainstDocker -count=1 -v` 实际通过。
- Docker Engine `29.6.2` 和 Compose 配置检查通过；Core/Edge MySQL 现有独立卷均为 `healthy`。本轮应用镜像重建因 Docker Hub 拉取 `golang:1.25` / `debian:bookworm-slim` 返回 `EOF` 未完成，因此未宣称新 BVS2-03 应用镜像容器验证通过。
- 没有执行 `docker compose down -v`、DROP、TRUNCATE、破坏性 migration down，也没有停止或修改旧 `flight-mysql`（3307）/`flight-redis`（6379）。

下一步只进入 `BVS2-04 Leader Confirm`；在此之前不实现 Edge Projection、Employee Accept/Complete 或旧 B3/B4/B5 现场。

## 本次会话结束交接（2026-08-31）

- 本次会话已完成 BVS2-03 收尾。当前正式状态为 `Architecture v2.0 Foundation: ACCEPTED / FROZEN`、`Phase 2 Business Implementation: IN PROGRESS`；BVS2-01 为 `FROZEN`，BVS2-02 与 BVS2-03 为 `COMPLETED`，下一项为 BVS2-04 `Leader Confirm`。
- 最终检查结果：`gofmt` 已执行；`go test ./...` 通过；`go build ./...` 退出码为 0。构建仍输出一次宿主 Go module stat cache 的 `Access is denied` 警告，但不影响退出成功；`git diff --check` 退出码为 0，仅有 LF→CRLF 提示。
- Docker Desktop 通过 `docker.exe` 验证，Engine Client/Server 为 `29.6.2`；Compose `config --quiet` 通过。当前 `architecture-v2-final-core-mysql-1`（3310）和 `architecture-v2-final-edge-mysql-1`（3311）均为 `healthy`。
- 真实数据库状态：Core `000001 core_foundation`、`000002 core_business_slice_v2` 均为 `applied`；Edge `000001 edge_foundation` 为 `applied`。隔离 Docker Core MySQL 上的 BVS2-03 SQL 集成测试通过，验证真实 Flight/Task/Candidate 写入和重复请求无重复副作用。
- 本轮再次执行 `docker.exe compose -p architecture-v2-final -f deployments/local/docker-compose.yml up -d --build`，应用镜像仍因 Docker Hub 请求 `golang:1.25` 和 `debian:bookworm-slim` 返回 `EOF` 未重建；不要把新 BVS2-03 应用镜像容器验证写成已通过。此前已用新代码连接 Docker Core MySQL 做过 Core API 临时进程验证：路由已注册，live/ready 均为 200；临时进程已停止，数据库容器保留运行。
- 没有执行 `docker compose down -v`、DROP、TRUNCATE、破坏性 migration down，也没有停止或修改旧 `flight-mysql`（3307）/`flight-redis`（6379）。没有 commit、push、切换或删除分支。
- 工作区仍包含本任务及之前任务的未提交/未跟踪差异，下一会话必须先按 `AGENTS.md` 重新检查 `git status`、`git diff`、未跟踪文件和关键文档，不能覆盖已有修改。继续工作时从 BVS2-04 Leader Confirm 开始，保持 Core 唯一事实源、Candidate/Assignment 分离和 Core/Edge 边界；不要恢复旧 B3/B4/B5。

## 长期记忆与 Docker 验证前置（2026-08-31）

- 新增 `memory-bank/project-memory.md` 作为跨会话长期记忆；长期协作规则同步写入 `AGENTS.md`。
- 新增 `scripts/ensure-docker.ps1` 和 `scripts/verify.ps1`。功能、集成、数据库和 Compose 验证前会自动检查 Docker；Docker Desktop 未运行时会自动启动并等待 Engine 就绪。
- 已实际验证 Docker 自动启动链路：脚本启动 Docker Desktop 后 Engine 就绪；统一验证入口的 Go 测试、构建、Compose 配置和 `git diff --check` 均通过。
- 该机制不会自动执行 `compose up`、Migration、Volume 删除、TRUNCATE、DROP 或其他破坏性操作。

## BVS2-05 Edge Projection → Employee Command 最新交接（2026-09-01）

当前状态：BVS2-05 代码与隔离数据库验证已完成；完整 Compose 服务闭环、认证收敛和取消/CRUD 仍属于 BVS2-06，尚未开始。

- Edge 已接入 `task.assigned.v1`、`task.accepted.v1`、`task.completed.v1`、`task.cancelled.v1` 的最小 Task Projection 快照；Projection 按 `sync_version` 单调收敛，同版本冲突会失败并留在 Inbox failed。
- Edge 新增 `migrations/edge/mysql/000002_edge_business_slice_v2`，以及 `POST /api/v1/tasks/{taskPublicID}/accept`、`/complete`。员工 Actor 只从 `X-Employee-Public-ID` 读取，Command 先入 Edge Store。
- Core 新增员工命令状态机与 MySQL 事务适配：Accept 为 `assigned + confirmed + reserved → in_progress + accepted + busy`；Complete 为 `in_progress + accepted + busy → completed + completed + idle`。两条路径均写 Task/Assignment/Personnel History、Audit 和对应 Outbox Event。
- Worker 新增 `CommandProcessor`，生产模式下使用业务 Repository 执行同一 Core MySQL transaction；Foundation Probe 仍保持可用。
- Command 幂等比较已兼容 MySQL JSON 列自动规范化对象键顺序：相同 command ID、相同业务内容可重放；不同业务内容仍返回冲突。
- 定向 Go 测试已通过；隔离临时 Edge MySQL 的两个 migration 与 Projection 版本收敛/冲突 SQL 测试已通过；隔离临时 Core MySQL 的两个 migration 与 Accept→Complete SQL 事务测试已通过，覆盖 Core Inbox 幂等、三类 History、Audit 和 Outbox。
- `powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all` 已通过，包含 Docker 前置、`go test ./...`、`go build ./...`、Compose config 和 `git diff --check`。本轮临时容器已停止并清理；原有损坏 Core 数据卷未删除、未重置，未执行 `down -v`/清库。

进入 BVS2-06 的接手顺序：

1. `powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all`
2. 在独立 Compose 项目上补跑 Core→Worker→Edge 的完整服务闭环，并覆盖成功、重复、越权、非法状态和恢复路径。
3. 补齐正式 JWT/mTLS 入口、Task Cancel/CRUD 和员工端正式筛选后，再更新 API/OpenAPI 与最终验收记录。

不要删除或重置 `local_core_mysql_data`，不要恢复旧 B3/B4/B5，也不要让 Edge 直接写 Core 业务事实。

## 2026-09-01 前端员工端入口调整

- 首期前端同时建设管理端和员工端：管理端为桌面优先响应式 Web；员工端首期为微信小程序，同时支持个人微信和企业微信入口。
- 两类微信入口共用一套小程序页面、Edge API、Task Projection 和 Accept/Complete 语义；外部微信身份必须由后端映射到同一个 Staff，不能由客户端直接伪造员工身份。
- `employee-web` 保留为正式可用网页版和备用入口；`employee-miniapp` 尚未创建业务源码，设计文档仍为 DRAFT，后续 F1 先完成小程序壳层、网页版壳层和双入口 AuthAdapter。
- 小程序生产联调前必须确认 HTTPS API 域名、个人微信/企业微信登录接线、企业归属校验、客户端 Command 幂等键和 Command 状态查询；HTTP 202 仍只代表 Edge 已持久化 Command。

## BVS2-04 Leader Confirm 历史交接（2026-08-31）

当前项目状态：

```text
Phase 1  Architecture Foundation                     ACCEPTED / FROZEN
Phase 2  BVS2-01 业务设计                             FROZEN
Phase 2  BVS2-02 Core 数据与 Migration               COMPLETED
Phase 2  BVS2-03 Flight → Task → Candidate          COMPLETED
Phase 2  BVS2-04 Leader Confirm                      COMPLETED
Phase 2  BVS2-05 Edge Projection → Employee Command IN PROGRESS
```

本轮已完成：

- 新增 `internal/core/application/flighttask/confirmation.go` 和 `confirmation_handler.go`，确认入口为 `POST /api/v1/tasks/{taskPublicID}/confirm`。
- 新增 `internal/core/adapter/mysql/flight_task_confirmation_repository.go`，在单个 Core transaction 内锁定 Task/Candidate/Personnel，校验 Human Principal + `task:assign` + Team/Area Scope，复核当前人员事实，完成 `idle → reserved`、Assignment `confirmed`、Candidate `selected/rejected`、Task `assigned`，并写入三类 History、Audit、`task.assigned.v1` Outbox。
- `confirmation_id` 已成为业务幂等键：同 ID 同内容重放首次结果，不同内容返回 `confirmation_id_conflict`；失败业务结果也会持久化并可稳定重放。Task version、候选资格和人员状态均在服务层与 SQL 乐观条件中复核。
- `task.Instance`、`task.Assignment`、`task.AssignmentStatusHistory`、`personnel.StatusHistory` 和业务幂等字段已补齐；`api/core/openapi.yaml`、`memory-bank/architecture.md`、`memory-bank/implementation-plan.md`、`memory-bank/progress.md` 已同步。

真实验证结果：

- 定向 BVS2-04 单元测试通过：成功、权限范围、人员变化、幂等/冲突、已分配竞争结果。
- 全仓 `go test ./...` 通过；`go build ./...` 通过；`git diff --check` 通过。
- 临时内存卷 Core MySQL 已完成 `migrations/core/mysql`，`FLIGHT_RUN_BVS2_DB_TEST=1 go test ./internal/core/adapter/mysql -run TestFlightTaskSQLAgainstDocker -count=1 -v` 通过，真实数据库确认事务和重放通过。临时容器已停止并清理。
- 原 `local_core_mysql_data` 数据卷启动时报告 InnoDB 数据损坏；没有删除、重置该卷，也没有执行 `down -v`、DROP、TRUNCATE 或 destructive migration。原先退出的 `local-core-mysql-1` 仅保留为现场证据。
- Docker 前置链路仍保留：`scripts/ensure-docker.ps1` 会在验证前自动启动 Docker Desktop；本轮真实 SQL 验证使用了单独临时容器，因为原有 Core 数据卷不可用。

下一步只做 `BVS2-05 Edge Projection → Employee Command`：先让 `task.assigned.v1` 进入 Edge 最小 Projection，再实现员工 Accept/Complete 的 Edge Command、Core Worker 拉取和 Core 状态机结果回传。不要恢复旧 B3/B4/B5，不要让 Edge 写 Core 事实，也不要在没有用户确认时删除或重置 `local_core_mysql_data`。
## BVS2-06 Task Cancel 最新交接（2026-09-01）

当前状态：BVS2-06 的首个功能步骤 Task Cancel 已实现并通过单元测试和独立 Docker Core MySQL SQL 验证；完整 Compose 闭环、正式认证、Task CRUD 和全链路恢复仍未完成。

- 新增 Core 管理取消入口：POST /api/v1/tasks/{taskPublicID}/cancel，请求字段为 cancellation_id、expected_task_version、reason。
- admin/manager/leader 使用 Human Principal、task:cancel 和同时满足的 Team/Area Scope；leader 不能取消已被员工 Accept 的 in_progress Task；staff/machine 无权。
- awaiting 取消使 proposed Candidate 失效且不创建 Assignment；assigned/in_progress 取消 Assignment 并释放 Personnel；所有成功取消均写三类适用 History、Audit、task.cancelled.v1 Outbox 和业务幂等记录。
- 相同 cancellation_id 重放只返回首次结果；不同内容返回 cancellation_id_conflict；completed/cancelled 任务不重复推进状态。由于现有 outbox_event.correlation_id 为 CHAR(36)，超长取消 ID 使用稳定派生 UUID 作为 CorrelationID。
- 验证结果：scripts/ensure-docker.ps1 成功；受影响 Core 测试通过；临时 Core MySQL migration + TestTaskCancellationSQLAgainstDocker 通过。失败测试第一次暴露并已修复长 CorrelationID schema 边界，未改动已有数据卷。

接手后的下一步：先执行 powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1，然后继续 BVS2-06 完整 Compose/Worker/Edge 闭环和恢复场景；不要执行 docker compose down -v、DROP/TRUNCATE 或 migration down，不要触碰 local_core_mysql_data、flight-mysql、flight-redis。

## 2026-09-01 BVS2-06 Compose 闭环与恢复验收（最新）

- 已在隔离 Compose project `bvs206-closure` 使用端口 3330/3331 启动当前代码的 Core API、Edge API、Worker 和双 MySQL；Core/Edge migration 均为 `000001`、`000002 applied`，四个 live/ready 端点均返回 200。
- 真实 `TestArchitectureProbeAgainstRunningServices` 已通过；覆盖 SQL Outbox/Inbox、Edge Projection、重复 Event/Command、失败记录重试和 API 读取。
- 物理故障恢复已通过：Worker 停止后重启可继续处理已提交消息；Edge API 停止期间 Core Outbox 进入 retry，Edge 恢复后补投成功。
- 故障注入发现并修复 `DATETIME(6)` 时间四舍五入导致的 Core Inbox command 内容冲突误判；新增 `internal/shared/event.EqualPersistedTime` 并统一 Core/Edge 持久化时间比较。
- Docker Hub `golang:1.25`/`debian:bookworm-slim` 元数据请求仍返回 `EOF`，正常 `up --build` 未通过。为验证当前代码，使用本地交叉编译二进制装入缓存运行时镜像；该验证未使用旧业务二进制。临时 Compose/镜像资源仍保留供下一阶段重跑，结束时只清理本阶段自建资源。

当前接手点：BVS2-06 继续处理正式 JWT/mTLS 入口和鉴权拒绝路径，然后实现冻结范围内的 Task 查询/详情/生命周期管理 CRUD。开始下一步前仍先读取 `memory-bank/project-memory.md`、`memory-bank/architecture.md`、`memory-bank/design-document.md`、`memory-bank/implementation-plan.md`、`memory-bank/progress.md`，并先运行 `scripts/ensure-docker.ps1`。

## BVS2-06 JWT/mTLS 入口（2026-09-01）

- 已新增 `internal/platform/httpauth` 并在 Core Confirm/Cancel、Edge 员工 Task/Command 生产装配中强制 Bearer JWT；未认证请求实际返回 401。
- Actor Header 仅在显式非 release 调试开关启用；Edge Command 校验 Actor 与 JWT Principal 一致。
- Core/Edge Server 与 Worker Sync Transport 已支持可配置证书、CA、客户端证书和 mTLS；默认本地配置关闭 TLS，生产需要提供证书路径并开启 `require_client_cert`。
- JWT 仍不保存完整权限列表；角色/Scope 若由签名 Token 提供只作为身份属性，业务授权仍在服务端完成。
- JWT/mTLS 单元/HTTP 回归和当前隔离 Compose 匿名 401、开发适配器、双向 Probe 已通过。

当前接手点：实现 Task 查询/详情和冻结范围内的 CRUD/生命周期管理；不要把 Edge Projection 当成 Core CRUD 数据源，也不要新增未设计确认的手工 Task 创建语义。
## 2026-09-01 BVS2-06 Task CRUD 读侧完成

- Core 已注册 `GET /api/v1/tasks` 和 `GET /api/v1/tasks/{taskPublicID}`；返回 Task、Candidate 快照和 Assignment，并执行 JWT 注入后的 `task:read` 与 Scope 查询。
- 本轮真实验证：`go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all`、Core MySQL Task Query 集成、Compose 匿名 401/开发适配器/health、双库 SQL/HTTP/Worker Probe 均通过。
- Task 创建仍由 `POST /api/v1/flights/{flightPublicID}/arrival` 事务触发；Task 删除使用 `POST /api/v1/tasks/{taskPublicID}/cancel`，保留 History/Audit/Outbox。手工 POST/PATCH/硬删除因语义未冻结暂不暴露。
- 隔离资源：`bvs206-closure` 的双 MySQL、Core/Edge/Worker 应用容器和临时验证镜像/二进制仍保留；原有 `flight-mysql`、`flight-redis` 未触碰。正常 `docker compose up --build` 仍被 Docker Hub `EOF` 阻断，当前代码验证使用本地交叉编译二进制+缓存运行时镜像。

## 2026-09-01 对话结束交接（当前基线）

- 后端当前冻结范围已完成：Architecture Foundation A1-A10、BVS2-01 至 BVS2-06 均已有代码、迁移、测试或隔离 Compose 验收记录。
- 后端仍有明确产品化待办：完整 OIDC/生产身份源与 Staff 映射、密钥轮换和正式账号体系；Edge Command 状态查询契约；手工 Task 创建、任意 PATCH、硬删除的业务语义。上述事项不能从现有 BVS2 语义自行推断。
- 前端尚未进入业务实现：`frontend/` 目前只有工作区目录骨架和设计/计划文档；下一步从 F0 设计冻结与前后端交接确认开始，之后执行 F1-F8。
- 已实际记录的验证包括 `go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all`、Core/Edge 隔离 MySQL、双向 SQL/HTTP/Worker Probe、JWT 匿名拒绝和 Worker/Edge 故障恢复；正常 `docker compose up --build` 仍受 Docker Hub 基础镜像元数据 `EOF` 阻断。
- 工作区包含本项目既有 v2 代码、配置、迁移、文档和前端骨架差异；`.bvs206-validation/` 仅为临时交叉编译验证二进制，已加入 `.gitignore`，不纳入提交。未执行删除、清库、`down -v`、migration down、分支切换或覆盖用户文件。
- 下一次接手先重新读取 `memory-bank/project-memory.md`、`memory-bank/architecture.md`、`memory-bank/design-document.md`、`memory-bank/implementation-plan.md`、`memory-bank/progress.md`，然后从前端 F0 开始；任何验证前先运行 `scripts/ensure-docker.ps1`。
- BVS2-07 Employee Identity & Session 已实现：Core 凭证、个人微信/企业微信绑定、一次性绑定票据、Edge Session、短期 JWT、Refresh 轮换/重放撤销、当前会话查询和 Logout 均已接线；没有接入真实第三方账号或提交生产密钥。
- 关键代码：`internal/core/application/identity`、`internal/core/adapter/mysql/identity_repository.go`、`internal/edge/application/identity`、`internal/edge/adapter/mysql/session_repository.go`、`internal/integration/identity`；迁移为 Core/Edge 各自的 `000003`。
- 已完成验证：Docker CLI 位于 `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`，前置检查通过；隔离项目 `bvs206-closure` 的 Core/Edge `000003` migration 已为 `applied`。`go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all` 和当前源码 HTTP 联调均通过；真实微信/企业微信 Provider 仍未接入。
- 下次接手优先补跑双库 `000003` migration 和 Core↔Edge HTTP 登录/绑定/刷新/撤销集成测试；继续保持旧 B3/B4/B5 现场不恢复，不执行 `down -v`、DROP/TRUNCATE 或 migration down。

## 2026-09-01 BVS2-07 在线双库验收补充

- 使用隔离 Compose project `bvs206-closure` 的 Core/Edge MySQL（3330/3331）完成 `000003_employee_identity` 与 `000003_employee_session` migration；未停止或修改 `flight-mysql`、`flight-redis`。
- 使用当前源码临时 Core/Edge 进程完成真实 HTTP 验证：工号密码、个人微信/企业微信绑定与 exchange、单次 ticket、同一外部身份冲突、未映射拒绝、refresh 轮换/重放撤销、当前会话、logout、内部共享密钥和 Staff 停用 fail-closed 均通过。
- 验证中发现并修复 MySQL Store 的 refresh replay 事务回滚问题；修复后替换会话在重放时已实际撤销。临时员工、凭据、绑定、ticket、Edge session 和 API 进程均已清理，Core/Edge 夹具残留计数为 0。
- 修复后 `go test ./...`、`go build ./...`、`git diff --check` 均通过；没有执行 commit、push、`down -v`、DROP、TRUNCATE、migration down 或分支切换。
- 下一步：与前端 F0 对齐身份/Command/OpenAPI 合同；真实 Provider、生产凭据、Command 状态查询和性能基线仍是产品化待办。

## 2026-09-02 T0-1 可观测性实现完成

- Core API、Edge API、Worker 已接入内部 `/metrics`；覆盖 HTTP 请求/延迟、数据库连接池、Core Outbox、Edge Command/Inbox backlog、oldest age、Projection lag，以及 Worker 投递/处理/确认延迟。
- Core/Edge backlog 指标来自持久化 Store，Worker 指标来自同步操作；没有削弱 Outbox、Inbox、Command、幂等、版本、Retry 或 Failed 语义。
- Docker 前置检查、受影响测试、`go build ./...`、运行时 metrics smoke、`scripts/verify.ps1 -Mode all` 均通过。真实代表性业务负载的 p50/p95/p99 尚未生成，因此没有把空跑结果写成性能结论。
- 当前后端下一步为 T0-3：增加提交后 Notification/Fan-out Port；随后按固定负载决定批量拉取/确认、连接池、索引、Gateway 或多 Worker 是否必要。

## 2026-09-02 T0-2 拉取恢复契约实现完成

- Edge `GET /api/v1/tasks` 继续按员工 JWT Principal 返回完整 Projection 快照，增加 `snapshot_at`、`sync_mode=full_snapshot`、员工级持久化 `projection_revision`、Projection lag 状态、`next_cursor` 和 `reset_required`。
- 空任务返回 HTTP 200 与空数组；取消/完成保留 Projection 终态；启动、刷新、重连、推送提示后的校准和离线恢复统一重新拉取快照。Command 使用稳定 `command_id` 和 `GET /api/v1/commands/{commandID}` 独立恢复。
- Edge MySQL 新增 `000004_employee_projection_cursor`；当前不启用增量 API，`next_cursor=null`、`reset_required=false`，不得把单 Task `sync_version` 当员工全局 cursor。
- 本轮 Docker 前置检查通过；Edge sync、Edge HTTP、迁移发现和 Worker 集成测试通过，最终 `scripts/verify.ps1 -Mode all` 也已通过（含全量 Go 测试、构建、Compose config 和 `git diff --check`）。真实业务负载 p50/p95/p99 仍未生成。

下一步进入 T0-3：增加提交后 Notification/Fan-out Port，再按固定负载生成性能基线；不要自动执行 000004 之外的 migration down、清库或 `down -v`。
## 2026-09-01 BVS2-07 后续：Employee Command 契约

- 已完成员工 Accept/Complete 的客户端幂等键：请求必须携带稳定 `command_id`；同一逻辑命令重试返回 `duplicate=true`，业务内容冲突返回 `409 command_id_conflict`。
- 已新增 `GET /api/v1/commands/{commandID}` 公共状态查询，仅命令所属员工可读，返回 `pending/syncing/confirmed/failed`、attempts、重试时间和安全失败码。
- 已更新 Edge Memory/MySQL Store、Core/Edge Command 等价判断、Edge OpenAPI 以及单元/HTTP 测试；下一步可进入 `/frontend` API Client 接入。真实微信/企业微信 Provider 和 Task 生命周期待确认项不在本次变更内。
## 2026-09-01 Employee Command 验收完成

- 隔离 `bvs206-closure` Edge MySQL 的当前源码 HTTP/SQL 验收已通过：稳定 `command_id` 首次提交/重复提交分别返回 `duplicate=false/true`，状态查询返回合法公共状态，跨员工查询 403，同 ID 内容冲突 409。
- 临时 Edge 进程和测试命令记录已清理；`go test ./...`、`go build ./...`、`scripts/verify.ps1 -Mode all`、OpenAPI YAML 解析和 `git diff --check` 均通过。未执行 commit/push。

## 2026-09-02 T0-3 通知抽象和提交后边界实现完成

- Edge 新增 `internal/edge/application/notification` Notification/Fan-out Port、`TaskChanged`、`Sink`/`Publisher` 和单实例员工隔离内存 fan-out；通知载荷不包含员工 ID。
- 内部同步事件先完成 `ApplyEvent`，仅成功且非重复的任务事件在提交后发布；单连接失败继续投递其他连接，通知失败只记录日志/指标，不回滚 Projection 或改变 `202`。
- 内存通知不是可靠队列；断线、重启、丢失和跨副本未命中由 `GET /api/v1/tasks` 完整快照及 `projection_revision` 恢复。
- Docker 前置检查、定向 Go 测试和最终 `scripts/verify.ps1 -Mode all` 已通过；全量验证包含 `go test ./...`、`go build ./...`、Compose config 和 `git diff --check`。下一步为 T0-4 WebSocket 适配器。

## 2026-09-02 T0-4 员工 WebSocket 提示实现完成

- Edge 新增 `POST /api/v1/realtime/ticket` 与 `GET /api/v1/ws`，员工 JWT 通过 ticket 接口换取 30 秒一次性不透明 ticket，浏览器以 WebSocket 子协议传递；服务端只协商 `flight.realtime.v1`，不接受 URL 凭据。
- WebSocket 适配器完成同源/协议握手、员工 Principal 绑定、`ready`/`ping`/`pong`、读写超时、空闲关闭、连接清理和 WebSocket 指标，并接入 T0-3 Notification Port。
- employee-web 完成 ticket 获取、心跳回复、指数退避重连和通知 ID 有界去重；连接成功、重连和 `task_changed` 均触发完整任务快照刷新，HTTP 仍是最终读取和恢复路径。
- Docker 前置检查、Go 定向测试、前端 `typecheck`、源码 `lint`、`test`（2 个测试文件、5 个测试）和 `build` 均通过；真实浏览器、服务重启/网络分区和多副本 fan-out 尚未验收。下一步进入 T0-5 Gateway 与共享 Edge 多副本。
## Current session handoff (2026-09-03)

- Core management SSO/session code, direct Enterprise WeChat/development provider adapters, Core migration `000005_admin_sso`, personnel/assignment read APIs, OpenAPI and frontend client contracts, and ADR-011 are present in the workspace.
- Authorization remains Core-owned: provider authentication must resolve to a pre-provisioned active `admin_identity`; current role and scope are restored from the Core session. Production identity source, credentials, callback domains, and admin provisioning remain pending.
- Docker Engine preflight succeeded in this session. The new migration has not been applied, and no destructive database or volume operation was run. Full verification status is recorded in `memory-bank/progress.md`.
- Next session: run the full Go test/build checks, apply `000005_admin_sso` only to an isolated database, then exercise management SSO/personnel/assignment HTTP flows before production-provider, failure-recovery, and performance gates.
## Provider decision clarification (2026-09-03)

- The current product decision is direct enterprise WeChat browser OAuth. The pushed Core implementation supports `wecom` as the current default, including server-side access-token caching and member `UserId` mapping; OIDC is an optional future-IAM adapter.
- The two post-push modifications in `memory-bank/architecture.md` and `memory-bank/progress.md` were preserved and are being synchronized in this supplementary commit; they were not overwritten as unknown parallel work.
