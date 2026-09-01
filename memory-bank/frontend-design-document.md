# 前端工程设计基线

> 状态：DRAFT，待产品/后端确认后冻结
> 更新时间：2026-09-01
> 适用范围：航空保障智能协同平台 Web 管理端与员工移动端

## 1. 设计依据与边界

本设计基于以下资料和当前工作区代码：

- `memory-bank/project-memory.md`：长期架构和协作约束；
- `docs/architecture/architecture-v2.md`：Architecture v2.0 `ACCEPTED / FROZEN`；
- `memory-bank/business-slice-v2-flight-task.md`：BVS2 业务规则、状态机、权限、事件和 Command 契约；
- `memory-bank/architecture.md`、`memory-bank/progress.md`、`HANDOFF.md`：当前 BVS2-04、BVS2-05、BVS2-06 的相关代码和隔离验证状态；历史章节可能保留当时的阶段描述，以最新进度和实际代码为准；
- `docs/frontend-backend-handoff.md`：页面、接口、异常和联调建议；其中较早的状态描述以实际代码和本文件的当前状态为准。

前端遵守以下不可变边界：

1. 浏览器只调用 Core API 或 Edge API，不能访问 MySQL、Redis、Outbox、Inbox 或 Worker 内部接口。
2. Core 是业务事实源；Edge 只提供员工端最小 Projection，并持久化员工 Command。
3. 前端缓存、乐观 UI 和本地草稿都不是业务事实，不能自行推进 Task、Assignment 或 Personnel 状态。
4. 员工 Accept/Complete 必须经过 Edge Command → Core 校验 → Core Event → Edge Projection 回传。
5. 不引入 `tenant_id`、`airport_id`、机场切换、多机场导航或跨机场数据模型。

## 2. 目标与首期范围

前端首期目标是让用户能清楚地回答两件事：

- 管理人员：哪些任务等待确认、候选人是谁、确认后是否已经进入员工端；
- 员工：我有哪些任务、当前是否可以 Accept/Complete、操作是否已经被 Core 确认。

首期采用“一个前端工作区、三个客户端形态”的形态：管理端 Web、员工微信小程序和员工 Web。员工微信小程序是微信生态主入口，员工 Web 是正式可用的直接浏览器入口；两个员工客户端不增加新的业务事实。

| 应用 | 面向用户 | 数据边界 | 首期交付 |
|---|---|---|---|
| `admin-web` | Admin / Manager / Leader | Core API | 任务工作台、任务详情、Candidate 选择、Confirm/Cancel 状态反馈；当前列表/详情读接口已存在，未收敛字段继续用 Mock |
| `employee-miniapp` | Staff | Edge API | 首期微信小程序；同时支持个人微信和企业微信入口，提供员工任务列表、详情、Accept、Complete、取消/失败/待同步状态 |
| `employee-web` | Staff（网页版） | Edge API | 正式可用的浏览器入口和备用入口；与小程序共用 contracts、task-domain、Mock 和 API 语义 |
| `dev-preview` | 开发和联调人员 | Mock 或本地 API | 场景切换、重复点击、网络错误、乱序 Projection 和权限失败演示；不作为生产功能 |

当前已创建 `frontend/` 目录骨架，包括管理端、首期员工小程序和员工 Web 三个应用目录、共享包目录、E2E 目录和前端文档目录；应用源码、依赖清单和构建配置仍将在 F1 创建。根目录原有的 `web-admin` 和 `miniapp` 目前为空，未作为新的前端入口使用。

首期范围已确认：管理端和员工端同时建设。管理端固定采用桌面优先、可响应式收缩的 Web；基于企业内部入口过多、员工不愿新增软件或网页的痛点，员工端首期以同时支持个人微信和企业微信入口的微信小程序作为主入口，同时保留正式可用的 `employee-web` 网页版。最终认证和平台接线在 F0 冻结。

### 2.1 响应式 Web/PWA 与微信小程序选型

这里要区分“响应式 Web”和“PWA”：响应式 Web 是页面适配不同屏幕的基础形态；PWA 是在 Web 之上增加可安装入口、Manifest、Service Worker 和有限离线能力的渐进增强方案。PWA 的安装、图标和离线能力受浏览器、系统和部署条件影响，不能把“可安装”当成所有设备上的一致能力。参考 [MDN Web App Manifest](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Manifest) 和 [MDN Installable PWAs](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Tutorials/js13kGames/Installable_PWAs)。

