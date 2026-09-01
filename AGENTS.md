# 项目长期协作规则

本文件只保存跨任务、跨会话长期有效的项目规则。当前任务进度、临时错误、当天计划和短期决定必须写入根目录 `HANDOFF.md` 或 `memory-bank/progress.md`，不得写入本文件。

## 长期记忆与验证前置

- 跨会话长期记忆统一维护在 `memory-bank/project-memory.md`；开始新任务或写代码前必须读取它，并结合 `memory-bank/architecture.md`、`memory-bank/design-document.md`、`memory-bank/implementation-plan.md` 和 `memory-bank/progress.md` 判断当前边界。
- 任何功能验证、集成验证、数据库验证或 Compose 验证前，必须先运行 `powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1`；Docker Desktop 未运行时由脚本自动启动并等待 Docker Engine 就绪。
- 推荐使用 `powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all` 作为统一验证入口。该入口只负责打开并检查 Docker，不自动执行 `docker compose up`、Migration、Volume 删除、数据库清理或其他破坏性操作。
- Docker CLI/Desktop 不可用时必须让验证明确失败并记录原因，不得静默跳过 Docker 前置检查。

## Architecture v2.0 技术栈与项目结构

- 后端使用 Go、Gin、GORM、MySQL、Redis、Viper、Zap 和 JWT；Core 继续是模块化单体，不拆 flight/task/personnel/event/rule 微服务。
- 运行程序为 `cmd/core-api`、`cmd/edge-api`、`cmd/worker`、`cmd/migrate`。旧 `cmd/server` 和旧业务目录是迁移期间的 legacy/paused 现场，不是 v2 生产入口。
- 新边界为 `internal/core`、`internal/edge`、`internal/integration`、`internal/platform`、`internal/shared`；Handler 不调用 GORM，Service 不依赖 Gin，Domain 不依赖 Gin/GORM/Redis。
- Core/Edge 使用完全独立的 MySQL、账号、schema 和 migration：`migrations/core/mysql`、`migrations/edge/mysql`。服务启动不得调用 GORM `AutoMigrate`。
- Core 是航班、人员、岗位、能力、人员状态、任务模板/实例/分配、事件、规则、审计和状态机的唯一事实源；Edge 只能保存 Projection、Command、Session、Inbox、delivery 等移动端最小数据。
- Redis 只承担 cache、rate limit、短锁、短期去重、在线状态和临时状态；可靠同步不能依赖 Redis。
- 系统只服务单机场；禁止 Tenant/Airport、多租户、多机场、`tenant_id`、`airport_id`、airport scope 和机场切换。
- Architecture Foundation 已 `ACCEPTED / FROZEN`；业务实现从 Phase 2 的 Business Slice v2 开始，旧 B3/B4/B5 现场保持 legacy/paused，不直接恢复。

## 构建、测试与检查命令

- 默认单元/HTTP 测试：`go test ./...`
- 默认构建检查：`go build ./...`；服务专项构建：`go build ./cmd/core-api`、`go build ./cmd/edge-api`、`go build ./cmd/worker`
- 统一验证入口：`powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all`
- 分项验证入口：`scripts/verify.ps1 -Mode test|build|compose-config|compose-ps`；所有入口都会先确保 Docker Engine 已就绪
- Core 迁移状态/应用：`go run ./cmd/migrate -target core -command status|up`
- Edge 迁移状态/应用：`go run ./cmd/migrate -target edge -command status|up`
- Compose 配置检查：`docker compose -f deployments/local/docker-compose.yml config`
- 本地依赖健康检查：`docker compose -f deployments/local/docker-compose.yml ps`
- 修改 Go 文件后必须运行 `gofmt`；修改后必须检查 `git diff --check` 和 `git status`。
- 测试未实际运行或失败时，不得在文档或交接中写成“已通过”。

## 代码风格与实现边界

- 遵循 Go 惯用风格，提交前格式化；错误要保留上下文，优先使用 `fmt.Errorf("...: %w", err)`。
- 请求处理使用 `context.Context`；资源由创建它的生命周期统一关闭。
- handler 不直接承载复杂业务规则；业务状态转换、权限和数据一致性必须在 service/store 事务边界内校验。
- 所有写操作必须经过后端权限和状态校验；前端、脚本或 AI 不得直接修改核心数据。
- API 错误使用稳定错误码、用户可读消息和 request ID；日志使用结构化字段，不记录密码、JWT secret、完整 token 或其他敏感凭据。
- 配置使用 YAML 基线和 `FLIGHT_` 前缀环境变量覆盖；真实密钥、密码和本地配置不得提交到仓库。
- 不在基础设施代码中提前实现未经设计确认的业务规则；每个业务步骤必须有对应设计、验收条件和测试。

### 代码注释规范

