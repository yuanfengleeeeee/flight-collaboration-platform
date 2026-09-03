# 项目长期记忆

> 用途：保存跨会话、跨任务仍然有效的协作约束。当前任务进度和临时错误仍记录在 `memory-bank/progress.md` 或根目录 `HANDOFF.md`。

## 当前基线

- Architecture v2.0 Foundation 已完成并冻结为 `ACCEPTED / FROZEN`。
- 当前处于 Phase 2 Business Implementation：BVS2-01 设计、BVS2-02 Core Migration、BVS2-03 `Flight → Task → Candidate`、BVS2-04 `Leader Confirm`、BVS2-05 Edge Projection/Employee Command 以及 BVS2-06 的 Task Cancel、Compose/恢复验证、JWT/mTLS 和 Task Query 读侧已完成对应代码或验证；当前转入前端 F0 架构冻结。
- Core 是航班、人员、任务、Assignment、状态历史、Audit 和业务状态机的唯一事实源；Edge 只保存最小 Projection、Command、Session 和 Inbox 数据。
- 前端采用前后端分离模式，所有前端运行时代码位于单仓库内独立的 `frontend/` 工作区；首期同时建设管理端和员工端。管理端 `admin-web` 只调用 Core API，采用桌面优先响应式 Web；员工端首期 `employee-miniapp` 只调用 Edge API，采用同时支持个人微信和企业微信入口的微信小程序；`employee-web` 保留为正式可用的员工网页版和备用入口，不改变 Core/Edge 边界。正式 React/Vite/pnpm workspace、共享 contracts/api-client/auth/task-domain/ui、管理端和员工 Web 首个真实闭环已落地；`frontend/preview/` 仍是独立 Mock 探索入口。
- 员工身份体验需求已明确：首次进入小程序使用工号 + 密码认证；个人微信和企业微信都绑定到同一个 Core `Staff`；任一入口读取同一份 Edge Projection；完成绑定后优先提供简易登录和较长会话。BVS2-07 已完成本地 Mock Provider、绑定、刷新和撤销实现，并已在隔离双库上完成在线联调；真实 Provider 适配器、原生小程序登录壳和管理端企业微信 SSO 回调适配已加入，但生产凭据、回调域名、管理员预置和线上联调仍待完成。
- 无 OIDC 时，Core MySQL 的 `personnel`、`employee_credential`、`external_identity_binding`、`operation_area/team/team_member` 构成员工主数据/凭证库；不新增第三套物理数据库，员工库的新增、停用、密码重置和身份绑定由受保护的 Core 管理接口或受控导入流程完成。
- F0 页面信息架构已按用户确认更新：主任=`manager` 查看全部任务，队长=`leader` 按团队/区域自动获得任务，员工=`staff` 只查看本人任务，系统管理员=`admin` 负责系统管理；管理端首期完整显示规划模块，员工端包含任务、通知、异常、历史和账号。正式应用已按独立客户端实现，员工 Web 真实 API/浏览器 E2E 已通过，首轮视觉收口已完成；最终视觉评审、企业微信生产凭据/域名联调、员工主数据导入和管理员预置仍待完成。
- 跨端字符编码约束已明确：源码、JSON/HTTP 请求与响应、WebSocket JSON、小程序网络数据和 MySQL 文本统一使用 UTF-8；JSON 请求显式声明 `application/json; charset=utf-8`，MySQL 使用 `utf8mb4`，禁止依赖 Windows 默认代码页写入中文 fixture。
- 旧 `cmd/server`、旧 `internal/model`、旧业务路由和旧 B3/B4/B5 现场保持 `legacy/paused`，不能作为新业务入口恢复。

## 不可变协作约束

- 新业务必须遵守 Core/Edge 独立数据库、版本化 SQL Migration、Outbox/Inbox、幂等、Retry、RBAC/Scope 和 Human/Machine Principal 边界。
- Handler 不直接调用 GORM；Service 不依赖 Gin；Domain 不依赖 Gin、GORM 或 Redis。
- 关键 Core 业务事实、状态历史、Audit 和 Outbox 必须在同一个 Core 事务内提交。
- 不使用 `tenant_id`、`airport_id` 或多机场切换模型；不擅自引入 Kafka、微服务拆分、真实外部推送或 AI 模型。

## 性能与可靠性约束