结合本项目的角色、作用和使用方式，两个端的优先级并不相同：

- 管理端服务 Admin、Manager、Leader，主要用于任务工作台、Candidate 对比、人员/区域/时间信息查看和 Leader Confirm。这些场景信息密度高、需要表格或分栏、键盘和大屏操作，响应式 Web 更合适，不建议把管理端做成微信小程序。
- 员工端服务 Staff，主要用于手机查看 Edge Projection、进入任务详情、Accept/Complete，以及观察待同步、冲突和失败提示。它是高频、短时、触控优先的现场操作，且员工已经在使用微信/企业微信，因此首期以微信小程序作为主入口更符合实际使用意愿。
- 无论选 Web/PWA 还是微信小程序，员工写操作都必须经过 Edge Command，并等待 Core 校验、事件同步和 Projection 收敛。平台外壳不能替代可靠 Command、幂等键、状态查询和同步失败处理。

| 维度 | 响应式 Web/PWA | 微信小程序 | 对本项目的影响 |
|---|---|---|---|
| 首期交付与代码复用 | 管理端和员工端可共用 TypeScript、contracts、API client、task-domain、Mock 和测试模型；PWA 只增加 Web 能力 | 需要独立的小程序运行时、页面壳层、构建/预览/发布链路；业务模型可复用，但 Web UI 不能直接复用 | 当前前端工作区刚起步，Web/PWA 能更快形成双端闭环 |
| 员工使用入口 | 浏览器链接、二维码或桌面安装；无需绑定某个超级 App | 个人微信可通过搜索/扫码/分享进入，企业微信可通过工作台、消息卡片或扫码进入 | 当前项目的主要痛点是减少新入口，小程序更符合员工既有使用习惯 |
| 发布与运维 | 静态资源部署后即可更新；PWA 可缓存资源，但需处理缓存版本 | 需遵守小程序审核、版本发布和平台配置流程 | 首期需求仍在验证，Web 发布节奏更适合快速联调 |
| 网络与后端接入 | 生产需 HTTPS、CORS、缓存策略和浏览器权限；PWA 的 Service Worker 可改善资源加载和部分读场景 | `wx.request` 等网络能力要求配置通信域名并使用 HTTPS，不能直接依赖本地 IP/localhost | 当前本地联调先用 Mock；生产前必须准备小程序合法 HTTPS API 域名 |
| 弱网/离线 | 可做离线壳、读缓存和“待提交”提示；不应把浏览器离线队列当作业务事实 | 也可做本地缓存，但仍受小程序生命周期和平台能力约束 | 两者都不能绕过 Edge 的持久化 Command；首期只做明确的 pending/syncing 展示 |
| 身份与权限 | 可接正式 JWT/OIDC、企业 SSO 或浏览器会话 | 需要同时处理个人微信登录态、企业微信登录态、企业身份映射、AppID/环境和会话换取 | 两种外部身份最终都必须映射到同一个 Staff，不得生成两套员工事实 |
| 原生能力 | 依赖浏览器能力，适合本项目当前的列表、详情和操作按钮 | 微信扫码、分享、工作台和消息卡片等入口更顺手，但带来平台耦合 | 入口价值已经成为员工端首期的主要决策因素 |
| 维护与观测 | 一套 Web 调试、错误上报和 E2E 体系，可覆盖桌面和移动浏览器 | 需要增加小程序端调试、兼容性、发布版本和平台告警维度 | 首期应把精力放在 Core/Edge 契约、认证和同步语义上 |

当前推荐落位：

```text
admin-web       → 响应式 Web，桌面优先
employee-miniapp→ 微信小程序，首期主端，同时支持个人微信和企业微信
employee-web    → 正式员工网页版 / 备用入口，与小程序共用业务
```

推荐的原因是：当前最大风险不是前端代码复用，而是员工是否愿意进入系统。微信小程序能够复用员工已有入口，减少安装、记网址和重复登录的负担；管理端仍使用 Web，以满足高信息密度和桌面工作台需求。真正影响上线的关键项是双入口身份映射、客户端 Command 幂等键、Command 状态查询，以及生产 HTTPS/域名配置。小程序本地缓存也不等于可靠业务写入。

