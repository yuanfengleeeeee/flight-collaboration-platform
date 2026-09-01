# Architecture v2 代码导览

这份文档回答两个问题：一个文件为什么存在，以及其中的函数负责什么。

## 先记住三条主线

```text
cmd/*
  负责启动进程和组装依赖

internal/platform + internal/shared
  负责通用基础设施、协议和标识符

internal/core / internal/edge / internal/integration
  负责 Core、Edge 和两者之间的同步边界
```

Architecture Foundation 已 `ACCEPTED / FROZEN`；BVS2-01 业务设计和 BVS2-02 数据基线已完成，BVS2-03 `Flight → Task → Candidate` 已落地。`Probe` 仍是专门验证架构的测试样例，不是生产业务；下一步是 BVS2-04 Leader Confirm。旧 B3 现场不作为 v2 入口。

## 一、从哪里开始读

建议顺序：

1. `cmd/core-api/main.go`：Core 进程如何启动。
2. `cmd/edge-api/main.go`：Edge 进程如何启动。
3. `cmd/worker/main.go`：同步 Worker 如何启动。
4. `internal/shared/event/envelope.go`：Core 和 Edge 交换什么消息。
5. `internal/core/sync/`：Core 如何保存 Outbox、Inbox 和事务。
6. `internal/edge/sync/`：Edge 如何保存 Projection、Command 和 Inbox。
7. `internal/integration/sync/`：Worker 如何把两边连接起来。
8. `internal/platform/`：配置、数据库、健康检查、日志和安全基础。

## 二、程序入口 `cmd/`

### `cmd/core-api/main.go`

Core API 的进程入口，只负责组装，不负责业务规则。

- `main`：读取配置，创建 Zap 日志，连接 Core MySQL 和可选 Core Redis，创建 `core/application.Server`，注册信号并启动服务。数据库或 Redis 连接失败时保留进程 liveness；真正是否 ready 由健康检查报告。

### `cmd/edge-api/main.go`

Edge API 的进程入口。

- `main`：读取配置，连接 Edge MySQL 和可选 Edge Redis；数据库可用时创建 Edge SQL Store；把 Store 注入 Edge Server，然后启动 HTTP 服务。它只连接 Edge 数据库，不连接 Core 数据库。

### `cmd/worker/main.go`

Core 侧同步进程入口。

- `main`：只打开 Core MySQL，创建 Core Store 和 Edge HTTP Transport，组装 `integration/sync.Worker`，按固定间隔执行 `DeliverOutbox` 和 `PullCommands`。Worker 通过 Edge API 访问 Edge，不直接连接 Edge MySQL。

### `cmd/migrate/main.go`

独立的数据库迁移入口。

- `main`：解析 `-target core|edge`、`-command up|down|status`，选择对应数据库配置和 migration 目录，调用 MySQL Migrator。`down` 必须显式传入 `-allow-destructive`。
- `fail`：打印带上下文的错误并以退出码 1 结束进程。

## 三、Core 应用层 `internal/core/application/`

### `boundary.go`

- `Boundary`：Core 应用边界的占位描述。
- `NewBoundary`：返回名称为 `core-modular-monolith` 的 Core 边界对象。当前用于表达架构边界，未来会由真实业务 Application Service 使用。

### `server.go`

Core HTTP Server 的生命周期和 Foundation 路由。

- `Server`：保存 Core 配置、GORM DB、Redis、日志、HTTP Server 和关闭状态。
- `NewServer`：创建健康检查、request/trace 日志中间件和当前 Foundation 路由。
- `Handler`：返回底层 `http.Handler`，主要供 HTTP 测试使用。
- `Run`：监听端口、启动 HTTP 服务；收到 Context 取消时执行优雅关闭。
- `Shutdown`：只执行一次，依次关闭 HTTP、MySQL 和 Redis 资源。

Core 当前除健康检查和 `/api/v1/foundation` 外，还提供 `POST /api/v1/flights/{flightPublicID}/arrival`。该入口只负责把 HTTP 输入交给 Flight → Task → Candidate Application Use Case，真实写入仍由 Core MySQL Repository 的事务完成。

### `flighttask/`

