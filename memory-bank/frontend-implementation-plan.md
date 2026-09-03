# 前端工程实施计划

> 状态：F2 源码与前端工具链验证已完成；员工 Web 真实登录、中文 Projection、WebSocket ready 和刷新恢复 E2E 已通过；管理端 SSO、真实 Provider、异常场景 E2E 和完整 CI 门禁仍待推进
> 更新时间：2026-09-02
> 依据：`memory-bank/frontend-design-document.md`、`memory-bank/frontend-tech-stack.md`

## 实施原则

- 先冻结目标端形态、认证和 API 契约，再写业务页面；
- Mock 先行，真实 API 逐条替换；未实现接口不得在生产构建中开放；
- 每个步骤都有独立验证；当前步骤未验证前不进入下一步；
- 前端从服务端状态重建 UI，不用本地点击结果代替 Core Event/Edge Projection；
- 任何真实后端、数据库、Compose 或双库验证前，先执行根目录 `scripts/ensure-docker.ps1`。

## F0 设计冻结与交接确认

首期同时建设管理端和员工端已确认；员工端首期采用同时支持个人微信与企业微信入口的微信小程序，并保留正式员工 Web。用户进一步确认首次使用工号 + 密码认证、双微信绑定同一员工账号、任一入口读取同一份数据，以及绑定后的简易长会话体验。F0 当前按 [`frontend/docs/architecture-freeze-gate.md`](../frontend/docs/architecture-freeze-gate.md) 执行，重点冻结身份绑定/刷新/撤销、员工 Web 登录、Edge Command 幂等键/状态查询、OpenAPI 一致性、Task 生命周期、通信域名、Mock/测试数据和视觉基线。

当前身份合同见 [`frontend/docs/identity-contract-proposal.md`](../frontend/docs/identity-contract-proposal.md)，BVS2-07 已完成本地 Mock Provider 下的后端基础实现。页面路由、线框、视觉 Token 和状态矩阵见 [`frontend/docs/page-design.md`](../frontend/docs/page-design.md)；用户已确认角色映射、团队/区域 Scope、管理端完整工作区和员工端扩展入口，视觉美化仍待评审；真实 Provider、Web 会话 Adapter、工具链和在线联调继续按 F0/F1 验证。

## F1 建立前端工作区与应用壳层

正式 `frontend/` workspace 已落地 React/Vite 管理端与员工 Web、共享 contracts/api-client/auth/task-domain/ui、Vite Core/Edge 代理和小程序 `wx.request`/存储适配层；不接数据库，两个员工客户端只调用 Edge API。`frontend/preview/` 继续保留为脱离后端的探索 Mock。

验证：Node.js v24.20.0、npm 11.19.0、pnpm 9.15.0 已配置；`pnpm lint`、`pnpm typecheck`、`pnpm test`、`pnpm build` 已通过。生产模式默认关闭 dev actor。

## F2 建立契约与 API Client（源码已落地）

已实现 Core/Edge 分离的请求上下文、响应/错误归一化、超时取消、环境地址和显式开发 Actor 适配器；已为 Task List/Detail、Confirm/Cancel、Projection、Accept/Complete、登录/绑定/刷新/登出和 Command Status 建立 DTO/API Client，并为个人微信/企业微信登录态提供 AuthAdapter 边界，不让平台身份进入共享业务域。

验证：源码中已覆盖真实请求路径和错误映射；工具链构建已通过，仍待浏览器和隔离后端环境验证成功、空数据、401/403、404、409、503、202 以及 Core/Edge 路径隔离。

## F3 建立共享 UI 与 Mock 场景库

建立状态徽标、错误面板、请求诊断、静态加载骨架、空状态、确认弹窗、响应式布局和 fixture 场景；动画只用于短时反馈和状态变化，不引入持续高成本特效。场景覆盖 BVS2-AT-05、AT-06、AT-08、AT-09、AT-10、AT-12、AT-15、AT-16、AT-17、AT-23、AT-25、AT-26 的前端可见部分。

验证：组件测试检查键盘操作、禁用状态、非颜色提示、错误信息和 reduced motion；Mock 可切换正常/冲突/延迟/失败而不修改页面业务代码，并记录动画只影响视觉、不改变请求时序。

## F4 员工端微信小程序 Projection MVP

实现微信小程序的我的任务列表、任务详情、业务状态映射、Accept/Complete 按钮和 Command 显示层状态；个人微信与企业微信共用页面。202 只进入 pending/syncing，Projection 更新后才进入 confirmed；取消和完成状态不可执行。小程序本地缓存只用于展示优化，不实现离线可靠写入。

