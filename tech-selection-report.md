# 航空保障智能协同平台技术选型报告

> 版本：Architecture v2.0
> 更新时间：2026-09-01
> 文档状态：当前技术基线，以实际代码、Migration 和配置为准
> 依据：AGENTS.md、memory-bank/architecture.md、memory-bank/design-document.md、memory-bank/implementation-plan.md、memory-bank/progress.md、go.mod 及 v2 代码

## 1. 结论

当前项目采用 Go 技术栈。Architecture v2 的技术方向已经冻结，当前实现应继续沿用，不建议在 Phase 2 中途更换后端语言或拆分微服务。

当前基线如下：

- 后端：Go 1.25.1、Gin、标准 net/http、GORM、MySQL Driver、Viper、Zap、JWT。
- 架构：单仓库、Core 模块化单体、独立 Edge API、Core 侧 Worker。
- 数据库：Core MySQL 与 Edge MySQL 完全独立，使用版本化 SQL Migration。
- 同步：HTTP Transport + Transactional Outbox + Inbox + Command Store + Retry，语义为 at-least-once 和幂等消费。
- Redis：可选基础设施，只承担缓存、限流、短锁、在线状态和临时去重，不承担可靠事实或可靠消息投递。
- 前端：已创建 `frontend/` 工作区目录骨架，包含 `admin-web`、`employee-web` 和共享包目录；页面、依赖、构建配置和移动端实现尚未落地。React、TypeScript、Vite、pnpm workspace 仍标记为提议方案。
- 部署：Docker Compose 作为本地开发和基础验证环境；生产部署、HA、备份、监控和密钥管理尚未全部落地。

## 2. 当前项目范围

### 2.1 业务与边界

- 系统只服务单机场，不引入 tenant_id、airport_id、多租户或多机场切换。
- Core 是航班、人员、岗位、能力、人员状态、任务模板/实例/候选/分配、事件、审计和状态机的唯一事实源。
- Edge 只保存移动端所需的 Projection、Command、Session、Inbox 和 delivery 等最小数据。
- Core 与 Edge 不共库、不跨库 JOIN；Edge 不直连 Core DB，Core 不直写 Edge DB。
- 旧 cmd/server、旧 internal/model、旧 internal/store、旧 migrations/mysql 和旧 B3/B4/B5 现场保持 legacy/paused，不是 v2 生产入口。
- 当前 Phase 2 已完成 BVS2-01 至 BVS2-05；BVS2-05 的 Edge Projection/Employee Command 已有代码和隔离 SQL 验证，下一项是 BVS2-06 完整 Compose 闭环验收。

### 2.2 当前代码实际存在的入口

| 入口 | 实际职责 | 当前状态 |
| --- | --- | --- |
| cmd/core-api | Core DB、可选 Redis、Core HTTP API 和 Core 业务用例装配 | 已实现 |
| cmd/edge-api | Edge DB、可选 Redis、Projection/Command/Inbox HTTP API 装配 | 已实现 |
| cmd/worker | Core Outbox 投递、Edge Command 拉取和重试循环 | 已实现 |
| cmd/migrate | 按 target core 或 edge 执行独立 SQL Migration | 已实现 |
| 前端工程 | `frontend/` 工作区目录骨架 | 目录已创建；前端源码、依赖和构建配置尚未实现 |

## 3. 技术栈总表

