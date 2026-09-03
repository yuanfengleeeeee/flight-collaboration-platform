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
7. 性能预算、动效限制、降级策略和同步状态展示不能互相冲突。

技术基础 `Architecture v2.0 Foundation` 已经是 `ACCEPTED / FROZEN`；本文件冻结的是面向前端交付的业务接入合同。角色、页面边界、管理端完整工作区和员工端扩展入口已按用户确认形成 F0 信息架构基线；视觉评审、工程接入、平台配置和在线验证门仍在进行。

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
| 工号密码首次认证 | `PARTIAL` | BVS2-07 已提供 Core 凭证、bcrypt 校验、失败次数限制和 Edge 工号密码登录；Docker/在线联调未完成 | 完成迁移/接口在线验证，补齐改密/重置和生产密码管理入口 |
| 双微信身份映射 | `PARTIAL` | BVS2-07 已支持个人微信/企业微信 Provider exchange、一次性绑定票据和同一 Staff 映射；当前仅有显式 Mock Provider | 接入并验证真实 AppID/企业微信配置、首次绑定、换绑、离职和多身份冲突场景 |
| 员工 Web 登录 | `PARTIAL` | `employee-web` 已接入 Edge 工号密码认证，浏览器会话由共享 `auth` 包管理；真实 Provider 快捷入口和 E2E 仍待完成 | 完成真实平台入口、跨浏览器 E2E 和生产会话策略 |
| 长期/简易登录 | `PARTIAL` | BVS2-07 已实现短期 Access JWT、Refresh 轮换、重放撤销、当前会话和 Logout；员工 Web 已接入恢复流程，小程序平台存储适配已落地 | 完成在线验证，冻结 Web/小程序安全存储、改密/停用/管理员撤销失效规则 |
| 员工命令幂等 | `PARTIAL` | 后端已支持稳定 command ID；员工 Web 已生成并提交客户端幂等键，重复点击由请求状态和按钮禁用控制 | 在 Node/浏览器和真实 Edge 环境验证重复提交、超时恢复和内容冲突 |
| 员工命令状态查询 | `PARTIAL` | 后端已有 `GET /api/v1/commands/{commandID}`，员工 Web 已轮询并区分 pending/syncing/confirmed/failed | 在 Node/浏览器和真实 Edge 环境验证刷新恢复、失败重试和 Projection 延迟 |
| OpenAPI 与生产认证一致 | `PARTIAL` | Core/Edge 已有 Bearer JWT；部分 OpenAPI 仍描述 `X-Actor-*` 或 `X-Employee-Public-ID` 调试头 | OpenAPI 将 Bearer JWT 作为生产合同，调试头标注为非 release 适配器或移出公共文档 |
| 任务展示状态映射 | `PARTIAL` | 已约定业务状态与命令状态分层 | 完成状态字典、按钮可用性、过期/冲突/失败文案和刷新策略 |
| Core/Edge 同步边界 | `DONE` | `/internal/sync/v1/*` 由 Worker 使用，浏览器不得调用 | API Client、环境变量和 E2E 加负向断言 |
| 双入口数据一致性 | `PARTIAL` | BVS2-07 Mock 测试已验证两类 Provider 映射到同一 Staff；Core → Edge Projection 有同步基础，但双入口在线读取更新验证尚未完成 | 用同一 `StaffPublicID` 验证个人微信/企业微信读取、更新可见性和同步延迟提示 |
| 权限与 Scope | `PARTIAL` | 用户已确认 manager 全量、leader 按团队/区域、staff 本人任务、admin 系统管理；Core 已有 Human Principal、RBAC、Scope 约束 | 将角色、Scope、401/403 和无数据状态转成最终 API 与前端可验证契约 |
| 错误与请求追踪 | `PARTIAL` | 后端已有稳定错误码、request ID、trace ID 基础 | 冻结公共错误 Envelope、展示分级和支持人员排障字段 |
| 前端工具链 | `PARTIAL` | Node.js v24.20.0、npm 11.19.0、pnpm 9.15.0 已配置；依赖已安装，lint/typecheck/test/build 已通过 | 补充 CI、完整 Mock/Contract/E2E 和浏览器冒烟 |
| Mock 与测试数据 | `PARTIAL` | `frontend/preview/` 已提供零依赖页面和关键状态 Mock；尚无共享 fixture、测试账号和双身份测试矩阵 | 为三客户端提供脱离真实外部微信的可重复 Mock/Contract/E2E 夹具 |
| 环境与域名 | `BLOCKED` | 小程序 HTTPS 域名、Web 域名、回调和证书尚未形成环境清单 | 形成 dev/test/prod 域名、回调白名单、TLS 和密钥注入清单 |
| 页面信息架构与角色工作区 | `PARTIAL` | 用户已确认独立页面、完整管理端模块、员工端任务/通知/异常/历史/账号入口和角色任务 Scope；`frontend/preview/` 已按独立入口实现骨架 | 完成最终页面地图、角色可见模块、团队/区域 Scope 交叉规则、越权/无数据表现和真实路由合同 |
| 视觉基线 | `PARTIAL` | 已有“蓝调停机坪”第一版方向和预览，但用户要求继续美化，当前方案未作为最终视觉冻结 | 重新评审视觉方向、密度、导航层级和关键页面后再冻结 Token 与组件基线 |
| 性能与动效预算 | `PENDING` | 已确定同步准确性优先和高成本特效默认禁止；指标、设备基线和自动化检查尚未建立 | 冻结 Web INP/LCP/CLS、资源体积、长任务、内存、帧率和小程序性能检查；关键操作不能被动画延迟 |

