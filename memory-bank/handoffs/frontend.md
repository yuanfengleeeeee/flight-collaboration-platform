# 前端任务交接

> 更新时间：2026-09-03
>
> 状态：F3 员工任务生命周期、Core 角色 Scope 和 Command 收据恢复已落地；隔离双库真实 E2E 已通过登录/Projection/WebSocket ready、主任/队长 Scope、Accept/Complete/刷新恢复；真实 Provider 适配器、原生小程序页面壳和管理端企业微信 SSO callback 已加入，员工主数据/管理员预置、生产凭据和域名仍待完成

## 当前范围

- `admin-web`：桌面优先响应式 Web，只调用 Core API。
- `employee-miniapp`：首期员工入口，个人微信和企业微信共用业务页面，只调用 Edge API。
- `employee-web`：正式支持的员工网页版和备用入口，只调用 Edge API。
- 所有前端运行时代码、构建配置、Mock、测试和 E2E 位于 `/frontend`；不直连 MySQL、Redis、Outbox、Inbox、Worker 或 `/internal/sync/v1/*`。

## 已完成

- 已创建 `frontend/` 工作区目录骨架、三个客户端目录、共享包、E2E 和 docs 目录。
- 已形成前端设计、技术栈、实施计划、架构冻结门槛和身份合同提案。
- 已确定 Core/Edge 客户端边界、业务状态与 Command 显示状态分离、202 pending 不能直接当作最终成功。
- 已形成三端页面信息架构基线：管理端、员工 Web、小程序和各自登录页独立；管理端首期完整工作区；员工端包含任务、通知、异常、历史和账号；详见 [`frontend/docs/page-design.md`](../../frontend/docs/page-design.md)。视觉美化仍待评审，不把当前 Token 视为最终品牌方案。
- 已加入性能优先约束：同步准确性优先于动画；关键操作不等待动画；高成本持续特效默认禁止。
- 已新增 [`frontend/preview/index.html`](../../frontend/preview/index.html) 页面地图，以及独立的 `login-admin.html`、`login-employee.html`、`admin.html`、`employee.html`；管理端演示主任/队长 Scope 和完整导航空态，员工端演示任务、通知、异常、历史、账号及 Accept/Complete pending 状态。
- 已按确认项实现 Mock 权限视图：主任可见全部团队/区域任务；队长视图仅显示“客舱保障组 · T2”任务，系统管理入口显示为需授权；该逻辑只用于预览，最终权限仍由 Core Scope 决定。
- 预览只用于页面和状态确认，不连接真实 API；所有预览文件仍位于 `/frontend`，没有把根目录 `web-admin` 或 `miniapp` 恢复为新入口。
- 已落地正式 `frontend/package.json`、pnpm workspace、TypeScript/Vite 配置，以及 `packages/contracts`、`api-client`、`auth`、`task-domain`、`ui`、`mock`。
- `admin-web` 已接入真实 Core Task List/Detail/Confirm/Cancel；`employee-web` 已接入真实 Edge 工号密码登录、Refresh/Me/Logout、Projection、Accept/Complete 和 Command Status；`employee-miniapp` 已加入 `wx.request` 和微信存储适配层。
- `employee-miniapp` 已加入原生 `app.json`、登录/任务/任务详情/账号页面壳；个人微信入口调用 `wx.login` 获取一次性 code，企业微信入口支持由企业微信 OAuth/H5 或容器注入 code，工号密码登录仍可单独使用。
- `admin-web` 已加入可配置企业微信 SSO 起始地址、state 校验、一次性 code 回调交换和 Core token 会话适配；回调 URL 不携带 Core JWT，Core 管理会话服务已接入，仍需真实企业微信配置和管理员预置。
- 已在隔离 Compose 项目 `frontend-live` 上用当前源码 Core API、Edge API 和 Worker 完成真实到达生成、主任确认、Projection 同步、员工登录/Refresh/Me、Accept、Complete、Command 状态和重复命令验证；员工 Web 已在 `http://localhost:4175/login` 实际登录并读取真实 Edge Projection。
- 已统一 JSON 请求 UTF-8 声明：Web `ApiClient`、小程序 `wx.request`、Edge→Core Identity HTTP、Worker→Edge Sync HTTP 均使用 `application/json; charset=utf-8`；服务端 JSON 响应为 `application/json; charset=utf-8`，MySQL DSN/表使用 `utf8mb4`。联调中发现的隔离测试数据乱码已按 UTF-8 原始字节修复，并在浏览器确认中文正常显示。
- `employee-web` 已接入 `@flight/api-client` 的 `RealtimeClient`：自动获取一次性 WebSocket ticket、回复应用层心跳、按 notification ID 去重、指数退避重连，并在连接成功/重连/`task_changed` 后触发完整任务快照刷新；`employee-miniapp` 当前仍只使用 HTTP/微信网络适配器。
- `employee-web` 顶部实时连接状态现在反映 `connecting/open/retrying/closed/unauthorized`；实时会话未授权时清理员工会话并回到登录页，避免把断线或失效状态显示为已连接。
- `employee-web` 现在将当前 Accept/Complete 的 Command 收据作为单条 best-effort localStorage 恢复线索保存；页面刷新后按原 `command_id` 继续查询 Edge 状态，服务端确认/失败或 409 冲突后清理/重建收据，不把本地缓存当作可靠队列。
- Command 状态查询遇到临时网络/503 时最多自动重试 15 次；重试耗尽仍保留收据，用户刷新页面可以重新恢复查询，不会把异常误报成业务完成。
- 新增 `frontend/e2e/employee-task-lifecycle.spec.ts`，真实验证员工 `Accept → Command status confirmed → Complete → 刷新恢复已完成`；新增 `frontend/e2e/core-task-scope.spec.ts`，验证主任全局可见、正确团队/区域队长可见、错误 Scope 队长列表为空且详情 404。
- 新增 `frontend/e2e/edge-error-contract.spec.ts`，真实验证 Edge 401/403/404/409 稳定错误码和 UTF-8 响应；503 非 JSON 上游错误由 API Client 单元测试覆盖。
- 新增 `frontend/e2e/README.md`，明确真实 E2E 只访问 HTTP/浏览器业务 API，账号、任务、团队和区域通过环境变量/隔离夹具注入，不写入凭据。
- `@flight/api-client` 新增 401 错误回调、非 JSON 503 响应的稳定错误映射测试；新增 Command 收据持久化测试，所有 JSON 测试数据/请求继续使用 UTF-8。
- 管理端当前没有后端公开登录签发接口，正式页面支持 Core JWT 注入或显式非 release 开发 Actor；不会伪造管理端密码登录。
- 真实业务页面与预览页面已经分开：`frontend/apps/*` 是正式源码，`frontend/preview/` 仍是静态探索 Mock。