验证：Mock 场景覆盖空列表、assigned→Accept、in_progress→Complete、completed、cancelled、重复点击、网络超时、旧 Projection 和版本冲突；分别验证个人微信和企业微信入口最终映射到同一 Staff。真实 Edge 联调前先执行 Docker 前置检查，并且只调用公开 Edge 业务接口。

## F5 管理端任务工作台与 Confirm

实现任务列表、详情、Candidate 排序/资格提示、确认弹窗和 `expected_task_version`/`confirmation_id` 管理。Core 列表/详情 API 未就绪时，页面由同一 DTO 接口接 Mock，不能伪造真实接口已上线。

验证：Mock E2E 覆盖 awaiting_confirmation、candidate shortage、forbidden、candidate_no_longer_eligible、task_already_assigned、stale_task_version、confirmation_id_conflict；确认操作重复提交只保留一个 in-flight 请求。

## F6 认证、路由守卫和 Scope 展示

在后端身份接口完成前端可用的 OpenAPI/Client 合同后接入 SessionProvider、工号密码登录、绑定引导、快捷登录、Refresh、Logout、过期处理、角色/Scope 导航和 Core/Edge 各自的认证注入；移除生产构建中的测试 Header 适配。

验证：测试未登录、过期、无权限、跨 Team/Area、staff 非本人任务和管理员不能伪造员工 Command；前端只负责显示和路由控制，后端拒绝仍作为最终结果。

## F7 真实契约联调与文档收敛

将 Mock 按接口逐项切换到 Core/Edge；同步修正 OpenAPI、前后端交接、前端 contracts 和联调说明。Core 任务列表/详情/取消基础实现已存在，仍需完成最终字段契约；Edge 命令状态查询或明确降级文案所需的后端契约仍待冻结。

验证：联调前执行 `scripts/ensure-docker.ps1`；按项目安全边界使用隔离数据，完成 health、migration、API、同步和恢复检查；不执行 `down -v`、TRUNCATE、DROP 或破坏性 migration。

## F8 前端验收与 BVS2-06 对齐

执行管理端和员工端关键路径、权限、重复请求、网络恢复、乱序 Projection、刷新恢复、可访问性和性能预算验收；检查关键操作没有被动画延迟、长列表没有无界渲染、过期请求会取消、内存资源会释放。将真实结果写入 `memory-bank/progress.md`，将新增目录职责写入 `memory-bank/architecture.md`。

验证：前端 lint/typecheck/unit/contract/E2E 与必要的后端端到端结果均有实际命令输出；`git diff --check`、`git status` 检查无意外文件或敏感配置；未运行的检查不写成已通过。

## 性能专项门禁

性能专项与业务页面并行推进，但不能绕过 F0/F2 的 API、身份和同步契约：

- F1：建立按应用分包、按路由加载、请求取消和统一性能观测的基础；
- F3：建立低成本状态动效、静态加载骨架和 reduced-motion 场景；
- F4/F5：验证员工小程序与管理端在真实状态刷新、重复操作和错误恢复时不出现无界渲染或高成本动画；
- F7/F8：测量 Web INP/LCP/CLS、资源体积、内存、长任务和关键同步链路延迟；不达标时优先删除非必要特效并定位真实瓶颈。

后端同步、数据库、Worker 和 API 的性能待办统一记录在 `docs/performance-and-reliability-baseline.md`，优化前必须保留可靠同步和幂等语义。

## 当前明确阻塞项

当前首个真实 Task Slice 已完成源码接入，以下事项仍不能由前端自行猜测或伪造：

- 管理端正式 SSO/管理用户会话签发，以及生产环境 Web 会话安全策略；
- 个人微信、企业微信真实 Provider、首次绑定/换绑/失效和小程序 AppID、通信域名；
- 通知、异常、历史、人员、规则、报表等尚无公开业务接口的功能内容；
- 共享 Mock/Contract/E2E 夹具、浏览器运行验证和隔离联调数据；
- 可重复业务种子数据、隔离双端联调环境和代表性性能基线。
## 2026-09-01 Command contract update

后端 Command 契约已补齐：员工 Accept/Complete 使用客户端稳定 `command_id`，重复业务内容返回 `duplicate=true`，冲突返回 `409 command_id_conflict`；客户端状态恢复使用 `GET /api/v1/commands/{commandID}`，只消费 `pending/syncing/confirmed/failed` 公共状态。