- 消息准确性、可靠同步、状态收敛和故障可恢复性优先于视觉效果；任何优化不得删除或弱化 Outbox、Inbox、Command、幂等、版本控制和 Retry。
- 前端允许使用短时、低成本、服务于反馈的 `transform`/`opacity` 动画，但默认禁止高成本持续特效、全屏视频/WebGL/Canvas 粒子、视差/鼠标跟随、大面积 blur、动态渐变、全量列表 stagger 和大型动效资源；关键操作不能等待动画。
- 前端必须支持 reduced motion，优化请求去重/取消、代码分包、长列表、内存生命周期和小程序增量更新；不把本地缓存或离线队列当作可靠业务消息。
- 后端优化先测量 API p95/p99、数据库查询、Worker 投递延迟、Outbox/Inbox 堆积和 Projection lag；Redis 不能替代可靠存储或同步。
- 详细规则和未来待办见 `docs/performance-and-reliability-baseline.md`。

## 功能验证前置规则

- 任何功能验证、集成验证、数据库验证或 Compose 验证，先执行：

  ```powershell
  powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
  ```

- 推荐使用统一入口：

  ```powershell
  powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
  ```

- `ensure-docker.ps1` 会检测 Docker Engine；未就绪时启动 Docker Desktop 并等待 Engine 可用。它不会自动启动项目容器、执行 Migration、删除 Volume 或清理数据库。
- Docker CLI 或 Docker Desktop 不可用时，验证必须明确失败并记录原因，不得静默跳过 Docker 前置检查。
- 运行验证前仍须阅读 `memory-bank/architecture.md`、`memory-bank/design-document.md`、`memory-bank/implementation-plan.md` 和 `memory-bank/progress.md`，并保留工作区已有修改。

## 记忆维护规则

- 跨任务仍有效的架构和协作规则更新 `AGENTS.md` 与本文件。
- 里程碑完成情况、测试真实结果和临时阻塞更新 `memory-bank/progress.md`。
- 长会话结束或交接时更新对应的 `memory-bank/handoffs/frontend.md`、`backend.md` 或 `infrastructure.md`，再更新根 `HANDOFF.md` 的全局索引；不把未运行的测试写成已通过。

## 分任务交接策略

- `HANDOFF.md` 只保存项目级总览、当前任务文件索引和跨任务冲突；下方保留的历史章节不是当前状态的唯一来源。
- `memory-bank/handoffs/` 使用滚动快照：`frontend.md` 负责前端，`backend.md` 负责后端，`infrastructure.md` 负责 Docker/Compose/数据库/部署和基础设施。交接时替换过期当前状态，不无限追加重复内容。
- `memory-bank/progress.md` 才是按时间记录里程碑和临时结果的地方；它可以追加，过长后按阶段归档。`project-memory.md` 和 `AGENTS.md` 只保存长期规则。
- 同时进行多个任务时，读取不会冲突，但并发写同一文件可能覆盖内容。写入前必须重新读取目标文件、根 `HANDOFF.md` 和 `git diff`；发现其他会话修改时先合并事实，不覆盖未知修改。
- 结束会话触发语义包括“我要结束当前这个对话了”“我要退出当前对话”“准备开新线程”等明确表达；普通退出页面、退出程序不触发项目交接。

## GitHub 同步规则

- 当用户表达“我要结束当前这个对话了”“我要退出当前对话”或语义相近的明确结束会话意思时，除更新对应任务交接文件和 `HANDOFF.md` 外，自动执行一次 GitHub 同步。
- 同步前检查当前分支、远程、工作区差异和未跟踪文件；只提交已经核对且属于项目的修改，不提交密钥、Token、生产配置、本地数据库/Volume、缓存、临时产物或归属不明的用户文件。
- 触发该规则时，用户已明确授权创建一次 commit 并 push 到当前分支的 `origin`；不得 force push、reset、删除或覆盖本地修改，也不自动创建 Pull Request。
- Push 后核对远程 SHA 和工作区状态。若远端领先、冲突或推送失败，保留现场并报告真实原因，不得把未成功的操作写成已同步。
## Current handoff baseline (2026-09-01)

- BVS2-06 Compose closure and Worker/Edge outage recovery have been physically verified on isolated project `bvs206-closure`; existing `flight-mysql` and `flight-redis` were not changed.
- Core/Edge management entrypoints now require Bearer JWT in normal configuration; development actor headers are explicit non-release compatibility adapters. TLS/mTLS is configurable for Core/Edge servers and Worker sync transport.
- Task CRUD currently means Core list/detail plus Arrival-created tasks and Confirm/Cancel lifecycle operations. Manual `POST /tasks`, arbitrary PATCH and hard delete are intentionally not implemented until the product contract is frozen.

## Current frontend freeze baseline (2026-09-01)

