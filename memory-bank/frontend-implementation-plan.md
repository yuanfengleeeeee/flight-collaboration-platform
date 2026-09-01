# 前端工程实施计划

> 状态：PROPOSED；F0 架构冻结执行中，必须按顺序一次完成一个步骤并验证
> 更新时间：2026-09-01
> 依据：`memory-bank/frontend-design-document.md`、`memory-bank/frontend-tech-stack.md`

## 实施原则

- 先冻结目标端形态、认证和 API 契约，再写业务页面；
- Mock 先行，真实 API 逐条替换；未实现接口不得在生产构建中开放；
- 每个步骤都有独立验证；当前步骤未验证前不进入下一步；
- 前端从服务端状态重建 UI，不用本地点击结果代替 Core Event/Edge Projection；
- 任何真实后端、数据库、Compose 或双库验证前，先执行根目录 `scripts/ensure-docker.ps1`。

## F0 设计冻结与交接确认

首期同时建设管理端和员工端已确认；员工端首期采用同时支持个人微信与企业微信入口的微信小程序，并保留正式员工 Web。F0 当前按 [`frontend/docs/architecture-freeze-gate.md`](../frontend/docs/architecture-freeze-gate.md) 执行，重点冻结双入口身份映射、员工 Web 登录、Edge Command 幂等键/状态查询、OpenAPI 一致性、Task 生命周期、通信域名、Mock/测试数据和视觉基线。

当前身份合同提案见 [`frontend/docs/identity-contract-proposal.md`](../frontend/docs/identity-contract-proposal.md)。验证：产品/后端对身份入口、绑定、会话、撤销和员工 Web 登录逐项给出结论；前端设计文档状态从 `DRAFT` 变为 `FROZEN`，或明确哪些内容仍由 Mock 隔离。

## F1 建立前端工作区与应用壳层

`frontend/` workspace 的目录骨架已创建，包含 `admin-web`、首期 `employee-miniapp`、`employee-web`、共享包、E2E 和 docs 目录。下一步补充管理端 Web、员工 Web 与小程序的运行时配置、入口适配、页面路由和统一脚本；不接数据库，两个员工客户端只调用 Edge API。

验证：待前端工程初始化后，在 `frontend/` 内独立执行 lint、typecheck、unit test 和 build；两个应用都能启动并显示开发环境标识；生产模式默认关闭 dev actor。当前尚未运行前端验证。

## F2 建立契约与 API Client

实现 Core/Edge 分离的请求上下文、响应/错误归一化、超时取消、request/trace ID、环境地址和开发 Actor 适配器；为当前已实现的 Arrival、Confirm、Projection、Accept、Complete 建立 DTO/schema 和 Mock handler，并为个人微信/企业微信登录态预留 AuthAdapter，不让平台身份进入共享业务域。

验证：测试覆盖成功、空数据、400、401/403、404、409、422、500、503、202；断言 Core client 不会请求 Edge 路径，Edge client 不会请求内部同步路径。

## F3 建立共享 UI 与 Mock 场景库

建立状态徽标、错误面板、请求诊断、加载骨架、空状态、确认弹窗、响应式布局和 fixture 场景；场景覆盖 BVS2-AT-05、AT-06、AT-08、AT-09、AT-10、AT-12、AT-15、AT-16、AT-17、AT-23、AT-25、AT-26 的前端可见部分。

验证：组件测试检查键盘操作、禁用状态、非颜色提示和错误信息；Mock 可切换正常/冲突/延迟/失败而不修改页面业务代码。

## F4 员工端微信小程序 Projection MVP

实现微信小程序的我的任务列表、任务详情、业务状态映射、Accept/Complete 按钮和 Command 显示层状态；个人微信与企业微信共用页面。202 只进入 pending/syncing，Projection 更新后才进入 confirmed；取消和完成状态不可执行。小程序本地缓存只用于展示优化，不实现离线可靠写入。

验证：Mock 场景覆盖空列表、assigned→Accept、in_progress→Complete、completed、cancelled、重复点击、网络超时、旧 Projection 和版本冲突；分别验证个人微信和企业微信入口最终映射到同一 Staff。真实 Edge 联调前先执行 Docker 前置检查，并且只调用公开 Edge 业务接口。

## F5 管理端任务工作台与 Confirm

实现任务列表、详情、Candidate 排序/资格提示、确认弹窗和 `expected_task_version`/`confirmation_id` 管理。Core 列表/详情 API 未就绪时，页面由同一 DTO 接口接 Mock，不能伪造真实接口已上线。

验证：Mock E2E 覆盖 awaiting_confirmation、candidate shortage、forbidden、candidate_no_longer_eligible、task_already_assigned、stale_task_version、confirmation_id_conflict；确认操作重复提交只保留一个 in-flight 请求。

## F6 认证、路由守卫和 Scope 展示

在后端正式认证契约冻结后接入 SessionProvider、登录/登出、过期处理、角色/Scope 导航和 Core/Edge 各自的认证注入；移除生产构建中的测试 Header 适配。

验证：测试未登录、过期、无权限、跨 Team/Area、staff 非本人任务和管理员不能伪造员工 Command；前端只负责显示和路由控制，后端拒绝仍作为最终结果。

## F7 真实契约联调与文档收敛

将 Mock 按接口逐项切换到 Core/Edge；同步修正 OpenAPI、前后端交接、前端 contracts 和联调说明。Core 任务列表/详情/取消基础实现已存在，仍需完成最终字段契约；Edge 命令状态查询或明确降级文案所需的后端契约仍待冻结。

验证：联调前执行 `scripts/ensure-docker.ps1`；按项目安全边界使用隔离数据，完成 health、migration、API、同步和恢复检查；不执行 `down -v`、TRUNCATE、DROP 或破坏性 migration。

## F8 前端验收与 BVS2-06 对齐

执行管理端和员工端关键路径、权限、重复请求、网络恢复、乱序 Projection、刷新恢复和可访问性验收；将真实结果写入 `memory-bank/progress.md`，将新增目录职责写入 `memory-bank/architecture.md`。

验证：前端 lint/typecheck/unit/contract/E2E 与必要的后端端到端结果均有实际命令输出；`git diff --check`、`git status` 检查无意外文件或敏感配置；未运行的检查不写成已通过。

## 当前明确阻塞项

在 F6/F7 之前，以下事项不能被前端自行猜测：

- 个人微信、企业微信、员工 Web 和管理端的正式登录接线、Staff 绑定与测试账号；
- Core 任务列表、详情、Arrival 创建、Confirm、Cancel 的最终字段契约；基础路由已实现，但合同还需收敛；
- Edge 员工身份来源、客户端 Command 幂等键以及 Command 状态查询；
- OpenAPI 与实际路由的差异收敛；
- 可重复业务种子数据和双端联调环境。
