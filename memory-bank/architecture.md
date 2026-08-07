# 架构记录

## 当前实际状态

- 仓库是 Go 原型，已有 Gin 路由、配置加载、Zap 日志、MySQL/Redis 连接、健康检查和 AI 预测桩。
- 当前路由主要包含 `/health`、设备状态预留接口和 `/api/prediction/analyze`。
- 当前 `cmd/server/main.go` 负责配置、日志和依赖连接，已不再在服务启动时调用 `AutoMigrate`；数据库迁移将在独立迁移命令中执行。
- 业务模型文件已存在，但航班、任务、人员、事件等完整业务服务和路由尚未实现。
- Go 1.26.5 已安装；`go test ./...` 和 `go build ./cmd/server` 已通过。
- Docker Compose CLI v5.3.1 可用，Docker Server 29.6.2 已通过 `docker info` 验证；MySQL 8.0.46 与 Redis 7 容器已启动并完成连接验证。
- Compose 的 MySQL 宿主机端口支持 `MYSQL_HOST_PORT` 覆盖；本机因已有 `mysqld` 占用 3306，使用 3307 映射验证，未停止已有进程。

## 目标基础架构

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

## 基础阶段的架构决策

- 先使用模块化单体，不引入微服务通信和分布式事务。
- MySQL 保存业务事实和状态变更所需数据；Redis 只承担可替换的缓存、去重或轻量队列职责。
- 数据库采用 `migrations/mysql/` 递增版本 SQL，由独立迁移命令执行；服务启动不再调用隐式 `AutoMigrate`。
- 所有写操作最终必须经过后端服务层的权限和状态校验，不能让 AI、前端或脚本直接修改核心数据。
- 业务逻辑实现前，先完成基础层的启动、连接、迁移、认证、测试和 Docker 验证。
- 配置采用 YAML 基线和 `FLIGHT_` 环境变量覆盖；错误响应携带稳定错误码与 request ID；日志不记录密码和 token。
- 认证骨架使用 Bearer JWT 和四类角色（admin/manager/leader/staff），数据范围权限留给业务模块实现。
- 测试分为单元、HTTP 集成和显式开启的数据库/Redis 集成测试，默认不得清空用户已有数据库。

## 已实现的基础文件

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

## 基础阶段验证结果

- Go 测试和构建基线已通过。
- Docker Compose 配置、MySQL 数据库创建、Redis 连通性已通过。
- 迁移策略已冻结为独立的版本化 SQL 迁移；`initial_schema` 已在 Docker MySQL 中应用并可查询状态。

## 基础层验收结果

- 程序可在 MySQL/Redis 可用时启动并通过 readiness；依赖不可用时仍可提供 liveness，readiness 明确返回 503。
- `go test ./...`、`go build ./...` 已通过；JWT、角色权限、request ID、配置覆盖和迁移发现器均有测试。
- Docker Compose 的 MySQL/Redis 均已通过 healthcheck；迁移版本 `000001 initial_schema` 在 Docker MySQL 中为 `applied`，重复 `up` 不改变状态。
- 服务启动不创建默认用户；登录成功的数据库夹具留待业务测试阶段定义。

## 业务切片 001 数据结构

- `team`、`team_member`：表达班组和员工班组范围。
- `task_candidate`：表达候选人推荐，不等同于确认分配。
- `notification`：记录站内通知和去重键，暂不连接外部推送渠道。
- `audit_log`：记录关键状态、权限操作和操作人。
- `event.idempotency_key`：确保航班到达事件重复提交不重复生成任务。
- `task_template`、`task_instance`、`task_assignment`：增加触发类型、模板版本、班组和确认/接收/完成所需字段。
- 以上结构通过 `migrations/mysql/000002_task_confirmation_foundation` 应用并完成重复迁移验证；业务服务和 API 尚未实现。
- B2 夹具已通过单元测试；默认 `go test ./...` 不连接或清空开发数据库。

## 待补充

- 测试数据库隔离和部署清理脚本
- 业务模块依赖方向
