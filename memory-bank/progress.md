# 项目进度

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

## 基础层验收门（2026-08-07）

- 已通过：工具链、Go 构建测试、Compose 配置、MySQL/Redis 健康、版本化迁移、服务启动/优雅关闭、liveness/readiness、统一响应/request ID、配置校验、JWT、角色/权限拒绝路径、基础部署说明。
- 有意保留：没有默认用户/种子账号；真实登录成功流程需要后续测试夹具或明确的初始化账号策略。
- 有意未执行：没有对现有 Docker 数据卷执行真实 `down`，因为该操作会删除表；迁移文件、状态追踪和回滚入口已存在，真实回滚应在隔离测试库中验证。
- 当前结论：基础架构可以进入第一个业务垂直切片，但在写业务代码前仍需重新读取 PRD/设计文档，并冻结首个切片的业务状态机和验收标准。
- 业务切片 001 已冻结：航班到达后的任务确认闭环；已明确状态机、幂等、硬约束、角色权限、验收标准和数据库缺口。当前仍未开始业务代码，下一步是业务 Step B1 数据结构迁移。
- 业务 Step B1 已完成：新增班组/成员、任务候选、通知、审计模型；扩展事件幂等字段、任务模板触发类型、任务实例版本/班组、分配确认字段；`000002_task_confirmation_foundation` 已在 Docker MySQL 应用，重复迁移通过；全量测试和构建通过。
- 业务 Step B2 已完成：新增 `internal/testsupport` 场景夹具，覆盖合格员工、跨班组员工、能力不足员工和忙碌员工；支持显式 `Persist`/按主键反向 `Cleanup`，不使用 `TRUNCATE`；默认测试为内存单元测试，不接触开发数据库；全量测试和构建通过。
- 下一步：业务 Step B3，实现 `flight_arrived` 事件幂等处理和任务生成，不实现组长确认/员工接收流程。