这是 BVS2-03 的跨模块 Application Use Case：Flight 到达事实触发 Task 生成和 Candidate 快照。Service 不依赖 Gin/GORM；MySQL 事务适配器位于 `internal/core/adapter/mysql/flight_task_repository.go`。该切片不会创建 Assignment，也不会向 Edge 写入 Task Projection。

### `foundation_command.go`

只处理 Architecture Probe 的测试命令。

- `HandleFoundationCommand`：只接受 `probe.complete.v1`；写入 Probe Audit 和一个新的 Projection Outbox Event。它不会完成真实任务，也不会修改未来的业务聚合。
- `HandleCommand`：保留 Foundation Probe，同时把 `employee_accept_task.v1` / `employee_complete_task.v1` 路由到 Flight Task 员工命令状态机。
- `jsonPayload.MarshalJSON`：把已经是 JSON 的 payload 原样写回，空 payload 则写成 `{}`，避免 JSON 被二次编码。

### `worker.go`

这是早期的 Core Worker 生命周期骨架，不是当前真正执行同步的 Worker。真正的同步循环在 `cmd/worker` 加 `internal/integration/sync/worker.go`。

- `Worker`：保存 Core DB、日志和轮询间隔。
- `NewWorker`：创建生命周期骨架并设置默认轮询间隔。
- `Run`：周期检查 Core MySQL 是否可用。
- `Shutdown`：关闭 Core MySQL。

### `server_test.go`

- `TestHealthEndpointsWithoutMySQL`：验证没有 MySQL 时 liveness 仍返回 200，而 readiness 返回未就绪。

## 四、Core 同步存储 `internal/core/sync/`

这里是 Core 可靠同步的核心。接口先定义“需要什么”，SQL Store 和 Memory Store 再分别实现。

### `store.go`

- `AuditRecord`：一次业务操作的审计信息。
- `ProbeEvent`：Foundation Probe 使用的最小业务记录。
- `OutboxRecord`：待投递的 Core Event，加上状态、尝试次数和下次重试时间。
- `CoreTransaction`：Core 事务允许业务写入的能力，目前包括 Probe、Audit 和 Outbox。
- `CommandExecution`：Core 处理一个 Edge Command 的函数类型。
- `Store`：Core Store 的统一接口。
- `NewMemoryStore`：创建内存 Store，供单元测试和 Probe 使用。
- `MemoryStore.RunTransaction`：用快照模拟事务；回调失败时恢复 Probe、Audit 和 Outbox。
- `ClaimPendingOutbox`：领取到期的 pending/retry/processing Outbox，并标记为 processing。
- `MarkOutboxSent`：投递成功后将 Outbox 标记为 sent。
- `MarkOutboxRetry`：投递失败但还有预算时，记录错误、尝试次数和下一次时间。
- `MarkOutboxFailed`：重试预算耗尽后将消息标记为 failed。
- `ProcessCommand`：按 `command_id` 去重，调用业务处理器，并把处理过程和 Audit/Outbox 放入同一模拟事务。
- `PendingOutbox`：测试辅助方法，读取当前 Outbox。
- `ProbeEvents`：测试辅助方法，读取 Probe 记录。
- `AuditRecords`：测试辅助方法，读取 Audit 记录。
- `updateOutbox`：统一更新内存 Outbox 状态。
- `memoryTransaction.CreateProbeEvent`：在当前模拟事务中写入 Probe，并检查重复 public ID。
- `memoryTransaction.AppendAudit`：在当前模拟事务中追加 Audit。
- `memoryTransaction.AppendOutbox`：校验 Event 并写入 pending Outbox。
- `cloneProbe`：复制 Probe map，用于事务回滚快照。
- `cloneOutbox`：复制 Outbox map，用于事务回滚快照。

### `sql_store.go`

SQL Store 把同一套能力落到 Core MySQL。