| 层次 | 当前选型 | 实际依据和边界 |
| --- | --- | --- |
| 语言与运行时 | Go 1.25.1 | go.mod 声明版本；四个 v2 入口共用单仓库模块 |
| HTTP | Gin 1.12.0 + 标准 net/http | Core、Edge 使用独立路由；Handler 负责 HTTP 翻译 |
| 持久化 | MySQL 8.0 + GORM 1.31.2 + database/sql | Core/Edge 独立连接和 schema；Repository/Adapter 才接触 GORM |
| MySQL 驱动 | gorm.io/driver/mysql 1.6.0 | Core 和 Edge 的 SQL 连接 |
| Migration | 版本化 SQL + cmd/migrate | migrations/core/mysql 与 migrations/edge/mysql 分开；禁止启动 AutoMigrate |
| 缓存与临时能力 | go-redis/v9 9.21.0，可选 | cache、rate limit、短锁、在线状态和临时去重；不保存核心事实 |
| 可靠同步 | HTTP + Outbox + Inbox + Command Store | at-least-once、幂等、Retry、Failed；当前不引入额外消息中间件 |
| 配置 | Viper 1.21.0 + YAML + FLIGHT_ 环境变量覆盖 | 真实凭据通过环境或后续 Secret Manager 注入 |
| 日志与观测 | Zap 1.28.0、健康检查、request_id、trace_id | 当前已实现基础日志和健康接口；完整指标平台未接入 |
| 认证 | golang-jwt/jwt/v5 5.3.1 | JWT、Human/Machine Principal、RBAC、AccessScope 已有基础；正式入口接线仍需收口 |
| 测试 | Go unit、HTTP、Memory/显式 SQL integration test | 默认测试不清理用户数据库；数据库测试需要显式开启 |
| 本地部署 | Docker Compose | core-api、edge-api、worker、core-mysql、edge-mysql；Redis 通过 profile 可选 |
| Web 前端 | React + TypeScript + Vite（提议） | `frontend/apps/admin-web` 和 `frontend/apps/employee-web` 目录骨架已创建；尚无 package.json、页面源码和可执行构建 |
| 移动端 | 首期按响应式 Web/PWA 设计（提议） | 员工端应用目录已创建；是否改为原生小程序仍待产品确认 |
| 外部推送/AI/硬件 | Port 或桩预留 | 当前不接入真实账号、推送、模型、RFID/UWB |

## 4. 后端与服务架构

### 4.1 Core

Core 是模块化单体，不拆成多个业务微服务。它负责：

- 航班、人员、岗位、能力、人员状态；
- 任务模板、任务实例、候选人、分配和状态历史；
- 业务权限、状态机、Audit 和业务幂等；
- 关键事实与 Audit、Outbox 在同一个 Core 事务中提交。

代码边界：

~~~text
internal/core/application      Application Service 和 HTTP Handler
internal/core/module            Flight、Personnel、Task、IAM 领域模型
internal/core/adapter/mysql     Core-only GORM Repository
internal/core/sync              Core Inbox、Outbox 和事务接口
~~~

Handler 不承载复杂业务规则，Service 不依赖 Gin，Domain 不依赖 Gin、GORM 或 Redis。

### 4.2 Edge

Edge 是移动端接入和查询服务，不是 Core 业务事实库。它负责：

- 保存 Core 投递的最小 Task Projection；
- 接收员工端或移动端 Command；
- 维护 Sync Inbox、Command 状态、Session 和 delivery 记录；
- 为移动端提供查询和异步提交入口。

Edge 不能直接修改 Core 事实。员工 Accept/Complete 等业务 Command 需要经 Worker 投递到 Core，由 Core 状态机决定是否生效。

### 4.3 Worker

Worker 位于 Core 一侧：

1. 从 Core Outbox 领取事件；
2. 通过 HTTP Transport 投递到 Edge；
3. 对临时失败执行退避重试并记录 Failed；
4. 从 Edge 拉取 Pending Command；
5. 交给 Core Command Handler，并把处理结果 ACK 回 Edge。

Worker 不连接 Edge MySQL。

## 5. 数据库选型与数据边界

### 5.1 两套独立 MySQL

| 数据库 | 所有者 | 保存内容 | Migration |
| --- | --- | --- | --- |
| Core MySQL | Core | 航班、人员、岗位、能力、任务、候选、分配、状态历史、Audit、Outbox、业务幂等 | migrations/core/mysql |
| Edge MySQL | Edge | Task Projection、Notification Projection、Mobile Command、Session、Sync Inbox、delivery | migrations/edge/mysql |

当前仓库使用 MySQL 8.0。不同环境应分别拥有自己的 Core/Edge 数据库实例，但都使用对应目录中的同一套 Migration。Core 和 Edge 不共享数据库账号、schema 或业务表。

