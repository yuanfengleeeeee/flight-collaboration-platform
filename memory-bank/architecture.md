# 架构记录

## Architecture v2.0 当前冻结结论（2026-08-13）

- 最终形态为单仓库 + 内网 Core 模块化单体 + 云端 Edge 接入服务 + Core/Edge 独立数据库 + Core 唯一业务事实源 + Edge Projection + Outbox/Inbox 可靠双向同步。
- 系统只服务单机场。禁止 Tenant/Airport 模型、`tenant_id`、`airport_id`、airport scope 和机场切换。
- Core 拥有航班、人员、岗位、能力、人员状态、任务模板/实例/分配、事件、规则、审计和状态机；Edge 只保存 Projection、Command、Session、Inbox、delivery 等移动端最小数据。
- Core/Edge 使用独立 MySQL、账号、schema 和 migration；禁止共库、跨库 JOIN、Edge 直连 Core DB、Core 直写 Edge DB 和启动 AutoMigrate。
- 同步使用版本化 Event/Command Envelope、at-least-once、幂等消费、Retry 和 Failed 状态。Core Worker 优先主动拉取 Edge Pending Commands，生产 Transport 预留 HTTPS/mTLS。
- 当前角色为 admin/manager/leader/staff；Human Principal 与 Machine Principal 分离；RBAC + 结构化 global/area/team/assigned/self Scope；JWT 不保存完整权限列表。
- Architecture Foundation 阶段暂停 B3/B4/B5，且不引入 Kafka、RabbitMQ、Kubernetes、Service Mesh、分布式事务、真实 AI/RFID/推送；当前 Phase 2 已在冻结基线上完成 BVS2-03，旧 B3/B4/B5 现场仍保持 legacy/paused。

规范来源：`docs/architecture/architecture-v2.md`、`docs/adr/`、`memory-bank/design-document.md`。

## BVS2-05 实际代码地图（2026-09-01）

- `internal/edge/application/server.go` 负责四类 Task 快照事件的投影映射，并提供员工专用 Accept/Complete Command 入口；Edge 只写自己的 Projection/Command Store。
- `internal/edge/sync/store.go` 与 `sql_store.go` 按 `sync_version` 执行单调收敛，保留旧版本忽略、同版本幂等和同版本冲突失败语义；Edge migration 只扩展 `task_projection.assignment_public_id`。
- `internal/core/application/flighttask/employee_command.go` 定义员工 Command Port 和状态机；`internal/core/adapter/mysql/employee_command_repository.go` 在同一 Core MySQL transaction 内实现锁定、乐观版本更新、History、Audit、Core Inbox 和 Outbox。
- `internal/integration/sync/worker.go` 通过 `CommandProcessor` 连接 Foundation 与业务 Core 命令处理，`cmd/worker` 在 Core MySQL 可用时注入业务 Repository。
- Core 仍是 Task/Assignment/Personnel 唯一事实源；Edge 不因 HTTP 202 或本地点击直接把 Projection 改成最终业务状态，必须等待 Core Event 回传。

## 前端工程设计基线（2026-09-02，F2 API INTEGRATION IN PROGRESS / VISUAL REVIEW OPEN）

- `frontend/` 已落地 pnpm workspace、React/Vite 的 `admin-web` 与 `employee-web`、contracts/API client/auth/task-domain/ui 共享包，以及 `employee-miniapp` 的原生 `wx.request`/存储适配层；`frontend/preview/` 仍是脱离后端的零依赖 Mock。客户端不共享数据库或业务事实。
- `admin-web` 只调用 Core API；`employee-miniapp` 与 `employee-web` 只调用 Edge API。根目录原有空的 `web-admin` 和 `miniapp` 未作为新的 v2 前端入口使用。
- 首期同时建设管理端和员工端：`admin-web` 采用桌面优先响应式 Web；员工端首期采用同时支持个人微信和企业微信入口的独立微信小程序，同时保留正式可用的 `employee-web` 网页版。小程序和网页版复用共享契约但不改变 Core/Edge 边界。
- 个人微信与企业微信外部身份必须映射到同一个 Core Staff 事实；小程序只接收服务端签发的会话，不得把 openid、unionid 或企业微信用户标识直接当作业务身份，也不得自行伪造 `X-Employee-Public-ID`。
- `admin-web` 只调用 Core API，面向任务工作台、Task 详情、Candidate 选择和 Leader/Manager Confirm；`employee-miniapp` 与 `employee-web` 只调用 Edge API，面向最小 Task Projection、Accept 和 Complete；网页版作为正式可用的直接浏览器入口。
- `/internal/sync/v1/*` 永远不属于浏览器 API；`X-Actor-*`、`X-Employee-Public-ID` 只允许由显式开发适配器注入，不能作为生产身份方案。
- 员工端将 `business_status` 与 Command 的 `pending/syncing/confirmed/failed` 显示层状态分离；202 Accepted 不会直接推进业务状态，必须等待 Core Event 回传后的 Edge Projection。
- 所有跨端文本统一使用 UTF-8：JSON 请求显式使用 `application/json; charset=utf-8`，服务端 JSON 响应带 UTF-8 charset，WebSocket JSON 和小程序网络请求遵守 UTF-8，Core/Edge MySQL 连接与表使用 `utf8mb4`；不得通过 Windows 默认代码页生成业务数据。
- 性能约束为同步准确性、消息可靠性和状态收敛优先于视觉装饰；前端关键操作不能等待动画，默认只允许短时 `transform`/`opacity` 动效，后端优化必须先测量且不得削弱 Outbox/Inbox/Command、幂等和版本保护。
- 页面详细设计位于 `frontend/docs/page-design.md`，真实 API 接入说明位于 `frontend/docs/api-integration.md`；用户已确认角色映射、团队/区域任务 Scope、管理端完整首期导航、员工端通知/异常/历史/账号入口和主任全量可见。管理端 Task List/Detail/Confirm/Cancel、SSO 回调适配与员工端 Edge 登录/Projection/Accept/Complete/Command Status 已有源码；真实 Provider 适配器也已加入，平台凭据、管理主体映射、回调域名和运行时验证仍待完成。