- `NewSQLStore`：用 GORM DB 创建 SQL Store。
- `RunTransaction`：开启 GORM 事务，把 `sqlTransaction` 交给业务回调。
- `ClaimPendingOutbox`：使用 `FOR UPDATE SKIP LOCKED` 并发领取待投递 Outbox，避免多个 Worker 重复领取。
- `MarkOutboxSent`：更新 Outbox 为 sent。
- `MarkOutboxRetry`：更新为 retry，并递增 attempts。
- `MarkOutboxFailed`：更新为 failed，并记录最后错误。
- `ProcessCommand`：先写/检查 `core_inbox`，再在同一事务内执行 Command；重复 Command 不再重复执行业务。
- `recordFailedCommand`：当业务事务回滚后，在独立事务中保留 failed Inbox，保证失败原因不会随业务回滚丢失。
- `updateOutbox`：统一更新 SQL Outbox，并在没有匹配记录时返回 `ErrNotFound`。
- `sqlTransaction.CreateProbeEvent`：写入 `architecture_probe_event`。
- `sqlTransaction.AppendAudit`：写入 `audit_log`。
- `sqlTransaction.AppendOutbox`：校验 Event 并写入 `outbox_event`。
- `auditRow.TableName`：把 GORM 行映射到 `audit_log`。
- `probeEventRow.TableName`：把 GORM 行映射到 `architecture_probe_event`。
- `outboxRow.TableName`：把 GORM 行映射到 `outbox_event`。
- `newOutboxRow`：把 Event Envelope 转成数据库行。
- `outboxRow.outboxRecord`：把数据库行还原成程序使用的 OutboxRecord。
- `coreInboxRow.TableName`：把 GORM 行映射到 `core_inbox`。
- `newCoreInboxRow`：把 Command Envelope 转成 Core Inbox 行。
- `mapDatabaseError`：把 GORM 重复键错误转换为同步层的 `ErrDuplicate`。

### `store_test.go`

- `TestMemoryStoreTransactionRollsBackAllFoundationWrites`：验证事务失败时 Probe、Audit、Outbox 一起回滚。
- `TestMemoryStoreCommandIsIdempotent`：验证重复 Command 只执行一次。
- `TestMemoryStoreRetriesFailedCommand`：验证失败 Command 可以再次处理。
- `rollbackError.Error`：测试用错误类型。

## 五、Core IAM `internal/core/module/iam/`

### `rbac.go`

- `defaultRolePermissions`：定义四个角色的基础权限。
- `Authorizer`：保存角色到权限的映射。
- `NewAuthorizer`：复制默认权限表，避免调用方修改全局模板。
- `Authorizer.Authorize`：同时检查 Principal 类型、角色权限和 Scope。
- `Authorizer.hasPermission`：判断角色列表中是否至少一个角色拥有目标权限。
- `machinePermissionAllowed`：限制机器身份只能使用 `sync:*` 或 `device:*` 权限。

### `audit.go`

- `AuditWriter`：审计写入能力的接口。
- `WriteAudit`：校验审计字段，并把 Audit 写入当前 Core 事务，而不是只写普通日志。

### `errors.go`

- `ErrForbidden`：统一的权限拒绝错误。

### `rbac_test.go`

- `TestRBACAndScope`：验证角色权限和结构化 Scope。
- `TestMachinePrincipalCannotUseHumanRole`：验证机器身份不能伪装成人员角色。

## 六、Edge 应用层 `internal/edge/application/`

### `boundary.go`

- `Boundary`：Edge 应用边界的占位描述。
- `NewBoundary`：返回名称为 `edge-projection-gateway` 的 Edge 边界对象。

### `server.go`

- `Server`：保存 Edge 配置、Edge DB、Redis、Projection/Command Store、日志和 HTTP Server。
- `NewServer`：创建不带具体 Store 的 Edge Server，主要用于测试或无数据库场景。
- `NewServerWithStore`：创建健康检查、Projection 查询、Command 写入和内部同步路由。
- `projectEvent`：只识别当前 Probe Projection Event，解码 payload 后调用 Store 更新 Projection。
- `Handler`：返回 HTTP Handler，供测试使用。
- `Run`：监听 Edge 端口并响应 Context 取消。
- `Shutdown`：只执行一次，关闭 HTTP、Edge MySQL 和 Edge Redis。

`NewServerWithStore` 中的路由职责如下：