- F0 架构冻结已启动，冻结门槛和关闭条件统一记录在 `frontend/docs/architecture-freeze-gate.md`。
- 前端客户端边界固定为：`admin-web → Core API`、`employee-miniapp → Edge API`、`employee-web → Edge API`；员工小程序同时支持个人微信和企业微信，员工 Web 是正式支持的备用入口。
- Core Task 列表/详情读侧、Arrival 创建和 Confirm/Cancel 基础代码已有；手工 Task `POST/PATCH/硬删除` 不得由前端自行假设。
- 当前进入真实业务前的主要阻塞是生产微信/企业微信 Provider 凭据与环境域名、管理员 `admin_identity` 预置、员工主数据导入/管理流程、Mock/Contract/E2E 测试数据、浏览器联调、Task 生命周期补充验收和性能门禁；前端工具链已安装并通过 lint/typecheck/unit/build。
- BVS2-07 Employee Identity & Session code is now present: Core owns bcrypt employee credentials, external provider bindings and client-bound one-time binding tickets; Edge owns minimal revocable sessions and refresh rotation. Development uses explicit `mock:<subject>` provider codes; configurable server-side WeChat/WeCom adapters and native miniapp shell are present but production calls remain disabled until credentials and domains are supplied.
- BVS2-07 public Edge routes are `/api/v1/auth/password/login`, `/api/v1/auth/exchange`, `/api/v1/auth/bindings/complete`, `/api/v1/auth/refresh`, `/api/v1/auth/me`, and `/api/v1/auth/logout`; employee JWTs use the `-edge` audience and carry a session `sid` validated against Edge storage plus the current active Staff status from Core.
- BVS2-07 was verified against the isolated `bvs206-closure` Core/Edge MySQL pair after both `000003` migrations were applied: current-source Core/Edge processes passed live password login, both provider bindings and exchange, one-time ticket use, conflict/unmapped rejection, refresh rotation and replay revocation, logout, internal-key protection, and inactive-Staff fail-closed checks. Real WeChat/WeCom provider calls remain out of scope.
## 2026-09-01 员工 Command 幂等与状态查询

- Edge 员工 Accept/Complete 现在要求客户端提交稳定 `command_id`；同一逻辑 Command 的重试不会因 trace/occurred_at 变化而冲突，不同业务内容仍返回 `command_id_conflict`。
- 新增公开 `GET /api/v1/commands/{commandID}`，仅允许所属员工查询，返回 `pending/syncing/confirmed/failed`、attempts、更新时间和安全失败类别；未公开内部同步状态或原始错误文本。
- Edge Memory/MySQL Store 增加 `FindCommand` 和 `updated_at`，Core/Edge 统一使用逻辑 Command 等价判断；前端刷新恢复可直接基于该接口实现。

## T0 员工任务实时交付长期决策（2026-09-02）

- Edge 是统一的员工接入/Projection 边界，不按任务类型拆分；扩容使用相同 Edge 副本和共享 Edge MySQL。
- 员工任务以 Edge `GET /api/v1/tasks` 拉取为最终读取和恢复机制；前台实时化已接入 WebSocket 变化提示，后台平台通知仍待实施，二者都不能替代拉取。
- 推送只允许是可丢失、可重复、可乱序的 best-effort hint。Core Outbox、Worker、Edge Inbox、Projection、Command 幂等和版本收敛仍是可靠链路；Redis 只能承担临时 fan-out，不能成为可靠同步存储。
- T0 修改顺序固定为：契约 → 可观测性基线 → 拉取恢复 → Notification Port → WebSocket → Gateway/多副本 → 后台通知 → 故障/容量门禁。持久消息 Broker 和 Core 微服务拆分不属于 T0 默认动作。
- 详细设计和未完成验收项记录在 `docs/adr/ADR-009-employee-task-realtime-delivery.md` 与 `memory-bank/implementation-plan.md`。

## T0-4 WebSocket 提示实现状态（2026-09-02）

- Edge 已提供员工认证的 `POST /api/v1/realtime/ticket` 与 `GET /api/v1/ws`；ticket 为短期一次性不透明凭据，通过 `Sec-WebSocket-Protocol` 传递，不放入 URL。握手校验 Edge JWT/sid、Human Principal、同源 Origin 和 `flight.realtime.v1`。
- `internal/edge/application/realtime` 已接入 T0-3 Notification Port，提供员工隔离连接、`ready`/`ping`/`pong`、空闲关闭、写超时、连接清理和实时指标；通知仍是 best-effort，Projection/HTTP 快照是可靠恢复路径。
- `employee-web` 已使用 `RealtimeClient`：连接/重连成功和 `task_changed` 都触发完整 `GET /api/v1/tasks` 快照刷新；notification ID 有界去重，ticket 401 停止重连并进入认证恢复路径。
- 代码级测试已覆盖 ticket 单次消费、JWT 员工隔离、握手、心跳、通知、重复提示、断线重连和空闲关闭；员工 Web 真实浏览器握手已由 Playwright E2E 验证，服务重启、网络分区和多副本 fan-out 仍待 T0-5/T0-7。

