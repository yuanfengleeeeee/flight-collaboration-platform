# 项目长期协作规则

本文件只保存跨任务、跨会话长期有效的项目规则。当前任务进度、临时错误、当天计划和短期决定必须写入根目录 `HANDOFF.md` 或 `memory-bank/progress.md`，不得写入本文件。

## 技术栈与项目结构

- 后端使用 Go、Gin、GORM、MySQL、Redis、Viper、Zap 和 JWT。
- 项目采用模块化单体；HTTP handler、业务 service、repository/store、model 和公共能力分层。
- `cmd/server` 只负责程序入口和生命周期；`cmd/migrate` 只负责数据库迁移命令。
- `internal/config` 负责配置加载与校验；`internal/server` 负责 HTTP 服务和路由装配；`internal/middleware` 负责日志、恢复、request ID、认证和权限。
- `internal/store` 负责数据库/Redis 连接和迁移支持；`internal/model` 负责持久化模型；`internal/module` 负责领域模块；`internal/testsupport` 负责隔离测试夹具。
- 数据库结构必须使用 `migrations/mysql/` 中的递增版本 SQL 和独立迁移命令管理；服务启动不得隐式调用 GORM `AutoMigrate`。
- MySQL 是业务事实来源；Redis 只能承担可替换的缓存、幂等或轻量队列职责，核心一致性不能依赖 Redis。

## 构建、测试与检查命令

- 默认单元/HTTP 测试：`go test ./...`
- 默认构建检查：`go build ./...`；服务专项构建：`go build ./cmd/server`
- 迁移状态：`go run ./cmd/migrate -command status`
- 应用迁移：`go run ./cmd/migrate -command up`
- Compose 配置检查：`docker compose -f deployments/docker-compose.yml config`
- 本地依赖健康检查：`docker compose -f deployments/docker-compose.yml ps`
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