- `/api/v1/tasks`：读取 Edge 中的任务投影。
- `/api/v1/tasks/{taskPublicID}/accept` 与 `/complete`：从 `X-Employee-Public-ID` 生成 pending 员工 Command；Edge 不直接修改最终业务状态。
- `/api/v1/commands`：校验并持久化移动端 Command，返回 pending。
- `/internal/sync/v1/events`：接收 Core Event，先写 Edge Inbox，再更新 Projection。
- `/internal/sync/v1/commands/pending`：给 Core Worker 拉取待处理 Command。
- `/internal/sync/v1/commands/:commandID/ack`：接收 Worker 对 Command 的 applied/retry/failed 回执。

### `server_test.go`

- `TestHealthEndpointsWithoutMySQL`：验证 Edge 在没有 MySQL 时仍能提供 liveness，并正确报告 readiness。

## 七、Edge 同步存储 `internal/edge/sync/`

### `store.go`

- `TaskProjection`：员工移动端看到的最小任务字段。
- `InboxRecord`：Edge 接收 Core Event 后的处理记录。
- `CommandRecord`：Edge 保存的移动端 Command 及重试信息。
- `EventProjection`：收到 Event 后更新 Projection 的函数类型。
- `Store`：Edge Store 统一接口。
- `NewMemoryStore`：创建内存 Edge Store。
- `MemoryStore.ApplyEvent`：按 Event ID 幂等接收 Event，执行 Projection 回调，并记录 applied/failed。
- `MemoryStore.PutCommand`：按 Command ID 去重并保存 pending Command。
- `MemoryStore.ClaimPendingCommands`：领取到期的 Command，并标记 processing。
- `MemoryStore.MarkCommandSent`：将 Command 标记为已发送/已处理。
- `MemoryStore.MarkCommandRetry`：记录 Command 重试时间、次数和错误。
- `MemoryStore.MarkCommandFailed`：将 Command 标记为失败。
- `MemoryStore.UpsertTaskProjection`：按 public ID 写入或更新投影；旧版本不能覆盖新版本。
- `MemoryStore.ListTaskProjections`：按员工 public ID 过滤并读取任务投影。
- `MemoryStore.InboxRecord`：测试辅助方法，读取 Inbox 记录。
- `MemoryStore.CommandRecord`：测试辅助方法，读取 Command 记录。
- `updateCommand`：统一更新内存 Command 状态。

### `sql_store.go`

- `NewSQLStore`：创建 Edge SQL Store。
- `ApplyEvent`：在 Edge MySQL 事务中写 Inbox、执行投影并标记 applied。
- `recordFailedEvent`：Event 投影事务失败后，在独立事务中保留 failed Inbox。
- `PutCommand`：把移动端 Command 写入 `mobile_command`，重复键返回 duplicate。
- `ClaimPendingCommands`：使用行锁和 `SKIP LOCKED` 领取待处理 Command。
- `MarkCommandSent`：把 Command 标记为 sent。
- `MarkCommandRetry`：把 Command 标记为 retry，并递增 attempts。
- `MarkCommandFailed`：把 Command 标记为 failed。
- `UpsertTaskProjection`：按同步版本更新 `task_projection`，旧版本不会覆盖新版本；如果处于同步事务中，则复用该事务。
- `ListTaskProjections`：按员工筛选并读取投影。
- `updateCommand`：统一更新 SQL Command 状态。
- `syncInboxRow.TableName`：映射 `sync_inbox`。
- `newSyncInboxRow`：把 Event Envelope 转成 Edge Inbox 行。
- `taskProjectionRow.TableName`：映射 `task_projection`。
- `taskProjectionRow.projection`：把数据库行转换为 TaskProjection。
- `mobileCommandRow.TableName`：映射 `mobile_command`。
- `newMobileCommandRow`：把 Command Envelope 转成数据库行。
- `mobileCommandRow.commandRecord`：把数据库行转换为 CommandRecord。

### `store_test.go`

- `TestMemoryStoreAppliesDuplicateEventOnce`：验证重复 Event 只投影一次。
- `TestMemoryStoreRetriesFailedEvent`：验证失败 Event 可以重新投影。
- `TestMemoryStoreCommandDeduplicationAndProjectionVisibility`：验证 Command 去重和 Projection 查询。

## 八、同步集成层 `internal/integration/sync/`

### `ports.go`