## 尚未实现或待验证

- 503 浏览器级故障注入、服务重启/网络分区、多副本 fan-out 和容量门禁；401/403/404/409 错误契约、客户端错误映射、Core Scope、Command 生命周期和刷新恢复已有单元/API/真实隔离 E2E 覆盖。
- WebSocket 服务重启/网络分区、多副本 fan-out 和容量门禁；当前真实浏览器握手已通过，故障和扩展场景仍主要由客户端 fake socket 与后端代码级测试覆盖。
- 前端认证 Adapter 接入与真实平台联调：BVS2-07 后端已提供本地 Mock Provider、工号密码、个人微信/企业微信绑定、Session 刷新和撤销；真实个人微信 `code2Session`、企业微信 `gettoken/getuserinfo` 适配器已加入但默认关闭，真实凭据、通信域名和在线平台联调仍待完成。
- 管理端正式 SSO/管理用户认证：前端 callback 与 Core 企业微信 SSO 签发/会话解析已加入；当前不依赖 OIDC，企业归属策略和 `admin_identity` 预置仍待联调。
- 通知、异常、历史、人员、规则、报表等尚无公开业务接口的内容页。
- 最终 OpenAPI、Task 生命周期矩阵、Mock/测试账号、环境域名和性能实测；团队/区域 Scope 交叉规则、最终页面路由合同和最终品牌视觉仍待评审。
- 前端性能指标、资源体积、帧率、内存和长任务的实际设备测量。

## 性能硬约束

- Confirm、Cancel、Accept、Complete、Retry 不得等待动画或特效结束，必须立即发起请求并显示真实状态。
- 默认只使用短时 `transform`/`opacity` 过渡，支持 `prefers-reduced-motion`，不使用 `transition: all`。
- 默认禁止全屏视频、WebGL/Canvas 粒子、复杂 3D、视差/鼠标跟随、大面积 blur/backdrop-filter、动态渐变、无限循环装饰、全量列表 stagger 和大型动效资源。
- 前端优化优先处理代码分包、长列表、请求取消/去重、内存生命周期和小程序增量更新；本地缓存不能成为可靠消息队列。
- 详细规则和后端性能待办见 [`docs/performance-and-reliability-baseline.md`](../../docs/performance-and-reliability-baseline.md)。