## Foundation 与 BVS2-04 实际代码地图（2026-08-31）

- `cmd/core-api/main.go`：只装配 Core DB、可选 Redis、Core Server 和生命周期。
- `cmd/edge-api/main.go`：只装配 Edge DB、可选 Redis、Edge SQL Projection/Command Store 和生命周期。
- `cmd/worker/main.go`：只装配 Core Store、Edge HTTP Transport、Outbox 投递和 Command 拉取；不连接 Edge DB。
- `cmd/migrate/main.go`、`internal/platform/mysql/migrator.go`：按 `-target core|edge` 显式执行独立 migration；`down` 要求显式 destructive flag。
- `internal/core/sync/`：Core Outbox/Core Inbox/Probe Transaction 合约、Memory Probe Store、MySQL adapter。
- `internal/core/application/flighttask/`：BVS2-03 Flight 到达与 BVS2-04 Leader Confirm Application Use Case；Arrival 与 Confirmation 使用独立 Port，均不直接依赖 Gin/GORM。
- `internal/core/adapter/mysql/flight_task_repository.go`、`flight_task_confirmation_repository.go`：Core-only GORM Repository；Flight、Task、Candidate、Personnel、Assignment、History、Audit、Outbox 和业务幂等写入按用例在 Core transaction 内完成。
- `internal/core/module/{flight,personnel,task}/`：Core 领域事实和值对象；Candidate 与 Assignment 分离，Personnel work state 通过确认事务 Port 进入 `reserved`。
- `internal/edge/sync/`：Edge Inbox/Projection/Command Store 合约、Memory Probe Store、MySQL adapter。
- `internal/integration/sync/`：Core↔Edge Transport Port、HTTP/Memory adapter、RetryPolicy、Worker 和双向 Probe。
- `internal/platform/security/`、`internal/core/module/iam/`：Principal、JWT、RBAC、Scope 和 Audit 基础。
- `internal/platform/health`、`internal/platform/observability`：liveness/readiness、request_id、trace_id 和结构化 HTTP 日志。

Probe 与 `architecture_probe_event` 只服务 dev/test Foundation 验证；BVS2-03 至 BVS2-05 的真实业务入口和代码地图见上方，尚未实现的 BVS2-06 不得由 Probe 代替。

## 长期记忆与验证入口（2026-08-31）

- `memory-bank/project-memory.md` 保存跨会话长期协作约束；`progress.md` 和 `HANDOFF.md` 继续分别保存进度日志与交接状态。
- `scripts/ensure-docker.ps1` 是功能验证的 Docker 前置检查：Docker Engine 未就绪时启动 Docker Desktop 并等待就绪；不会自动启动项目容器或执行任何破坏性数据库操作。
- `scripts/verify.ps1` 是统一验证入口，先执行 Docker 前置检查，再按模式执行 Go 测试、构建、Compose 配置或服务状态检查。

## BVS2-06 Compose 闭环与恢复验收（2026-09-01）