- 编写或修改代码时，根据实际需要添加注释；不要求对所有代码逐行添加注释。
- 注释优先用于：核心业务逻辑、关键算法或重要处理流程；意图不直观、可能产生理解歧义的代码；特殊判断、边界条件、兼容性处理或异常处理；看似可以简化或删除但因特定原因必须保留的实现；关键参数、数据转换、状态变化或容易误用的逻辑。
- 命名清晰、逻辑简单且自解释性强的代码，无需为了增加注释而添加注释。
- 注释重点说明“为什么这样做”以及背后的意图、约束和注意事项，避免重复描述代码已经表达的“做了什么”。
- 注释应保持简洁、准确；代码逻辑发生变化时，必须同步更新相关注释。

## 必须先确认的操作

未经用户明确确认，不得执行以下操作：

- 删除、覆盖、批量重命名用户文件或用户未提交修改。
- 执行会删除数据的迁移回滚、`docker compose down -v`、数据库清空、`TRUNCATE` 或生产数据清理。
- 停止或修改用户已有的本机服务，例如占用 3306 的 MySQL；应优先使用可配置端口。
- 提交、创建 commit、切换/删除分支、push、创建 Pull Request 或向外部系统发送消息。
- 接入真实外部推送、AI 模型、生产账号或外部数据源。

只读检查、项目内正常构建/测试、创建可清理的临时测试夹具和实现用户明确要求的代码改动，不需要额外确认，但仍必须保留用户已有修改。

## 任务完成验收标准

- 需求对应的代码、配置、迁移或文档已经实际存在；没有把计划或设计描述当作实现。
- 相关单元/集成测试和构建命令已运行，并在交接中记录真实结果。
- `git diff --check` 通过，工作区差异经过检查，没有意外文件或敏感信息。
- 数据库迁移可追踪；破坏性操作未被默认执行。
- 重要架构或模块变化同步到 `memory-bank/architecture.md`，当前进度同步到 `memory-bank/progress.md`。
- 用户准备结束长会话时，根目录 `HANDOFF.md` 已按当前实际状态更新。

## 上下文不完整时的恢复规则

- 如果上下文因压缩、中断、摘要或信息缺失而不完整，不能只依赖对话摘要或 `HANDOFF.md`。
- 继续工作前必须检查当前工作目录、`git rev-parse --show-toplevel`、当前分支、远程、`git status`、`git diff`、未跟踪文件、相关测试结果和关键文档。
- 必须重新读取 `memory-bank/architecture.md`、`memory-bank/design-document.md`、`memory-bank/implementation-plan.md` 和 `memory-bank/progress.md`，再核对 `HANDOFF.md`。
- 必须检查待修改文件是否已有其他任务的修改；发现重叠或无法判断归属时，保留现场并先报告，不覆盖。
- 文档与代码、差异或测试结果矛盾时，以可验证的代码、修改差异和测试结果为准，并把矛盾记录到新的交接状态中。

## 长会话交接规则

- 当用户表达“我要开新线程了”或等价意思时，必须更新 `AGENTS.md`（仅在确有新的长期规则时）和 `HANDOFF.md`（本次任务状态）。
- 新线程接手前必须重新检查：哪些文件已修改、实际差异是否符合交接描述、是否存在未提交或意外修改、测试是否真的通过、文档与实现是否一致、是否有其他任务同时修改相关文件。
- 如果交接写着“功能已完成”但测试失败，必须相信测试结果，修正交接描述，不得继续宣称完成。

### 结束当前对话时的 GitHub 同步

- 当用户表达“我要结束当前这个对话了”或语义相近的结束会话意思时，除完成上述长会话交接外，自动执行一次安全的 GitHub 同步。本条是用户明确授予的提交和推送授权，仅在该触发语义出现时生效。
- 同步前必须先检查当前分支、远程、`git status`、`git diff` 和未跟踪文件；默认同步当前工作区中已核对、属于本项目且不含敏感信息的修改。
- 不得提交密码、JWT secret、Token、私钥、生产配置、本地数据库文件、Volume、缓存、临时构建产物或无法确认归属的用户文件。发现风险或归属不明时，保留现场并报告，不能为了同步而强行加入。
- 需要先更新 `HANDOFF.md`，按实际结果运行适用的检查；任何功能、集成、数据库或 Compose 验证仍必须先执行 `scripts/ensure-docker.ps1`。
- 同步使用当前分支的 `origin` 远程，创建一次说明清楚的 commit 并 push；不得 force push、重置、丢弃本地修改、切换分支或自动创建 Pull Request。
- Push 成功后必须再次核对远程提交 SHA 和工作区状态；没有实际 commit/push 成功时，不得写成“已同步”。如果远端领先、发生冲突或 push 失败，不得自动覆盖或重置，必须保留现场并报告原因。