## 下一步

1. 将现有 Playwright/Contract 测试夹具纳入 CI，并补充可重复创建的隔离生命周期数据；按项目规则先执行 Docker 前置检查，不自动清理或重置数据。
2. 在安全配置中注入真实 AppID/AppSecret、CorpID/AgentID/Secret 和 HTTPS 回调/通信域名，预置管理员 `admin_identity`，完成个人微信、企业微信员工入口和企业微信管理端 SSO 线上联调。
3. 如后续接入其他身份源，再增加对应 Provider；当前不依赖 OIDC。
4. 扩展 Mock/Contract/E2E 夹具，补上 503 浏览器故障注入、202 pending 长时间等待和重复提交场景。
5. 执行视觉评审、性能/可访问性回归，再扩展通知、异常、历史、人员、规则和报表模块。

## 验证状态

- 前端正式 `pnpm lint`：通过；`pnpm typecheck`：通过；`pnpm test`：通过（5 个测试文件、13 个测试，其中 SessionManager 3 个、realtime 客户端 2 个、API Client 3 个、TaskCommandStore 2 个、task-domain 3 个）；`pnpm build`：通过（admin-web、employee-web）。
- 本轮新增真实 Provider 的 httptest 覆盖：个人微信 code2Session 成功映射、企业微信 access_token 缓存和 userinfo 映射；原生小程序页面注册与管理端 SSO callback 适配已完成静态检查，真实平台调用尚未执行。
- 真实隔离 E2E 定向通过 4 个：基础员工登录/中文 Projection/WebSocket ready/刷新恢复、主任/队长 Scope、Accept→Complete→刷新恢复、Edge 401/403/404/409 错误契约。首次并行全套运行时基础场景因两条同名任务导致测试定位不唯一，已收紧为按航班号定位并单独复跑通过；未将首次失败写成通过。
- 静态 Mock 页面检查：已完成；4 个预览文件存在，无尾随空格，关键模式和交互标记已检查，`git diff --check` 无错误（仅有既有换行格式提示）；Python 本地 HTTP 资产冒烟确认 `index.html`、`preview.css`、`preview.js` 均返回 HTTP 200。
- Docker 前置检查：本轮已确认 Docker Engine 就绪；隔离项目 `frontend-live` 的 Core/Edge MySQL 保持运行，当前源码 Core API `:8081`、Edge API `:8082`、Worker 已用于真实联调。`go test ./...`、`go build ./...`、前端 lint/typecheck/test/build 和 `git diff --check` 均通过；标准 Compose 镜像构建仍受 Docker Hub EOF 影响，未把缓存镜像当作当前源码服务。
- 源码级核查：JSON 配置解析通过；`git diff --check` 通过（仅有既有 LF/CRLF 提示）；未发现正式源码中的 `require()`、`transition: all`、旧版预览跳转或内部同步 API 调用。Node.js v24.20.0、npm 11.19.0、pnpm 9.15.0 已安装并写入当前用户 PATH；E2E 使用环境变量注入临时账号，不把凭据写入仓库。
- 视觉回归：使用本机 Chrome 对员工登录页和真实员工任务页进行截图检查；本轮调整已验证登录卡片、实时状态、任务卡航班识别条、底部导航和移动触控尺寸，最终品牌视觉仍需产品评审。
## Current session handoff (2026-09-03)

- The backend now exposes direct Enterprise WeChat management SSO start/exchange/me/logout plus `GET /api/v1/personnel` and `GET /api/v1/assignments`; frontend contracts/API client types were added for the two read models.
- The `admin-web` callback adapter remains compatible with the Core SSO contract. Real Enterprise WeChat credentials, callback domains, and `admin_identity` provisioning are still deployment work; this deployment does not require OIDC.
- No frontend source changes were made in this handoff turn and no new frontend verification was run.
## Correction for current session (2026-09-03)

- The current backend slice also updated `frontend/packages/contracts` and `frontend/packages/api-client` with personnel and assignment read-model types/client methods; this was not a UI implementation.
- The previously recorded frontend verification remains valid for the existing frontend changes; no new frontend commands were run during this handoff.
