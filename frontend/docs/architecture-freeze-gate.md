# 前端工程架构冻结门槛

> 状态：`F0 IN_PROGRESS`
>
> 更新时间：2026-09-01
>
> 适用范围：首期管理端、员工微信小程序、员工网页版，以及它们与 Core/Edge API 的交接边界。

## 1. 冻结目标

在进入真实业务页面、真实登录联调和生产数据接入前，先冻结以下内容：

1. 三个客户端的职责、API 边界和发布边界。
2. 个人微信、企业微信与 Core `Staff` 的身份映射方式。
3. 员工端命令的幂等键、状态查询、失败与重试展示语义。
4. Task 的创建、分配、接受、完成、取消和重新分配生命周期。
5. OpenAPI、错误模型、权限模型和前端共享契约。
6. 本地开发、Mock、测试账号、域名和验收环境。

技术基础 `Architecture v2.0 Foundation` 已经是 `ACCEPTED / FROZEN`；本文件冻结的是面向前端交付的业务接入合同，当前尚未完成。

## 2. 客户端边界（当前基线）

| 客户端 | 首期定位 | 允许调用 | 明确禁止 |
| --- | --- | --- | --- |
| `admin-web` | 管理端，桌面优先的响应式 Web | Core API | Edge Projection、员工端命令、Core/Edge 数据库 |
| `employee-miniapp` | 员工端首要入口；个人微信和企业微信共用业务页面 | Edge API | Core API、`/internal/sync/v1/*`、伪造员工身份 |
| `employee-web` | 正式支持的员工网页版和备用入口 | Edge API | Core API、`/internal/sync/v1/*`、伪造员工身份 |

所有前端运行时代码、构建配置、静态资源、Mock、组件测试和 E2E 必须位于 `/frontend`。根目录 `web-admin/`、`miniapp/` 不作为 v2 前端入口恢复使用。

共享包只承载 DTO、错误模型、展示状态映射、API Client、认证适配器、Mock 和 UI 基础能力；权限校验、事务、最终业务状态和数据一致性仍由后端负责。

## 3. 冻结门槛清单

状态含义：`DONE` 已有证据；`PARTIAL` 有代码但合同未收敛；`BLOCKED` 未完成且阻止进入真实业务；`PENDING` 可在合同确定后执行。

| 门槛 | 当前状态 | 证据或缺口 | 关闭条件 |
| --- | --- | --- | --- |
| 前后端物理边界 | `DONE` | `/frontend` 已建立三客户端和共享包目录 | 代码审查确认不出现根目录前端运行时入口 |
| Core/Edge 职责边界 | `DONE` | 管理端走 Core；员工小程序和员工 Web 走 Edge | 在 API Client 和 CI 规则中固化 |
| Core Task 读侧 | `PARTIAL` | 当前已有 `GET /api/v1/tasks`、`GET /api/v1/tasks/{taskPublicID}`、JWT/RBAC/Scope 查询 | 用最终 OpenAPI 和前端 DTO 固化分页、筛选、详情字段 |
| Task 写入语义 | `PARTIAL` | 当前 Task 由 Arrival 事务创建；Confirm/Cancel 已有实现 | 明确是否保留 Arrival-only 创建；明确取消后的再分配规则 |
| 双微信身份映射 | `BLOCKED` | 已形成 [身份合同提案](identity-contract-proposal.md)，实际登录适配器和绑定流程尚未冻结 | 确认 AppID/企业微信配置、首次绑定、换绑、离职和多身份场景 |
| 员工 Web 登录 | `BLOCKED` | `employee-web` 必须是正式支持入口；身份合同提案已给出推荐方案，但浏览器登录通道尚未确定 | 确认企业微信网页登录、个人微信网页登录或受控链接到小程序的策略，并形成 API 合同 |
| 员工命令幂等 | `BLOCKED` | 当前快捷 Accept/Complete 会由服务端生成 command ID | 客户端提交稳定幂等键或 command ID；重复提交可安全返回同一结果 |
| 员工命令状态查询 | `BLOCKED` | 当前有 `202 pending`，没有面向员工的稳定状态查询合同 | 冻结 pending/syncing/confirmed/failed 的字段、查询接口、重试和冲突展示 |
| OpenAPI 与生产认证一致 | `PARTIAL` | Core/Edge 已有 Bearer JWT；部分 OpenAPI 仍描述 `X-Actor-*` 或 `X-Employee-Public-ID` 调试头 | OpenAPI 将 Bearer JWT 作为生产合同，调试头标注为非 release 适配器或移出公共文档 |
| 任务展示状态映射 | `PARTIAL` | 已约定业务状态与命令状态分层 | 完成状态字典、按钮可用性、过期/冲突/失败文案和刷新策略 |
| Core/Edge 同步边界 | `DONE` | `/internal/sync/v1/*` 由 Worker 使用，浏览器不得调用 | API Client、环境变量和 E2E 加负向断言 |
| 权限与 Scope | `PARTIAL` | Core 已有 Human Principal、RBAC、Scope 约束 | 将角色、Scope、401/403 和无数据状态转成前端可验证契约 |
| 错误与请求追踪 | `PARTIAL` | 后端已有稳定错误码、request ID、trace ID 基础 | 冻结公共错误 Envelope、展示分级和支持人员排障字段 |
| 前端工具链 | `PENDING` | `/frontend` 目前只有目录骨架，无 package/build/test 配置 | 冻结 Node 包管理器、TypeScript、构建、Lint、单测、E2E 和 CI 命令 |
| Mock 与测试数据 | `BLOCKED` | 尚无首期业务 fixture、测试账号和双身份测试矩阵 | 为三客户端提供脱离真实外部微信的可重复 Mock/Contract/E2E 夹具 |
| 环境与域名 | `BLOCKED` | 小程序 HTTPS 域名、Web 域名、回调和证书尚未形成环境清单 | 形成 dev/test/prod 域名、回调白名单、TLS 和密钥注入清单 |
| 视觉基线 | `PENDING` | 客户端形态已确定，视觉 token 和关键页面还未冻结 | 先冻结导航、任务列表、任务详情、命令反馈四类核心状态 |