- 隔离 Compose project `bvs206-closure` 使用独立双 MySQL、Core API、Edge API 和 Worker；Core/Edge migration 均应用 `000001`、`000002`，live/ready 健康检查和真实 SQL/HTTP 双向 Probe 通过。
- Worker 重启与 Edge API 暂停/恢复均验证通过：持久化 Outbox/Inbox 不依赖进程内存，失败投递进入 retry，服务恢复后继续收敛；现有本机 `flight-mysql`/`flight-redis` 未被触碰。
- Core/Edge Command Envelope 的时间幂等比较统一按 MySQL `DATETIME(6)` 四舍五入后的持久化精度执行，避免同一 command 在失败重试时被误判为内容冲突。
- 正常 `docker compose up --build` 仍受 Docker Hub 基础镜像元数据 `EOF` 阻塞；当前代码的容器行为已通过交叉编译二进制 + 本地缓存运行时镜像进行隔离验证，不能将该环境 workaround 表述为镜像仓库问题已解决。

## BVS2-06 正式 JWT/mTLS 入口（2026-09-01）

- Core 的 Leader/Manager Confirm、Task Cancel，以及 Edge 的员工 Task/Command API 在正式入口装配 `RequireJWT`；认证结果写入 `security.Principal`，业务 Service 继续负责 RBAC、Scope、状态机和 Human/Machine 边界。
- 开发 Actor Header 只由显式 `allow_dev_actor_headers` 开关启用，且 `release` 配置拒绝该开关；同步 API 不接受浏览器 JWT 作为 Worker 身份替代。
- `TLSConfig` 已接入 Core/Edge Server 和 Worker→Edge HTTP Transport：服务端可要求并验证客户端证书，Worker 可加载私有 CA、客户端证书/私钥并保持 `InsecureSkipVerify=false`；正式部署通过证书路径和环境变量启用，开发 Compose 仍使用 HTTP。
- JWT 不保存完整权限列表；可选角色/结构化 Scope 只作为签名身份属性，授权仍由服务端 Authorizer/Repository 进行。

## 保留的历史现场

旧 B1/B2/B3 现场中的 `internal/module/event/` 和 `migrations/mysql/000003_task_instance_trigger_event_unique.*` 仍保留在工作区，状态为 legacy/paused，不是 Architecture v2 规范，也不应作为新入口或新模块依赖。旧 B3 业务切片说明已从项目文档目录删除；未跟踪的旧 v1 架构草案仍保留在工作区，等待用户对该具体文件是否删除作最终确认。相关历史状态只在本文件和 `memory-bank/progress.md` 中保留摘要。

## Legacy 原型实际状态（暂停，仅供审计；不是 Architecture v2 运行基线）

- 仓库是 Go 原型，已有 Gin 路由、配置加载、Zap 日志、MySQL/Redis 连接、健康检查和 AI 预测桩。
- 当前路由主要包含 `/health`、设备状态预留接口和 `/api/prediction/analyze`。
- 当前 `cmd/server/main.go` 负责配置、日志和依赖连接，已不再在服务启动时调用 `AutoMigrate`；数据库迁移将在独立迁移命令中执行。

## Foundation verification boundary（最终状态见 2026-08-13 验收记录）

- Core/Edge sync SQL stores now persist failed Inbox records outside the rolled-back business transaction and allow failed or stale processing records to retry. Worker command delivery carries attempt metadata and retry time through the Sync API.
- Local Compose builds use `.dockerignore` to exclude temporary caches and database data. Container verification completed on the isolated Compose project `architecture-v2-final`: Core/Edge MySQL migrations, health checks, SQL/HTTP Probe, duplicate handling, Worker restart recovery and Edge unavailable retry/recovery all passed.
- Foundation 验证时尚未实现完整业务服务；后续已按冻结边界完成 BVS2-03 Flight → Task → Candidate，BVS2-04+ 仍未实现。
- Go 1.26.5 已安装；`go test ./...` 和 `go build ./cmd/server` 已通过。
- Docker Compose CLI v5.3.1 可用，Docker Server 29.6.2 已通过 `docker info` 验证；MySQL 8.0.46 与 Redis 7 容器已启动并完成连接验证。
- Compose 的 MySQL 宿主机端口支持 `MYSQL_HOST_PORT` 覆盖；本机因已有 `mysqld` 占用 3306，使用 3307 映射验证，未停止已有进程。

## Legacy 原型目标基础架构（暂停，仅供历史上下文）

```text
HTTP 请求
  ↓
Gin 路由
  ↓
中间件（恢复/日志/CORS/认证/权限）
  ↓
模块服务层
  ↓
数据访问层
  ↓
MySQL（事实数据） + Redis（缓存/轻量队列）
```

横切能力：配置校验、统一错误响应、结构化日志、审计、健康检查、优雅关闭、测试夹具和部署配置。

## Legacy 原型基础阶段的架构决策（暂停，仅供历史上下文）

