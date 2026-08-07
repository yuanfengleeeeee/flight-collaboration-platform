# Task Handoff

> 更新时间：2026-08-07
>
> 本文件只记录本次任务的实际状态，不替代工作区、代码差异和测试结果。新线程必须先检查工作区，再核对本文件；如有矛盾，以实际可验证状态为准。

## 任务目标

建立可持续的项目基础架构，并实现第一个业务垂直切片“航班到达后的任务确认闭环”：航班到达事件 → 任务生成 → 候选员工匹配 → 班组长确认 → 员工接收/完成。

当前验收目标是：基础层可构建、可测试、可迁移、可启动；业务切片按 B1-B5 分步实现，每步都有独立测试，不跨步实现。

## 已完成内容

- 基础层已完成并验证：Go/Gin 服务生命周期、配置校验和环境变量覆盖、MySQL/Redis 连接、版本化迁移、健康检查、结构化日志、统一响应、request ID、JWT、角色权限和 Docker Compose 健康检查。
- 已完成迁移 `000001_initial_schema` 和 `000002_task_confirmation_foundation`；后者新增班组/成员、任务候选、通知、审计和航班到达幂等字段。
- 已完成业务 B1：切片数据模型和迁移；已完成业务 B2：隔离测试夹具和清理策略。
- B2 夹具覆盖合格员工、跨班组员工、能力不足员工和忙碌员工；默认测试只构造内存数据，显式持久化使用自增 ID，清理按已创建主键反向删除，不使用 `TRUNCATE`。
- 已完成认证骨架接口：`/api/v1/auth/login`、`/api/v1/auth/me`；没有默认生产账号或用户管理功能。

## 当前工作区状态

- Git 分支：`main`，跟踪 `origin/main`。
- 存在未提交改动，且当前工作区的基础层、B1、B2 改动均属于本任务范围；未执行 commit 或 push。
- 已修改文件：`README.md`、`cmd/server/main.go`、`deployments/docker-compose.yml`、`go.mod`、`internal/common/response.go`、`internal/config/config.go`、`internal/middleware/common.go`、`internal/middleware/jwt.go`、`internal/model/event.go`、`internal/model/task.go`、`internal/server/router.go`、`tech-stack.md`。
- 未跟踪文件/目录：`AGENTS.md`、`HANDOFF.md`、`cmd/migrate/`、`internal/auth/`、`internal/config/config_test.go`、`internal/middleware/jwt_test.go`、`internal/middleware/request_id.go`、`internal/model/audit.go`、`internal/model/notification.go`、`internal/model/team.go`、`internal/module/auth/`、`internal/server/application.go`、`internal/server/router_test.go`、`internal/store/migrator.go`、`internal/store/migrator_test.go`、`internal/testsupport/`、`memory-bank/`、`migrations/`。
- 本机已有 MySQL 占用宿主机 3306；项目 Docker MySQL 验证使用 `MYSQL_HOST_PORT=3307`，未停止本机服务。
- 当前没有独立提交边界，Git 无法仅凭状态证明是否有其他任务同时改过这些未提交文件；新线程处理重叠文件前必须重新核对差异和上下文。

## 关键决定及原因

- 采用模块化单体，先保持单仓和清晰模块边界，避免过早微服务化。
- 服务启动不隐式迁移，使用 `migrations/mysql/` 和独立迁移命令，便于追踪和审查。
- MySQL 是业务事实来源，Redis 只用于可替换的缓存/幂等/轻量队列。
- 首个业务切片先逐任务由班组长确认，确认后才创建站内通知；暂不做批量确认、真实推送、定位硬约束、AI 或自动调度。
- 候选硬约束为班组、岗位/能力和空闲状态；第一版位置仅预留，不作为硬约束。
- 不创建默认生产账号；测试账号只由隔离夹具显式创建。

## 未完成事项

- 业务 B3：实现 `flight_arrived` 事件幂等处理和任务实例生成。
- 业务 B4：实现候选匹配、班组长确认/拒绝和并发唯一分配。
- 业务 B5：实现员工查看通知、接收、开始和完成任务。
- 登录成功的数据库测试夹具、用户管理和初始化账号策略仍未完成。
- 真实推送渠道、AI、定位硬件、航班取消/登机口变更等不属于当前切片。

## 当前阻塞与风险

- 没有业务代码阻塞。
- 本次交接核对时，当前 PowerShell 找不到 `docker` 命令，标准路径 `C:\Program Files\Docker\Docker\resources\bin\docker.exe` 也不存在，因此不能把此前的 Docker 健康状态视为当前状态；新线程需先确认 Docker/Compose 是否可用以及容器是否仍在运行。
- `go test ./...` 本次退出码为 0 并通过；`go build ./...` 本次退出码为 0，但 Go 报告无法写入用户级模块缓存统计临时文件（`C:\Users\yuanfengleeeeee\go\pkg\mod\cache\download\...`）的权限警告。若需要干净构建结果，应在可写的 Go 缓存配置下重新运行并记录结果。
- 数据库真实 `down` 尚未对现有 Docker 数据卷执行，因为会删除表；迁移文件、状态追踪和回滚入口已实现，真实回滚应在隔离测试库验证。
- 现有模型与业务切片的状态机仍需在 B3-B5 中通过 service 和事务实现，不能把迁移字段误认为业务流程已完成。

## 已尝试但失败的方法

- 直接在宿主机 3306 启动 Docker MySQL 失败，因为本机已有 `mysqld` 占用端口；已改为 Compose 支持 `MYSQL_HOST_PORT`，使用 3307 验证。
- 本次尝试运行 `docker compose -f deployments/docker-compose.yml ps` 失败，因为当前 PowerShell 找不到 Docker CLI；随后检查默认安装路径也未找到可执行文件，不能据此判断容器状态。
- 曾尝试在现有 Docker 数据卷执行真实 `down` 迁移，被安全检查阻止；原因是无法在未明确授权下假定数据可删除。后续使用隔离测试库验证，不要绕过该限制。
- 服务启动时的 GORM `AutoMigrate` 已移除；不能重新加回作为业务迁移替代方案。

## 下一步最小动作

执行业务 B3 的第一小步：重新读取 `memory-bank/business-slice-001.md`、`memory-bank/architecture.md`、`memory-bank/implementation-plan.md` 和现有模型，设计 `flight_arrived` 事件输入、幂等查询和任务生成事务；先写服务层/单元测试，再写 API。

## 需要重新检查的文件、测试和文档

- `AGENTS.md`、本文件、`memory-bank/progress.md`、`memory-bank/architecture.md`、`memory-bank/business-slice-001.md`、`memory-bank/implementation-plan.md`。
- `internal/model/`、`internal/testsupport/`、`migrations/mysql/000001_initial_schema.*`、`migrations/mysql/000002_task_confirmation_foundation.*`。
- `git status --short --branch`、`git diff`、`git diff --check`。
- `go test ./...`、`go build ./...`、`go run ./cmd/migrate -command status`；本次已实际运行前两项，结果见“当前阻塞与风险”。
- `docker compose -f deployments/docker-compose.yml ps`，确认本地依赖实际状态；本次未能执行成功，原因见“已尝试但失败的方法”。