- `EdgeTransport`：Worker 需要的传输能力，包括发送 Event、拉取 Command、确认 Command。
- `CoreCommandHandler`：抽象 Core Command 处理器。当前真正使用的是 `coresync.CommandExecution`。

### `http_transport.go`

- `HTTPTransport`：通过 HTTP 访问 Edge API 的生产边界适配器。
- `NewHTTPTransport`：校验 Edge Base URL 并创建带超时的 HTTP 客户端。
- `PublishEvent`：调用 Edge Event 接口。
- `PullCommands`：调用 Edge Pending Command 接口并解码返回值。
- `AcknowledgeCommand`：向 Edge 回传 applied/retry/failed 状态。
- `doJSON`：统一发送 JSON 请求、检查 HTTP 状态码并处理响应。

### `memory_transport.go`

Foundation Probe 使用的内存传输适配器。

- `MemoryTransport`：把 Edge Store 伪装成一个可开关的传输边界。
- `NewMemoryTransport`：创建内存传输。
- `SetAvailable`：模拟网络或 Edge 暂时不可用。
- `PublishEvent`：直接调用 Edge Store 的 ApplyEvent。
- `PullCommands`：直接从 Edge Store 拉取 Command。
- `AcknowledgeCommand`：根据状态调用 Edge Store 对应的状态更新方法。

### `retry.go`

- `RetryPolicy`：重试次数和基础延迟配置。
- `RetryPolicy.Next`：计算指数退避后的下一次时间；超过最大次数时返回不再重试。

### `worker.go`

- `Worker`：保存 Core Store、Edge Store、Transport、日志、重试策略和批大小。
- `NewWorker`：创建同步 Worker，并设置默认批大小。
- `DeliverOutbox`：领取 Core Outbox，发送到 Edge；失败时 retry 或 failed，成功时标记 sent。
- `PullCommands`：从 Edge 拉取 Command，交给 Core Inbox/业务处理器；处理失败时向 Edge 回传 retry/failed，成功时回传 applied。

### `probe_test.go`

Memory 版本的双向架构测试。

- `TestArchitectureProbeBidirectionalAtLeastOnceFlow`：验证 Core Transaction → Outbox → Edge Inbox → Projection，以及 Edge Command → Core Inbox → Outbox 的完整闭环。
- `writeCoreProbe`：写入 Probe、Audit 和 Outbox。
- `probeProjection`：构造一个测试任务投影。
- `newProjectionEvent`：构造 Projection Event。
- `projectProbeEvent`：解码 Probe Event 并更新 Edge 内存投影。
- `assertProjection`：断言 Edge 已看到投影。
- `containsOutboxStatus`：断言指定 Event 的 Outbox 状态。

### `http_transport_test.go`

- `TestHTTPTransportAndEdgeInboxAreIdempotent`：验证 HTTP Transport 和 Edge Inbox 的幂等性。

### `retry_test.go`

- `TestOutboxMovesToFailedAfterRetryBudget`：验证超过重试预算后进入 failed。

### `sql_probe_test.go`

这是可选的真实双数据库/真实 HTTP Probe，默认不运行，通常通过环境变量显式开启。

- `TestArchitectureProbeAgainstRunningServices`：验证真实 Core MySQL、Edge MySQL、Core API、Edge API 和 Worker 之间的同步。
- `findProbeConfig`：寻找 SQL Probe 使用的配置文件。
- `mustWorkingDirectory`：定位仓库工作目录。
- `writeSQLProbe`：在 Core 事务中写入 SQL Probe Event、Audit 和 Outbox。
- `sqlProbeProjection`：构造 SQL Probe 投影。
- `sqlProbeEvent`：构造 SQL Probe Event。
- `sqlProbeEventWithID`：使用指定 Event ID 构造 Event，便于测试重复消息。
- `waitForProjection`：轮询 Edge 数据库直到投影出现或超时。
- `hasSQLProbeTask`：判断查询结果中是否存在目标测试任务。
- `postSQLProbeCommand`：向 Edge API 提交测试 Command。
- `readSQLProbeTasks`：从 Edge API 读取任务投影。
- `assertSQLCount`：断言 SQL 查询返回的记录数量。

## 九、Platform 基础设施 `internal/platform/`

### `config/config.go`