- 先使用模块化单体，不引入微服务通信和分布式事务。
- MySQL 保存业务事实和状态变更所需数据；Redis 只承担可替换的缓存、去重或轻量队列职责。
- 数据库采用 `migrations/mysql/` 递增版本 SQL，由独立迁移命令执行；服务启动不再调用隐式 `AutoMigrate`。
- 所有写操作最终必须经过后端服务层的权限和状态校验，不能让 AI、前端或脚本直接修改核心数据。
- 业务逻辑实现前，先完成基础层的启动、连接、迁移、认证、测试和 Docker 验证。
- 配置采用 YAML 基线和 `FLIGHT_` 环境变量覆盖；错误响应携带稳定错误码与 request ID；日志不记录密码和 token。
- 认证骨架使用 Bearer JWT 和四类角色（admin/manager/leader/staff），数据范围权限留给业务模块实现。
- 测试分为单元、HTTP 集成和显式开启的数据库/Redis 集成测试，默认不得清空用户已有数据库。

## Legacy 原型已实现的基础文件（暂停入口；v2 文件地图见上文）

- `internal/server/application.go`：集中 HTTP server 的监听、运行上下文、优雅关闭和 MySQL/Redis 资源释放。
- `internal/server/router.go`：提供 `/health/live`、`/health/ready` 和兼容路径 `/health`；MySQL 是 readiness 必需依赖，Redis 为可选依赖。
- `internal/server/router_test.go`：验证无外部依赖时 liveness/readiness 的 HTTP 状态。
- `internal/common/response.go`：提供统一响应写入函数和 request ID 字段，供基础接口复用。
- `internal/config/config.go`：负责 YAML、`FLIGHT_` 环境变量覆盖和启动配置校验。
- `internal/middleware/request_id.go`：生成/复用 request ID，并写入响应头和上下文。
- `internal/store/migrator.go`、`cmd/migrate`、`migrations/mysql/`：负责版本化 SQL 迁移、状态查询和回滚入口。
- `internal/auth/`：提供 JWT claims、签发/校验和角色权限表；不依赖业务模块。
- `internal/middleware/jwt.go`、`internal/module/auth/handler.go`：分别提供 HTTP 认证/权限中间件和最小登录/当前用户接口。
- `internal/testsupport/fixtures.go`：提供 B2 隔离夹具；默认只构造内存数据，显式持久化时使用自增 ID，清理按已创建主键反向删除。

## Legacy 原型基础阶段验证结果（历史记录）

- Go 测试和构建基线已通过。
- Docker Compose 配置、MySQL 数据库创建、Redis 连通性已通过。
- 迁移策略已冻结为独立的版本化 SQL 迁移；`initial_schema` 已在 Docker MySQL 中应用并可查询状态。

## Legacy 原型基础层验收结果（历史记录）

- 程序可在 MySQL/Redis 可用时启动并通过 readiness；依赖不可用时仍可提供 liveness，readiness 明确返回 503。
- `go test ./...`、`go build ./...` 已通过；JWT、角色权限、request ID、配置覆盖和迁移发现器均有测试。
- Docker Compose 的 MySQL/Redis 均已通过 healthcheck；迁移版本 `000001 initial_schema` 在 Docker MySQL 中为 `applied`，重复 `up` 不改变状态。
- 服务启动不创建默认用户；登录成功的数据库夹具留待业务测试阶段定义。

## 业务切片 001 数据结构（legacy/paused，非 v2 Foundation 运行基线）

- `team`、`team_member`：表达班组和员工班组范围。
- `task_candidate`：表达候选人推荐，不等同于确认分配。
- `notification`：记录站内通知和去重键，暂不连接外部推送渠道。
- `audit_log`：记录关键状态、权限操作和操作人。
- `event.idempotency_key`：确保航班到达事件重复提交不重复生成任务。
- `task_template`、`task_instance`、`task_assignment`：增加触发类型、模板版本、班组和确认/接收/完成所需字段。
- 以上结构通过 `migrations/mysql/000002_task_confirmation_foundation` 应用并完成重复迁移验证；业务服务和 API 尚未实现。
- B2 夹具已通过单元测试；默认 `go test ./...` 不连接或清空开发数据库。

## Legacy 待补充（不阻塞 Architecture v2 Foundation）

- 测试数据库隔离和部署清理脚本
- 业务模块依赖方向

## Architecture v2.0 最终容器验收证据（2026-08-13）