## 4. 当前禁止进入的工作

以下工作必须等对应门槛关闭后再开始：

- 不能在前端自行定义 `Staff`、Assignment 或最终 Task 状态。
- 不能把 `202 Accepted` 当成 Accept/Complete 已完成。
- 不能让客户端直接提交或信任 `X-Employee-Public-ID` 作为生产身份。
- 不能让浏览器或小程序调用 `/internal/sync/v1/*`。
- 不能因为前端页面需要而恢复 Core 的手工 `POST /tasks`、任意 PATCH 或硬删除语义。
- 不能先写一套登录页面，再倒推个人微信、企业微信和员工 Web 的身份关系。
- 不能先加入高成本动画，再用动画掩盖接口延迟、同步延迟或状态不确定性。

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
9. 已按性能基线完成关键页面、长列表、重复操作、202 pending 和 Projection 刷新的性能与正确性验收。

## 6. 当前收尾执行顺序

首个真实 Task Slice 的源码已经接入；剩余门槛按以下顺序关闭：

1. 启动隔离的 Core/Edge/Worker 服务，完成 API Contract 和两个 Web 客户端最小浏览器冒烟验证。
2. 完成真实个人微信/企业微信 Provider、小程序平台配置和管理端 SSO。
3. 用隔离数据验证双入口同一 Staff、Scope、Projection 延迟、Command 幂等和失败恢复。
4. 评审视觉、无障碍和性能基线，再扩展没有公开 API 的通知、异常、历史、人员、规则和报表模块。

## 7. 关联文档与代码证据

- 前端设计：[memory-bank/frontend-design-document.md](../../memory-bank/frontend-design-document.md)
- 页面设计：[frontend/docs/page-design.md](page-design.md)
- 前端技术栈：[memory-bank/frontend-tech-stack.md](../../memory-bank/frontend-tech-stack.md)
- 前端实施计划：[memory-bank/frontend-implementation-plan.md](../../memory-bank/frontend-implementation-plan.md)
- 前后端交接：[docs/frontend-backend-handoff.md](../../docs/frontend-backend-handoff.md)
- Core OpenAPI：[api/core/openapi.yaml](../../api/core/openapi.yaml)
- Edge OpenAPI：[api/edge/openapi.yaml](../../api/edge/openapi.yaml)
- Core Task Query：`internal/core/application/flighttask/query.go`、`query_handler.go`
- Edge 员工 Command：`internal/edge/application/server.go`

## 2026-09-01 Command 契约收敛补充

员工 Command 的两个 F0 阻塞项已经完成：

- `POST /api/v1/tasks/{taskPublicID}/accept` 与 `/complete` 要求客户端提交稳定的 `command_id`；同一 ID 的重试忽略每次传输生成的 trace/time 元数据，只要业务内容一致即返回 `duplicate=true`，内容不一致返回 `409 command_id_conflict`。
- 新增 `GET /api/v1/commands/{commandID}`，仅允许命令所属员工查询，返回 `pending`、`syncing`、`confirmed` 或 `failed`，并提供 attempts、下一次重试时间和安全的 `error_code`。
- Edge `CommandRecord` 增加 `updated_at`；Memory Store、MySQL Store 和 Core Inbox 使用统一的逻辑命令等价判断。公共状态不会泄露持久化错误文本。
- 已补充共享事件、Edge Store、Edge HTTP 的幂等/状态测试；客户端刷新恢复应直接查询该公开状态接口，不得读取 `/internal/sync/v1/*`。