- `Config`：完整进程配置，包含 Core、Edge、Sync、JWT 和日志。
- `ServiceConfig`：一个 API 服务的端口、模式、数据库和 Redis 配置。
- `DatabaseConfig`：MySQL 连接配置。
- `SyncConfig`：Worker 的 Edge 地址、轮询间隔、批大小、重试次数和超时。
- `JWTConfig`：JWT secret、issuer 和 audience。
- `LogConfig`：日志级别、编码和输出位置。
- `RedisConfig`：Redis 是否启用及连接信息。
- `DatabaseConfig.DSN`：生成带 UTC session 时区的 MySQL DSN。
- `DatabaseConfig.Validate`：校验数据库基础字段。
- `Load`：加载 YAML，绑定 `FLIGHT_` 环境变量，反序列化并校验配置。
- `Config.Validate`：校验端口、数据库、运行模式、JWT 和日志配置。

### `mysql/mysql.go`

- `Open`：创建 GORM MySQL 连接并配置连接池。
- `Ping`：检查 MySQL 是否可用。
- `Close`：关闭 GORM 底层 SQL 连接。
- `SQLDB`：取出 GORM 背后的 `database/sql` 连接，供 Migrator 使用。

### `mysql/migrator.go`

- `Migration`：描述一个 migration 的版本、名称和 up/down 文件。
- `Status`：描述 migration 是否已经应用。
- `Migrator`：保存 SQL DB 和 migration 目录。
- `NewMigrator`：创建 Migrator。
- `Discover`：扫描目录，配对 up/down 文件，并按版本排序。
- `Up`：按连续版本顺序应用未执行的 migration。
- `Down`：回滚当前最高版本；调用方必须先经过显式 destructive flag。
- `Status`：读取 `schema_migrations` 并返回每个 migration 的状态。
- `ensureTable`：创建迁移状态表。
- `current`：读取当前最高 migration 版本。
- `apply`：在一个 SQL 事务中执行 migration 文件并写入/删除状态记录。
- `splitStatements`：按分号拆分 SQL 脚本中的语句。

### `redis/redis.go`

- `Open`：Redis 未启用时返回 nil；启用时连接并 Ping。
- `Ping`：检查 Redis 连接；Redis 是 optional 依赖。

### `health/health.go`

- `Checker`：依赖检查函数类型。
- `Endpoint`：保存 required 和 optional 依赖检查器。
- `New`：创建健康检查端点。
- `Live`：只证明进程还活着，不检查数据库。
- `Ready`：检查必需依赖；MySQL 失败返回 503，Redis 失败只显示 unavailable。

### `health/health_test.go`

- `TestDisabledRedisDoesNotBlockReadiness`：验证 Redis disabled 不会阻止 readiness。

### `observability/http.go`

- `Middleware`：生成或复用 request ID/trace ID，写入响应头和结构化访问日志。
- `headerOrGenerated`：在请求头合法时复用 ID，否则生成随机 ID。
- `RequestID`：从 Gin Context 读取 request ID。
- `TraceID`：从 Gin Context 读取 trace ID。
- `HealthResponse`：统一输出健康检查响应。
- `InternalErrorResponse`：统一输出不暴露内部细节的 500 响应。

### `httpclient/client.go`

- `Client`：带统一超时的 HTTP 客户端包装器。
- `New`：创建默认超时的 HTTP 客户端。
- `Do`：在请求中绑定 Context 后发送 HTTP 请求。

### `logger/logger.go`

- `New`：根据配置创建 Zap 结构化日志实例。

### `security/principal.go`

- `PrincipalType`：Human 或 Machine 身份类型。
- `Principal`：当前调用者的身份、角色、Scope 或机器用途。
- `AccessScope`：结构化的数据访问范围。
- `Permission`：资源操作权限类型。
- `IdentityProvider`：账号凭据认证接口。
- `Authenticator`：Token 认证接口。
- `Authorizer`：权限和 Scope 授权接口。
- `ScopeAllows`：判断请求范围是否被调用者范围允许。

### `security/jwt.go`