- `docker --version`：Docker `29.6.2`；`docker compose version`：Compose `v5.3.1`；`docker info`：Server `29.6.2`、context `docker-desktop`。
- `docker compose -p architecture-v2-final -f deployments/local/docker-compose.yml config --quiet` 通过；五个验收服务均保持运行，两个 MySQL 容器为 healthy。
- Core/Edge migration 分别在各自 MySQL 上从 `pending` 应用到 `applied`；Core API、Edge API、Worker 未共享数据库连接。
- Core/Edge `/health/live` 与 `/health/ready` 均返回 200；Redis 缺失仅显示 unavailable，不影响 readiness 或可靠同步。
- 容器 SQL/HTTP Probe、重复 Event/Command、failed record/retry、Worker 重启和 Edge API 暂时不可用后的恢复均通过。
- 生产 HA、PITR、mTLS、负载均衡等仍是文档化目标，不能从本次开发环境验证外推为生产已达成。

## Phase 2 业务实施边界（2026-08-31）

Architecture Foundation 已结束并冻结；BVS2-01 业务设计、BVS2-02 Core 数据与版本化 Migration、BVS2-03 `Flight → Task → Candidate`、BVS2-04 `Leader Confirm` 和 BVS2-05 `Edge Projection → Employee Command` 代码均已完成。第一条完整业务闭环仍为 `Flight → Task → Personnel → Leader Confirm → Edge → Employee → Complete`，详细入口见 `memory-bank/business-slice-v2-flight-task.md`；下一项为 BVS2-06 完整闭环验收。

后续业务代码必须遵守以下不可变边界：

- Flight、Task、Personnel、Assignment、状态历史、Audit 和业务状态机事实只在 Core；
- Edge 只保存员工执行所需的最小 Projection、Command、Session、Inbox 和 delivery 数据；
- 员工首次使用工号 + 密码认证，个人微信和企业微信身份都绑定到同一个 Core `Staff`；绑定后的任一入口通过 Edge Session 读取同一份 Projection，长会话必须可刷新、可撤销且不能使用永久 JWT；
- 员工接收/完成只能作为 Edge Command 由 Core Worker 拉取并由 Core 权限/状态机校验；
- Core 业务事务、Audit 和 Outbox 必须保持同一事务；同步继续使用既有 at-least-once、Inbox 幂等、Retry/Failed 协议；
- 业务 Handler/Service 不得直接跨模块查表、直接使用 GORM 或绕过 Repository/Application Port；
- 旧 B3 现场不恢复为 v2 入口，任何新业务需求若无法适配冻结边界，必须先提交明确的架构变更评审。

BVS2-01 已进一步冻结：Flight 只提供 `scheduled → arrived` 触发事实，不拥有 Task 生命周期；Candidate 与正式 Assignment 分离；Leader/Manager 在 Area/Team Scope 内确认；员工只能对本人 Assignment 发出 `employee_accept_task.v1` 和 `employee_complete_task.v1`；Core 以 `source_event_id`、`generation_key=flight_public_id + trigger_type` 和 `command_id` 处理业务幂等，并以版本化 Event 更新 Edge Projection。

BVS2-02 已新增 `migrations/core/mysql/000002_core_business_slice_v2.up.sql` 及对应 down 文件，并在隔离 Docker Core MySQL 上实际应用。该 migration 建立 14 张 Core 业务事实、组织前置、状态历史和业务幂等结果表；Core 的 `schema_migrations` 为 `1,2`，Edge 仍为独立的 `1`。

BVS2-03 已在冻结数据模型之上实现 `internal/core/application/flighttask` 和 `internal/core/adapter/mysql`：Core 接收 `scheduled → arrived` 事实，在同一事务内更新 Flight/History、解析启用模板、创建 `awaiting_confirmation` Task、生成 Candidate 快照、写入业务幂等结果、Audit 与 `task.generated.v1` Outbox。Candidate 与 Assignment 保持分离；候选筛选由 Team/Area、主成员关系、岗位、能力、`idle`、启用状态和计划时间冲突共同约束，并按状态变更时间和 `public_id` 稳定排序。Core API 入口为 `POST /api/v1/flights/{flightPublicID}/arrival`。

BVS2-03 的真实验证包括内存 Application/HTTP 测试和隔离 Docker Core MySQL 测试：正常生成、候选过滤/排序、重复来源事件、来源内容冲突、同 generation key 重放、无启用模板、非法 Flight 状态以及 SQL 中的事务结果均已覆盖。

BVS2-04 已实现 `POST /api/v1/tasks/{taskPublicID}/confirm`：只有 Human Principal 且同时满足 Task Team/Area Scope 的 admin/manager/leader 可确认；确认会复核当前 Candidate/Personnel，原子化完成 reserved、Assignment、候选状态、Task `assigned`、三类 History、Audit 与 `task.assigned.v1` Outbox。`confirmation_id` 结果可重放并拒绝内容冲突。BVS2-05 已补齐 Edge Projection 与员工 Accept/Complete Command 代码；正式 JWT 中间件接线和 BVS2-06 完整闭环验收仍待后续完成。
## BVS2-06 Task Cancel 实际代码边界（2026-09-01）