### 5.2 Migration 规则

- Migration 由 cmd/migrate 显式执行，支持 target core 和 target edge。
- 服务启动不调用 GORM AutoMigrate。
- Core 当前包含 Foundation 和 BVS2 业务表 Migration；Edge 当前包含 Edge Foundation Migration。
- Migration down 属于破坏性操作，必须显式确认，不作为日常启动步骤。
- 迁移状态必须以目标环境实际 status 结果为准，不能只根据 SQL 文件存在就宣称已应用。

### 5.3 Redis 边界

Redis 是可替换的可选组件，允许用于：

- 缓存和热点数据；
- 限流；
- 短锁；
- 在线状态；
- 短期去重或临时状态。

Redis 不允许承担：

- Core 业务事实；
- 状态历史、Audit 或可靠事件；
- 唯一的 Outbox/Inbox；
- 唯一的员工 Command；
- 需要故障恢复的长期队列。

## 6. 同步和消息技术选型

### 6.1 当前选择

当前使用版本化 Event/Command Envelope，Core 与 Edge 通过 HTTP Transport 传递。可靠性依赖：

~~~text
Core 事务
  → Outbox
  → Worker HTTP 投递
  → Edge Inbox 去重
  → Edge Projection
~~~

反向链路为：

~~~text
移动端 Command
  → Edge Command Store
  → Worker 拉取
  → Core Inbox
  → Core 事务处理
  → Core Outbox
  → Edge Projection 或结果事件
~~~

网络语义是 at-least-once delivery + idempotent consumption，不承诺 exactly-once。

### 6.2 明确不选的方案

当前不引入额外消息中间件、分布式事务、跨库 JOIN 或 Event Sourcing。原因是当前系统是单机场、模块化单体 + 独立 Edge，现有 Outbox/Inbox 已能满足当前可靠同步边界；增加消息集群会提高运维和一致性复杂度。

### 6.3 当前未完成部分

- BVS2-05 尚未完成 task.assigned.v1 到 Edge Projection 的完整业务映射。
- 员工 Accept/Complete Command 的完整 Core 处理尚未完成。
- 生产级服务认证、HTTPS/mTLS 和更完整的 Worker lease/恢复策略仍需补齐。
- 同 ID 不同 payload 的冲突规则、Projection 同版本冲突规则需要统一。

## 7. 认证、授权与安全

当前选择：

- golang-jwt/jwt/v5 负责 JWT 解析和签发基础；
- Human Principal 与 Machine Principal 分离；
- 角色为 admin、manager、leader、staff；
- 权限采用 resource:action；
- 数据范围采用 global、area、team、assigned、self 等 AccessScope；
- JWT 不保存完整权限列表。

实际状态需要区分：

- Principal、JWT、RBAC、Scope 和 Authorizer 基础代码已经存在；
- BVS2-04 Service 已在确认任务时校验 Human Principal、task:assign 和 Team/Area Scope；
- Core HTTP 入口仍保留 X-Actor-* 测试适配，正式 JWT Middleware 和生产服务认证尚未完全收口；
- 本地配置中的 JWT secret 是占位值，不能用于生产；
- 生产密钥、HTTPS/mTLS、Secret Manager、SSO/OIDC 需要后续单独确认。

因此，安全技术方向已选，但当前系统不能直接视为生产认证已经完成。

## 8. 前端与低代码边界

当前仓库已经创建前端目录骨架，但仍没有 package.json、前端源码、移动端源码或前端构建配置。因此以下内容不能写成已落地：

- Web 管理端具体框架；
- 移动端具体框架；
- 页面路由和状态管理方案；
- 前端登录、Token 存储和刷新方案；
- 设计系统和组件库；
- 生产域名、网关和 CDN。

当前采用的前端目录边界如下，目录已创建，内容仍待 F1 实施：