该应用只替换客户端适配层，复用 `packages/contracts`、`packages/task-domain`、错误模型和 API 契约；微信 API、页面生命周期和登录态不得渗透到共享业务域。微信小程序网络请求的通信域名、HTTPS 和并发等约束应纳入部署清单，可参考 [腾讯云微信小程序网络配置文档](https://intl.cloud.tencent.com/zh/document/product/1219/61745?lang=zh)。

### 2.2 个人微信与企业微信双入口

两种入口应汇聚到同一个 `Staff`，而不是为同一名员工创建两份账号：

```text
个人微信身份 ─┐
              ├─ 身份适配层 / 绑定关系 ─→ StaffPublicID ─→ Edge Session ─→ 员工任务
企业微信身份 ─┘
```

设计约束：

- 个人微信入口负责普通微信用户的登录态；企业微信入口负责工作台、消息卡片或企业扫码场景的登录态。两种登录态都不能直接作为 Core Personnel 的业务主键。
- 后端需要把外部身份映射到既有 Staff，映射结果由服务端签发会话；小程序不能自行拼接 `X-Employee-Public-ID`，也不能把 openid、unionid 或企业微信用户标识直接当作业务身份。
- 同一 Staff 可能存在个人微信和企业微信两个外部身份，必须有明确的绑定、解绑、换绑和冲突处理规则；未完成身份映射时只能展示登录/绑定引导，不能读取或写入任务。
- 两种入口共用同一套任务列表、详情和 Accept/Complete 交互；入口差异只存在于启动上下文、登录适配和可选的微信生态能力。
- 个人微信与企业微信的登录接线、企业归属校验和测试账号需要后端在正式认证方案中确认；当前开发 Header 仅保留给 Mock/本地联调。

## 3. 信息架构与页面

### 3.1 管理端

```text
admin-web
├── /tasks                  任务工作台
├── /tasks/:taskPublicID   任务详情
└── /settings/diagnostics  开发诊断（仅 dev，可选）
```

任务工作台包含状态筛选、航班显示号、Area/Team、计划时间和候选数；当前 Core 已提供带 `task:read`、JWT 和 Scope 约束的列表/详情读接口，尚未冻结的字段和异常场景仍通过 Mock 先行验证。

任务详情包含：任务摘要、航班摘要、状态时间线、Candidate 快照、当前 Assignment、操作结果和 request/trace 诊断信息。Candidate 只有 `awaiting_confirmation` 时可选择；Confirm 必须提交 `candidate_public_id`、`confirmation_id` 和 `expected_task_version`。

首期不把“Publish”作为前端概念。界面统一使用“确认分配”，对应当前实际 Core 路由 `POST /api/v1/tasks/{taskPublicID}/confirm`；如果产品最终把 Publish 设计为另一种业务动作，再单独增加状态和契约。

### 3.2 员工端

```text
employee-miniapp
├── /tasks                  我的任务
└── /tasks/:taskPublicID   任务详情和执行操作
```

员工小程序列表只展示 Edge Projection 返回的最小字段：航班显示号、任务名称、Area、计划时间、提示信息和业务状态。个人微信与企业微信共用同一套页面和操作；区别只体现在进入方式与登录态。详情页按业务状态显示动作：

| `business_status` | 可见动作 | 说明 |
|---|---|---|
| `assigned` | Accept | 只能提交 Command，不能直接改成本地 `in_progress` |
| `in_progress` | Complete | 只能提交 Command，不能直接改成本地 `completed` |
| `completed` | 无 | 终态，只读 |
| `cancelled` | 无 | 终态，只读并明确不可执行 |

### 3.3 不做的页面

首期不建设完整航班管理、人员档案、规则编排、消息中心、设备监控、AI 预测、跨机场切换、内部同步控制台或直接查询数据库的页面。Arrival 入口仅作为后端/联调能力，不自动包装为面向最终用户的航班管理页面。

## 4. 前端状态模型

前端同时维护“服务端业务状态”和“客户端操作状态”，两者不得混用。

### 4.1 业务实体状态

```text
Task:
  awaiting_confirmation → assigned → in_progress → completed
           └────────────────────────────────────→ cancelled

Assignment:
  confirmed → accepted → completed
       └────────────────────────→ cancelled

Personnel:
  idle → reserved → busy → idle
```

管理端展示 Core Task 状态；员工端展示 Edge Projection 的 `business_status`。Candidate 是推荐快照，不能作为员工可执行任务，也不能显示 Accept/Complete。

### 4.2 员工端同步状态

客户端为正在发起的 Command 维护一个显示层状态，不把它写回业务实体：

```text
idle → pending → syncing → confirmed
                    └────→ failed
```

- `pending`：Edge 已返回 HTTP 202，Command 已进入 Edge Store，但 Core 尚未确认；
- `syncing`：客户端知道操作已提交，等待 Projection 或后端结果；
- `confirmed`：读取到对应的 Core Event Projection，业务状态已变化；
- `failed`：收到明确的终止业务拒绝或后端提供的失败结果；
- 页面刷新后若没有 Command 状态查询接口，只能显示“等待同步/待重新确认”，不能假设已经成功或失败。

当前 Edge 员工路由返回 202 和 `command_id`，但没有公共 Command 状态查询；因此首期不承诺刷新后仍能精确恢复 `syncing/failed`。后端补充状态查询或带结果的查询契约后，再开放完整的同步状态面板。

### 4.3 页面通用状态

所有页面和请求至少覆盖：`loading`、`ready`、`empty`、`error`、`forbidden`、`not_found`、`stale`、`version_conflict`。写操作额外覆盖：`submitting`、`pending`、`retryable_error`、`terminal_error`。

## 5. API 与客户端边界

### 5.1 两个独立 API Client

前端建立 `coreClient` 和 `edgeClient` 两个明确的客户端实例，分别注入不同的 Base URL、认证适配器和错误策略：

| Client | 本地地址 | 当前可用业务接口 | 备注 |
|---|---|---|---|
| Core | `http://127.0.0.1:8081` | `POST /api/v1/flights/{id}/arrival`、`GET /api/v1/tasks`、`GET /api/v1/tasks/{id}`、`POST /api/v1/tasks/{id}/confirm`、`POST /api/v1/tasks/{id}/cancel` | 到达、列表/详情、Confirm、Cancel 已有当前代码；手工 `POST /tasks`、任意 PATCH 和硬删除不属于当前冻结合同 |
| Edge | `http://127.0.0.1:8082` | `GET /api/v1/tasks`、`POST /api/v1/tasks/{id}/accept`、`POST /api/v1/tasks/{id}/complete`、`POST /api/v1/commands` | 正式配置使用 Bearer JWT；`X-Employee-Public-ID` 仅是显式非 release 开发适配器；公共 Command 状态查询和客户端幂等键仍待冻结 |

`/internal/sync/v1/*` 只允许 Worker 调用，前端永远不调用。健康检查可以用于开发诊断，但不能作为业务可用性或登录成功的替代。

### 5.2 请求上下文

每个请求由客户端统一处理：

- 生成或透传 `X-Request-ID`、`X-Trace-ID`，在错误界面和联调日志中保留；
- 解析响应头中的 request/trace ID，优先使用服务端返回值；
- 统一解析当前的 `data`、`items` 和错误响应差异；
- 将 HTTP 状态、稳定 `code`、用户可读 `message`、request ID、trace ID 和是否可重试归一化为 `ApiError`。

### 5.3 幂等与重试

- Confirm、Arrival、未来 Cancel 使用业务幂等 ID；超时后重试必须复用原 ID；
- Accept/Complete 的业务目标幂等由 `command_id` 保护；当前员工快捷路由由 Edge 服务端生成 Command ID，前端首期不对其做自动重试；
- 生产契约必须允许客户端传递稳定的 `command_id` 或 `Idempotency-Key`，否则网络超时后无法保证前端重试仍指向同一 Command；
- GET 可在有限次数和退避后重试；POST 只在复用同一幂等键且后端明确支持时重试；
- 409、`stale_task_version`、`stale_assignment`、`projection_version_conflict` 不盲目重试，先重新读取并提示数据已被其他操作更新；
- 收到 202 时只显示“已提交，等待同步”，不把 Projection 直接改为最终业务状态。

### 5.4 数据持久化

首期不建设浏览器可靠消息队列。Edge `mobile_command` 是可靠 Command 存储，浏览器只保存查询筛选等展示偏好；必要的 pending 显示元数据可临时保存在会话级缓存，但不能作为业务恢复依据。

## 6. 认证、授权与安全

当前 v2 Core/Edge 业务路由已接入正式 Bearer JWT；`X-Actor-*` 与 `X-Employee-Public-ID` 仅在显式、非 release 的开发适配器中使用，不是生产身份来源。前端先实现抽象接口，并为个人微信、企业微信、员工 Web 和管理端分别提供可替换的认证适配器，不把测试 Header 封装成生产登录方案：

```text
SessionProvider
  → AuthAdapter
  → coreClient / edgeClient request context
```

生产接线前必须由后端确认：个人微信/企业微信登录入口、外部身份到 Staff 的绑定关系、JWT/OIDC 或等价会话入口、Token 生命周期、Core/Edge 是否共享会话、Staff 与管理角色映射、Scope 传递方式和登出语义。身份合同提案见 [`frontend/docs/identity-contract-proposal.md`](../frontend/docs/identity-contract-proposal.md)。长生命周期凭据不放入 localStorage；开发环境的 Actor Header 仅在显式 dev flag 下启用，并且构建产物默认关闭。

前端可以根据 Session 中的角色做导航和按钮隐藏，但所有写权限、对象 Scope、员工本人校验和状态机校验必须由后端再次执行。隐藏按钮不等于授权。

## 7. Mock-first 与契约管理

由于当前仍存在身份、Command 幂等/状态查询和 OpenAPI 与实际路由的合同差异，前端先用 Mock 完成页面和状态机，再逐接口切换真实 API；Core 任务列表/详情/取消的基础实现已经存在：

1. Mock 数据必须标注来源和假设，不伪造当前未实现接口已经存在；
2. Mock handler 与真实 client 使用相同 DTO/错误归一化接口；
3. 状态场景覆盖正常、空列表、候选为空、越权、409 版本冲突、202 pending、同步失败、迟到/乱序 Projection 和重复点击；
4. API 字段以实际代码、迁移和冻结业务契约为准；OpenAPI 修正前不自动把不一致的文档生成成生产客户端；
5. 后端契约冻结后，Mock、OpenAPI、API client、页面验收场景和本交接文档同步更新。

## 8. 视觉与交互原则

- 采用“运行态工作台”风格：信息密度适中，状态、负责人、计划时间和可执行动作优先；
- 状态颜色只能表达语义，必须同时有文字或图标，不能仅靠颜色区分；
- 关键动作采用二次确认和明确结果：确认分配、Accept、Complete、Cancel；
- 写操作期间按钮锁定，防止重复点击；返回 202 时用非阻塞提示和任务卡片状态提示；
- 移动端以单列卡片、底部主操作区和较大的触控目标为主；管理端以表格/详情分栏为主；
- 时间以服务端 ISO 8601 解析，按单机场时区展示；不要在客户端重新定义业务时间；
- 错误提示展示稳定业务含义和 request/trace ID，禁止展示 token、密码或内部堆栈。

## 9. 验收标准

前端设计进入实现门槛前必须满足：

- Core 与 Edge 请求代码物理分离，前端没有数据库或内部同步接口依赖；
- 员工 Accept/Complete 的 202、重复点击、网络超时、任务取消和 Projection 延迟均有可验证 UI 状态；
- 管理端 Confirm 使用 `expected_task_version` 和原始 `confirmation_id`，遇到冲突刷新详情，不覆盖服务端事实；
- 所有业务写按钮受角色/状态控制，但最终权限仍交由后端校验；
- Mock 与真实 API 可替换，且 Mock 不把建议接口伪装成当前生产接口；
- loading/empty/error/forbidden/not_found/stale/version conflict 有独立展示；
- request ID、trace ID、Command ID 或幂等 ID 可用于联调定位；
- 前端单元、契约、组件和关键端到端场景可独立运行。

## 10. 冻结前待确认决策

以下事项会影响实现入口，未确认前保持设计为 DRAFT：

1. 微信小程序的个人微信、企业微信入口和身份适配是否按同一 AppID/同一业务端建设；需要根据平台主体和企业微信接入规则确认 AppID、通信域名、发布主体、工作台入口和两类外部身份到 Staff 的映射；
2. Core 任务列表、详情、Arrival 创建和取消 API 的最终字段契约，以及 Confirm 是否继续使用当前命名；基础列表/详情/取消路由已经存在；
3. 个人微信、企业微信、员工 Web 和管理端的正式登录接线、外部身份到 Staff 的绑定、测试账号/种子数据；
4. Edge Command 的客户端幂等键、Command 状态查询和刷新恢复语义；
5. 生产环境 HTTPS、微信小程序 API 域名、个人微信/企业微信登录接线、CORS 和备用 Web 域名规划；
6. 视觉品牌、主题色、字体和是否存在现有设计系统。

F0 冻结门槛、当前状态和进入真实业务的验收条件统一维护在 [`frontend/docs/architecture-freeze-gate.md`](../frontend/docs/architecture-freeze-gate.md)。本设计文档在该门槛关闭前保持 `DRAFT`。