- Core 管理取消入口位于 internal/core/application/flighttask/cancellation.go 与 cancellation_handler.go；Core API 装配在 cmd/core-api/main.go，路由为 POST /api/v1/tasks/{taskPublicID}/cancel。
- 取消是独立的管理用例，不由 Flight 取消自动触发，也不通过 Edge 员工 Command 进入 Core。Core 以 Task 为事实源，在一个事务内锁定 Task、必要时锁定 Assignment/Personnel，再提交状态、History、Audit、Outbox 和幂等记录。
- Edge 已有 task.cancelled.v1 投影处理；取消事件使用 Task sync_version，assigned/in_progress 事件携带原 Assignment/Employee 快照，awaiting 取消携带空 Assignment/Employee。
- 现有 outbox correlation_id 为 CHAR(36)；取消 ID 可达 128 字符，因此短 ID 原样关联，长 ID 通过 task.cancel:<cancellation_id> 的稳定 UUID 派生关联，业务幂等和 Assignment cancellation_id 仍保存原始 ID。

BVS2-06 首个实现步骤已完成；后续仍不得让 Edge 写 Core 事实，也不得恢复旧 B3/B4/B5。
## 2026-09-01 BVS2-06 Task CRUD 读侧

- Core Task CRUD 当前采用受状态机约束的读侧与生命周期模型：Arrival 创建 Task，GET 列表/详情读取 Core 事实，Confirm 负责 Assignment 生命周期，Cancel 负责保留审计事实的删除语义。
- Task Query 使用 Application Query Port 与 MySQL adapter 分离，按 `task:read` 和服务端 Team/Area/Assigned Scope 过滤；Handler 不直接访问 GORM，也不接受任意 SQL 条件。
- `POST /api/v1/tasks`、任意 PATCH 和硬删除暂不实现，等待手工创建、模板绑定、候选计算、幂等和事件契约冻结。

## 2026-09-01 前端 F0 架构冻结门槛

- 前端交付边界固定为三个客户端：`admin-web` 只调用 Core API；`employee-miniapp` 和正式支持的 `employee-web` 只调用 Edge API。所有前端运行时代码、构建配置、Mock、测试和 E2E 位于 `/frontend`。
- 员工小程序同时支持个人微信和企业微信入口；两类外部身份必须由服务端映射到同一个 Core `Staff`，客户端不得把 openid、unionid、企业微信标识或 `X-Employee-Public-ID` 当作生产业务身份。
- F0 的工程验收门槛、当前差异和未决合同统一记录在 `frontend/docs/architecture-freeze-gate.md`。页面路由、线框、视觉 Token、状态矩阵和可访问性已由 `frontend/docs/page-design.md` 冻结；Command 客户端幂等/状态查询、OpenAPI 基础对齐、工具链、员工 Web 真实 API/浏览器联调和管理端企业微信 SSO callback 已有源码与验证，异常恢复 E2E、真实 Provider 线上联调、环境域名和凭据仍待完成。
- F0 身份合同提案位于 `frontend/docs/identity-contract-proposal.md`：外部微信/SSO 身份只用于登录和绑定，平台会话中的员工 `sub` 必须是服务端确认的 `StaffPublicID`；Core/Edge 不接受客户端伪造员工身份，也不把外部身份标识写入业务 Command。
- BVS2-07 身份边界已落地：Core 持有 `employee_credential`、`external_identity_binding` 与一次性 `identity_binding_ticket`；Edge 仅持有最小 `mobile_session`，不直连 Core 身份表。Core 内部身份 Port 由共享密钥保护，开发环境仍只接受显式 `mock:<subject>`；真实 Provider 适配器已支持配置化个人微信 `code2Session` 和企业微信 `gettoken/getuserinfo`，但未配置生产凭据或执行线上联调。

### 真实微信/企业微信与管理端 SSO 接入边界（2026-09-02）