```text
frontend/
├── apps/
│   ├── admin-web/        # 管理端，只调用 Core API
│   │   └── src/
│   └── employee-web/     # 员工端，只调用 Edge API
│       └── src/
├── packages/
│   ├── contracts/src/    # DTO、枚举、schema、错误码
│   ├── api-client/src/   # Core/Edge HTTP client
│   ├── auth/src/         # 认证适配接口
│   ├── ui/src/           # 共享 UI 基础组件
│   ├── task-domain/src/  # 前端展示状态映射
│   └── mock/src/         # fixture 和 Mock handler
├── e2e/                  # Playwright 场景
└── docs/                 # 前端本地运行和联调说明
```

目录结构已经确定，但前端框架、正式认证、API 契约和可执行脚本仍需按前端设计文档冻结后实现。

当前对前端和低代码平台的硬约束只有：

1. 通过 Core API 或 Edge API 调用业务，不直接访问数据库。
2. Core 写操作必须经过 API 的权限、状态机、事务和幂等校验。
3. Edge 查询只能视为 Projection 查询，不把 Projection 当作 Core 事实源。
4. 收到 202 pending、sync_pending、sync_failed 或版本冲突时，必须按异步状态展示。
5. 前端字段、状态、错误码和版本规则必须以正式 API 契约为准。

前端目录边界已确定；React、TypeScript、Vite、pnpm workspace 仍属于 `PROPOSED` 技术选型，在前端工程初始化并完成契约测试前，不宣称已安装或验证。

## 9. 本地部署、配置和测试

### 9.1 本地服务

当前本地基线由 deployments/local/docker-compose.yml 定义：

~~~text
core-api    8081
edge-api    8082
core-mysql  3310 → 3306
edge-mysql  3311 → 3306
worker      Core DB + Edge HTTP Transport
Redis       optional profile
~~~

配置入口：

- configs/config.v2.yaml：本地地址和默认参数；
- configs/config.example.yaml：Compose 容器环境示例；
- FLIGHT_ 前缀环境变量：覆盖配置；
- 真实密码和密钥：不得提交到仓库。

### 9.2 当前验证方式

项目规定的验证入口是：

~~~powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
~~~

Go 基线命令是：

~~~powershell
go test ./...
go build ./...
go build ./cmd/core-api
go build ./cmd/edge-api
go build ./cmd/worker
~~~

Docker、数据库和 Compose 验证前必须先执行 Docker 前置脚本。本文不把未实际运行的命令写成已通过。

### 9.3 生产环境尚未定稿

当前只冻结本地 Compose 和基础容器边界，以下生产选型尚未完成：

- 多实例部署与负载均衡；
- Core/Edge MySQL 高可用、备份、恢复和升级策略；
- 正式 TLS/mTLS、密钥托管和证书轮换；
- 指标、告警、日志集中化和链路追踪平台；
- CI/CD 发布权限和回滚流程；
- RPO/RTO、容量压测和故障演练。

## 10. 选型决策与后续工作

### 10.1 现在可以直接执行的决策

- 继续使用 Go + Gin + GORM + MySQL；
- Core/Edge 保持独立数据库和独立 Migration；
- 继续使用 Outbox/Inbox/HTTP/Retry，不增加额外消息中间件；
- Redis 保持 optional，不把一致性建立在 Redis 上；
- Core 继续使用模块化单体，不提前拆微服务；
- 前端/低代码只依赖版本化 API，不直连数据库。

### 10.2 下一步需要确认的决策

1. Web 管理端框架、移动端最终形态以及 React/Vite/pnpm 方案的正式冻结。
2. 正式 JWT Middleware、登录身份源、Token 刷新和服务间认证。
3. BVS2-05 的 Edge Projection 字段、版本规则和员工 Command 契约。
4. 生产数据库 HA、备份恢复、监控告警和密钥托管。
5. OpenAPI 与实际路由、错误码、响应 envelope 的最终统一。

### 10.3 文档维护规则

本报告只记录当前 v2 技术基线。历史方案、旧单库结构、旧业务入口和未实现的外部能力必须明确标注为历史或待确认，不能继续作为新代码的依据。

规范优先级：

1. 实际代码和注册路由；
2. 已应用的目标环境 Migration 状态；
3. 当前测试和验证结果；
4. 已接受的 Architecture v2 设计；
5. 本报告和其他说明文档。