## 4. 当前禁止进入的工作

以下工作必须等对应门槛关闭后再开始：

- 不能在前端自行定义 `Staff`、Assignment 或最终 Task 状态。
- 不能把 `202 Accepted` 当成 Accept/Complete 已完成。
- 不能让客户端直接提交或信任 `X-Employee-Public-ID` 作为生产身份。
- 不能让浏览器或小程序调用 `/internal/sync/v1/*`。
- 不能因为前端页面需要而恢复 Core 的手工 `POST /tasks`、任意 PATCH 或硬删除语义。
- 不能先写一套登录页面，再倒推个人微信、企业微信和员工 Web 的身份关系。

## 5. 进入真实业务的验收条件

F0 只有在以下条件全部满足后才能关闭：

1. 三个客户端的 API 方向、运行环境和发布边界已经由项目负责人确认。
2. 个人微信、企业微信、员工 Web 的身份映射、首次绑定、换绑和失效策略已经形成后端接口合同。
3. Accept/Complete 的幂等键、命令状态查询、失败重试和业务状态刷新规则已经形成 OpenAPI 和示例。
4. Task 生命周期矩阵已经明确，尤其是 Arrival 创建、Confirm、Accept、Complete、Cancel 和重新分配之间的允许/拒绝关系。
5. Core/Edge OpenAPI 与当前代码一致，生产认证不再依赖调试 Header 文档。
6. `/frontend` 工具链、共享 contracts/api-client/auth、Mock fixture 和最小 E2E 骨架可运行。
7. 已准备脱离真实微信平台的双身份测试数据，并完成一次 API Contract + 三客户端冒烟验收。
8. Docker 前置检查和项目统一验证入口可重复运行；未实际运行的验证不得标记为通过。

## 6. F0 下一步执行顺序

按以下顺序关闭，不跨步编写真业务页面：

1. 先冻结身份合同：登录通道、外部身份绑定和员工 Web 方案。
2. 再冻结员工命令合同：幂等键、状态查询和失败恢复。
3. 再修正 Core/Edge OpenAPI 与错误模型。
4. 冻结 Task 生命周期和首期页面状态矩阵。
5. 初始化 `/frontend` 工具链与共享契约包，接入 Mock。
6. 最后做三客户端壳层和首个真实业务 Slice。

## 7. 关联文档与代码证据

- 前端设计：[memory-bank/frontend-design-document.md](../../memory-bank/frontend-design-document.md)
- 前端技术栈：[memory-bank/frontend-tech-stack.md](../../memory-bank/frontend-tech-stack.md)
- 前端实施计划：[memory-bank/frontend-implementation-plan.md](../../memory-bank/frontend-implementation-plan.md)
- 前后端交接：[docs/frontend-backend-handoff.md](../../docs/frontend-backend-handoff.md)
- Core OpenAPI：[api/core/openapi.yaml](../../api/core/openapi.yaml)
- Edge OpenAPI：[api/edge/openapi.yaml](../../api/edge/openapi.yaml)
- Core Task Query：`internal/core/application/flighttask/query.go`、`query_handler.go`
- Edge 员工 Command：`internal/edge/application/server.go`