- `JWTAuthenticator`：基于 JWT 的身份认证实现。
- `accessClaims`：JWT 内部声明，包含 session ID 和标准声明。
- `NewJWTAuthenticator`：校验 JWT 配置并创建认证器。
- `Issue`：给 Principal 签发带 issuer、audience、issued-at 和 expiry 的 HS256 Token。
- `AuthenticateToken`：校验 Bearer Token 的签名算法、issuer、audience、有效期和 subject。

### `security/jwt_test.go`

- `TestJWTContainsIdentityClaimsButNotRolePermissions`：验证 JWT 不携带完整角色权限列表。
- `TestJWTRejectsWrongIssuer`：验证错误 issuer 的 Token 会被拒绝。

### `clock/clock.go`

- `Clock`：时间抽象接口，便于业务测试替换时间。
- `Real.Now`：返回当前 UTC 时间。

## 十、Shared 协议和工具 `internal/shared/`

### `event/envelope.go`

- `EventEnvelope`：Core → Edge 的版本化事件格式。
- `CommandEnvelope`：Edge → Core 的版本化命令格式。
- `NewEvent`：序列化 payload 并生成 Event ID、Correlation ID、Trace ID。
- `NewCommand`：序列化 payload 并生成 Command ID；没有 Trace ID 时自动生成。
- `EventEnvelope.Validate`：校验 Event ID、版本化类型、聚合信息、时间、Producer、Trace ID 和 JSON payload。
- `CommandEnvelope.Validate`：校验 Command ID、版本化类型、Actor、聚合信息、时间、Trace ID 和 JSON payload。

### `id/id.go`

- `NewPublicID`：生成 UUIDv7-compatible public ID。数据库内部仍使用自增 BIGINT，跨服务 ID 使用它。
- `MustPublicID`：测试和夹具专用的快捷生成方法，生成失败直接 panic。

### `errors/errors.go`

定义跨 HTTP/同步边界使用的稳定错误类别：参数无效、未认证、无权限、找不到、冲突、依赖不可用和重复消息。

## 十一、测试文件如何看

v2 测试大致分成三层：

1. `*_test.go` 位于单个包内：验证 Store、JWT、RBAC、健康检查和迁移发现器。
2. `internal/integration/sync/probe_test.go`：使用 Memory Store 验证双向同步，不需要真实数据库。
3. `internal/integration/sync/sql_probe_test.go`：显式开启后使用真实双库和 HTTP 服务，验证容器级架构。

测试中的 `Probe` 不是正式业务。它的价值是证明以下基础能力可恢复：重复 Event、重复 Command、Outbox retry、Inbox failed、Worker 重启和 Edge 暂不可用。

## 十二、不要混淆的 legacy 文件

下面这些文件仍会参与 `go test ./...`，但不是 v2 的新入口：

- `cmd/server`
- `internal/server`
- `internal/model`
- `internal/store`
- `internal/auth`
- `internal/middleware`
- `internal/module/event`
- `migrations/mysql`
- `configs/config.yaml`
- `deployments/docker-compose.yml`

它们属于早期单库原型和 B3 现场。v2 的入口是 `cmd/core-api`、`cmd/edge-api`、`cmd/worker`、`cmd/migrate`；v2 的数据库迁移是 `migrations/core/mysql` 和 `migrations/edge/mysql`。

## 十三、下一阶段业务代码应该放在哪里

业务实现应遵循：

```text
HTTP Handler
  -> Application Service
  -> Domain / Port
  -> Repository Adapter
```

下一条业务闭环是：

```text
Flight -> Task -> Personnel -> Leader Confirm -> Edge -> Employee -> Complete
```

入口文档是 `memory-bank/business-slice-v2-flight-task.md`。BVS2-01 设计冻结、BVS2-02 Core 业务表与迁移、BVS2-03 Flight → Task → Candidate 均已完成；下一步为 BVS2-04 Leader Confirm。不要把旧 `internal/module/event` 直接搬回 v2。
## BVS2-06 Task query boundary

- `internal/core/application/flighttask/query.go` contains the Core read-side port, RBAC/scope boundary, pagination, and API view mapping.
- `internal/core/application/flighttask/query_handler.go` registers `GET /api/v1/tasks` and `GET /api/v1/tasks/{taskPublicID}`.
- `internal/core/adapter/mysql/task_query_repository.go` performs parameterized task, candidate, and assignment reads; handlers and services do not use GORM directly.