- 个人微信员工入口需要原生小程序：小程序调用 `wx.login()` 获取一次性 code，Core 后端用服务端 AppID/AppSecret 换取平台身份；AppSecret、session_key 不进入小程序或 Edge。
- 企业微信员工入口可以采用企业微信 OAuth/H5 code，也可以配置企业微信容器内的小程序入口；后端统一将企业成员身份映射到 Core `Staff`，同一 Staff 可同时绑定个人微信和企业微信。
- 管理端 SSO 是独立的浏览器企业微信 OAuth 流程（当前不依赖 OIDC，也可在未来增加其他 Provider），不需要开发管理端小程序；浏览器只处理一次性 code，后端负责 state、redirect、企业归属、管理角色校验并签发 Core audience 会话。
- 因为没有 OIDC，员工账号、组织、凭证和微信绑定由 Core MySQL 的员工主数据模块承载；该“员工数据库”由 Core 的组织/人员业务表和 `000003_employee_identity` migration 建立，当前不新增第三套物理数据库，Edge 只保存移动端 Session/Projection/Command。
- 真实 Provider 默认关闭，配置开关为 `identity.real_providers_enabled`；没有凭据、回调域名和管理用户映射时，不得宣称真实登录或 SSO 已完成。
- 员工 Edge Session 使用短期 Access JWT（`sid` + Edge audience）和可撤销 Refresh Token；Refresh Token 只保存 SHA-256 摘要，轮换重放会撤销替换 Session。JWT 解析后由 Edge Session Resolver 校验本地 Session，并向 Core 查询 Staff active 状态；Core/Edge audience 分离。
## 2026-09-01 Employee Command public contract

- Edge employee Accept/Complete routes require a client-generated stable `command_id`; the Edge command store treats trace IDs and delivery timestamps as non-identity metadata while preserving payload/type/actor/aggregate conflict detection.
- `GET /api/v1/commands/{commandID}` is the only public command status read model. It enforces employee ownership and maps internal pending/processing/retry/sent/failed states to `pending/syncing/confirmed/failed`; raw sync errors remain internal.
- This status read model does not change the source-of-truth boundary: Core still validates and applies employee commands, and Edge only reports durable command delivery state until the Core event updates the projection.

## T0 员工任务实时交付与 Edge 扩展决策（2026-09-02）

- 当前 Edge 仍是一个逻辑服务，不按任务类型拆分；本地 Compose 已提供两个相同副本，使用共享 Edge MySQL 和 Gateway，员工请求不感知具体副本。
- 员工获取任务的权威方式是带 JWT Principal 过滤的 `GET /api/v1/tasks`。前台 WebSocket `task_changed` 和 Redis 跨副本 fan-out 都只是提示通道，客户端收到后仍通过 HTTP 快照校准；后台平台通知仍待实施。
- Core→Worker→Edge Inbox/Projection 仍是可靠事实同步链路。Projection 事务提交后才触发尽力而为的通知；推送可以丢失、重复、乱序，不能替代 Projection、Outbox/Inbox 或 Command status。
- 多 Edge 副本只在实例内存维护自己的 WebSocket 连接；共享 SQL ticket 解决连接建立的副本无关性，Redis Pub/Sub 负责尽力转发提示，Redis 不可用时降级为 HTTP 拉取。
- T0 实施顺序固定为：契约冻结、指标基线、拉取恢复契约、Notification Port、前台 WebSocket、Gateway+多 Edge/Worker、后台平台通知、故障/容量门禁。T0 不默认引入 Kafka、RabbitMQ、NATS、Service Mesh 或按任务拆分微服务。

详细 ADR：`docs/adr/ADR-009-employee-task-realtime-delivery.md`。

## T0-1 可观测性实现（2026-09-02）

- Core API、Edge API 和 Worker 暴露内部 `/metrics`；HTTP 指标使用稳定的组件、方法、路由模板和状态标签，避免把任务 ID、员工 ID 或原始 URL 作为高基数标签。
- Core Outbox、Edge Command/Inbox backlog、oldest age、Projection lag、数据库连接池压力，以及 Worker 投递/处理/确认延迟均可采集；队列数量来自持久化 Store，不依赖 Redis 作为可靠状态。
- 该实现只建立观测能力，不代表已经完成代表性业务负载的 p50/p95/p99 性能基线；数值报告需在固定数据规模和负载参数后补齐。

## T0-2 员工任务拉取恢复契约（2026-09-02）

- Edge `GET /api/v1/tasks` 继续返回按 JWT Principal 过滤的完整员工快照，并增加 `snapshot_at`、`sync_mode=full_snapshot`、员工级持久化 `projection_revision`、Projection lag 状态、`next_cursor` 和 `reset_required`。
- `projection_revision` 由 Edge Projection 的新增或更高版本更新递增；同版本幂等重放和过期版本不递增。它是员工范围的变化序列，不是单 Task `sync_version` 的全局替代品；当前增量 API 尚未开启，`next_cursor` 固定为 null。
- HTTP 200 的空 `items` 是合法空快照；取消/完成任务以 Projection 中的终态为准。启动、刷新、重连、推送提示后的校准和离线恢复均重新拉取完整快照，客户端本地缓存不承担可靠恢复。
- 公共 `GET /api/v1/commands/{commandID}` 继续负责稳定 Command ID 的状态查询和重试恢复；任务快照和 Command 状态是两条可分别恢复的读取路径。