## T0-1 可观测性实现状态（2026-09-02）

- Core API、Edge API 和 Worker 已接入内部 `/metrics`；覆盖 HTTP 请求/延迟、数据库连接池、Core Outbox、Edge Command/Inbox backlog、oldest age、Projection lag，以及 Worker 投递/处理/确认延迟。
- 队列指标通过 Core/Edge 持久化 Store 统计，Worker 指标服务只监听 `/metrics`；本实现不改变 Outbox、Inbox、Command、幂等、版本收敛、Retry 或 Failed 语义。
- 代码测试、Docker 前置检查、全量验证和无数据库写入的运行时指标 smoke 已通过；真实代表性业务负载下的 p50/p95/p99 报告仍待固定测试参数后生成。

## T0-2 拉取恢复契约实现状态（2026-09-02）

- Edge 员工任务接口仍是 JWT Principal 过滤的完整 `GET /api/v1/tasks` 快照；响应现在带 `snapshot_at`、`sync_mode=full_snapshot`、员工级 `projection_revision`、Projection lag 状态、`next_cursor` 和 `reset_required`。
- Edge MySQL 新增 `000004_employee_projection_cursor`，Memory/SQL Store 都在 Projection 新增或更高版本更新时推进员工 revision；同版本重放、过期版本和重复拉取无副作用，转派同时推进原员工和新员工 revision。
- 客户端语义已冻结：空数组是合法空结果；取消/完成保留终态；启动、刷新、重连和离线恢复重新拉取完整快照；本地缓存不作为可靠事实；Command 仍通过稳定 ID 和公共 status API 恢复。
- 当前没有增量 API，`next_cursor=null`、`reset_required=false` 是明确的未启用状态；不得使用单 Task `sync_version` 作为员工全局游标。部署 Edge SQL 前需按正常流程应用 000004 migration。

## T0-3 通知抽象和提交后边界实现状态（2026-09-02）

- Edge 新增 `internal/edge/application/notification` Notification/Fan-out Port、`TaskChanged` 消息、`Sink`/`Publisher` 接口和员工隔离的单实例 `InMemoryFanout`；业务 Handler 不直接操作连接。
- 内部同步事件先完成 `ApplyEvent`，仅成功且非重复的任务事件在提交后发布 `task_changed`；通知失败只记录日志和 `flight_notification_delivery_total`，不回滚 Projection、不改变 `202`。
- 员工公共 ID 仅作为内部路由键，不进入通知载荷；载荷只包含通知 ID、任务 ID、版本、原因和时间。连接表不持久化，重启、断线、丢失和跨副本未命中均由完整任务快照恢复。
- 定向测试已覆盖提交后可见性、重复事件不重复 fan-out、单连接失败继续投递和员工隔离；下一步进入 T0-4 WebSocket 适配器。
## T0-5 Gateway 与多副本实现状态（2026-09-02）

- 已实现本地 Gateway（Nginx）和两个相同 Edge 副本：宿主机只暴露 Gateway，Edge 副本共享 Edge MySQL，Worker 访问 Gateway，WebSocket Upgrade 由 Gateway 转发。
- 已实现 Core Outbox/Edge Command 数据库租约及 Worker owner 传递；租约过期可恢复，旧 owner 的确认不会覆盖新 owner。
- 已实现共享 Edge SQL realtime ticket（只保存 hash）和 Redis Pub/Sub best-effort 跨副本 `task_changed` fan-out；Redis 不是可靠同步依赖。
- 本轮 Go 全量测试已通过；Docker 前置脚本因当前 PATH/机器未找到 `docker.exe` 明确失败，因此 Compose、迁移和双副本故障场景尚未宣称验收通过。下一步是恢复 Docker CLI 后执行 T0-5/T0-7 验收。
## Current handoff update (2026-09-03)

- The next P0 backend slice is implemented in the workspace: Core-owned management SSO/session (`admin_identity`, `admin_sso_state`, `admin_session`) and read-only personnel/assignment APIs with RBAC and server-derived scope.
- OIDC is a production-shaped adapter but remains disabled until the identity source, credentials, callback domains, and provisioned `admin_identity` records are supplied. The explicit development provider is not a production substitute.
- Core migration `000005_admin_sso` is present but unapplied. No Compose startup, migration up, volume deletion, migration down, DROP, or TRUNCATE was performed in this handoff.
- Docker preflight and `scripts/verify.ps1 -Mode all` passed on 2026-09-03 after switching Go to the ignored project-local temporary cache because the default cache was access denied.