## T0-3 通知抽象和提交后边界（2026-09-02）

- Edge 新增 `internal/edge/application/notification` Notification/Fan-out Port；业务 Handler 不直接依赖连接实现。首期 `InMemoryFanout` 只保存当前实例的员工→连接注册表，不保存业务事实。
- `TaskChanged` 是最小 best-effort hint，包含通知 ID、任务公共 ID、任务 `sync_version`、变化原因和签发时间；员工公共 ID 只作为内部路由键，不进入 wire payload。单个连接失败时继续尝试其他同员工连接，并通过指标和结构化日志记录失败。
- `/internal/sync/v1/events` 先完成 Edge Inbox/Projection 的 `ApplyEvent`，只有成功且非重复的任务事件才触发通知；通知构造或投递失败不回滚 Projection，也不改变事件已应用的 `202` 响应。重复 Event 不再次 fan-out。
- 通知不是可靠队列。断线、重启、跨副本未命中或提示丢失时，客户端依靠 `GET /api/v1/tasks` 完整快照和员工级 `projection_revision` 校准；跨副本临时 fan-out 留待 T0-5。

## T0-4 员工 WebSocket 提示实现（2026-09-02）

- Edge 公共实时入口为 `POST /api/v1/realtime/ticket` 和 `GET /api/v1/ws`。ticket 接口要求员工 Bearer JWT；返回的短期一次性 ticket 只用于浏览器 WebSocket 子协议协商，不进入 URL、日志或业务消息。
- WebSocket 握手继续校验 Edge JWT/sid 解析出的 Human Principal、同源 Origin 和 `flight.realtime.v1`；浏览器不能安全设置 Authorization 时，使用 `flight.realtime.ticket.{ticket}` 作为候选子协议，服务端只回显应用协议。
- 连接适配器位于 `internal/edge/application/realtime`，负责员工级订阅、`ready`/`ping`/`pong`、读写超时、空闲关闭、连接清理和 WebSocket 指标。它只持有活跃连接，不保存任务事实。
- `employee-web` 的 `RealtimeClient` 在连接成功、重连和 `task_changed` 后重新读取完整任务快照；notification ID 有界去重并支持指数退避。ticket 401 停止重连并交给会话恢复，WebSocket 不可用不阻塞任务读取和 Command status。
- 代码级测试已覆盖 ticket 单次消费、JWT 员工隔离、握手、通知投递、心跳回复、重复提示去重、断线重连和空闲关闭；真实浏览器、服务重启、网络分区与多副本 fan-out 不在本轮验收结论内。
## T0-5 Gateway 与共享 Edge 多副本实现（2026-09-02）

- 本地 Compose 已增加 `gateway`、`edge-api` 和 `edge-api-2`；宿主机只暴露 Gateway 8082，两个 Edge 副本共享 Edge MySQL，Gateway 同时支持 HTTP/1.1 WebSocket Upgrade。
- Edge realtime ticket 已抽象为 `TicketStorePort`，SQL 实现跨副本共享并只存 ticket hash；没有 Edge DB 时测试仍使用进程内短期 store。
- Core Outbox 与 Edge mobile command 新增 `lease_owner`/`lease_expires_at`。Worker 以唯一 worker ID 领取并携带 owner 确认，过期可重新领取，旧 owner 确认不会覆盖新状态。
- Edge Redis Pub/Sub 仅做跨副本 `task_changed` 提示 fan-out；本地投递先于 Redis 发布，Redis 失败不回滚 Projection，客户端继续依靠 HTTP 全量快照和 `projection_revision` 恢复。
- 代码级租约、Worker 闭环和现有同步测试已通过；Docker CLI 当前不可用，双副本 Compose、迁移、实例停止和 Redis 分区尚未宣称验收通过。
## 2026-09-03 Core management identity and read models

- Core owns management identity provisioning and session state in `admin_identity`, `admin_sso_state`, and `admin_session`; Edge never stores or resolves management roles.
- The management browser uses Core's authorization-code SSO endpoints. State is allow-listed, stored as a hash, consumed once in Core MySQL, and paired with an HttpOnly cookie. Core issues the Core-audience JWT only after resolving an active, provisioned admin/manager/leader identity.
- Each management request re-resolves the current role and Team/Area/User scope from the Core session, so revocation and disablement fail closed without trusting stale JWT role claims.
- Personnel and assignment list reads are application query ports with RBAC and server-derived scope. Query filters are bounded and cannot widen the principal's scope; the MySQL adapter is the only layer that builds the parameterized SQL.
- OIDC credentials, callback domains, identity provisioning, and migration application are deployment work. See `docs/adr/ADR-011-admin-sso-and-management-read-model.md`.
