# 前端 API 接入说明

> 状态：F3 员工任务生命周期和 Core Scope E2E 已接入并在隔离双库通过；员工 Web 的 Command 收据支持刷新后恢复查询；个人微信/企业微信 Provider 适配器、原生小程序页面壳和管理端企业微信 SSO callback/Core 会话服务已加入，真实凭据/域名、员工与管理员主数据预置、服务故障注入和最终品牌评审仍待完成。

## 客户端边界

| 客户端 | API | 当前实现 |
| --- | --- | --- |
| `apps/admin-web` | Core API | Task List、Task Detail、Confirm、Cancel |
| `apps/employee-web` | Edge API | 工号密码登录、Refresh、Me、Logout、Projection、Accept、Complete、Command Status |
| `apps/employee-miniapp` | Edge API | 原生 `wx.request` 网络适配、会话存储、登录/任务/任务详情/账号页面壳和同一组员工接口 |

浏览器只访问公开业务 API。Core/Edge 的 `/internal/sync/v1/*`、MySQL、Redis、Outbox、Inbox 和 Worker 不属于前端边界。

## 本地运行

在 `frontend/` 内执行：

```powershell
Copy-Item .env.example .env.local
pnpm.cmd install
pnpm.cmd dev:admin
pnpm.cmd dev:employee
```

如果 PowerShell 拦截 `npm.ps1`/`pnpm.ps1`，使用对应的 `.cmd` 命令即可；Node.js、npm 和 pnpm 已配置到当前用户 PATH，本项目不要求修改 PowerShell 执行策略。

两个 Vite 开发服务器分别使用 4174 和 4175 端口，并通过 `/core-api`、`/edge-api` 代理到本地 Core/Edge 端口，避免本地开发时直接产生跨域请求。

员工 Web 基础 E2E 需要本地服务和临时测试账号：

```powershell
$env:E2E_EMPLOYEE_NO = "<employee-no>"
$env:E2E_EMPLOYEE_PASSWORD = "<password>"
$env:E2E_BASE_URL = "http://127.0.0.1:4175"
pnpm.cmd e2e
```

用例默认校验 `FL-LIVE-001`、中文任务名称和区域；其他隔离 fixture 可通过 `E2E_EXPECTED_FLIGHT`、`E2E_EXPECTED_TASK_NAME`、`E2E_EXPECTED_AREA_NAME` 覆盖。账号只通过环境变量注入，不写入仓库。

F3 真实生命周期和 Scope E2E 还需要一条已经由 Core Confirm、并已同步到 Edge 的员工任务，以及对应的隔离员工账号：

```powershell
$env:E2E_LIFECYCLE_TASK_ID = "<assigned-task-public-id>"
$env:E2E_LIFECYCLE_EMPLOYEE_NO = "<employee-no>"
$env:E2E_LIFECYCLE_EMPLOYEE_PASSWORD = "<password>"
$env:E2E_SCOPE_TASK_ID = $env:E2E_LIFECYCLE_TASK_ID
$env:E2E_SCOPE_TEAM_ID = "<task-team-id>"
$env:E2E_SCOPE_AREA_ID = "<task-area-id>"
pnpm.cmd exec playwright test
```

`employee-task-lifecycle.spec.ts` 验证真实 `Accept → Command status → Complete → 刷新恢复`；`core-task-scope.spec.ts` 验证主任全局可见、正确团队/区域队长可见、错误 Scope 队长不可见；`edge-error-contract.spec.ts` 验证 Edge 的 401/403/404/409 错误契约。测试只通过 HTTP 访问业务 API，不连接数据库；测试夹具由隔离环境预置。

## UTF-8 编码约束

- 所有源码、JSON 请求体、API 响应、WebSocket JSON 消息和小程序网络数据统一使用 UTF-8；JSON 请求头显式声明 `application/json; charset=utf-8`。
- 服务端 Gin JSON 响应使用 `application/json; charset=utf-8`；Core/Edge MySQL 连接和表使用 `utf8mb4`。前端不做 GBK/Latin-1 转码，也不通过终端默认代码页生成中文测试数据。
- Windows 下准备中文 fixture 时应使用明确的 UTF-8 文件/字节输入，避免 PowerShell 原生命令参数在代码页转换时污染数据。浏览器看到 `è”...` 时，先检查数据库原始 HEX 和响应文本，再判断是否为终端显示问题。

## 管理端认证限制

当前 Core 公开了 Bearer JWT 校验，没有管理用户密码登录，但已经提供默认关闭的 SSO 签发路由。管理端因此提供三种明确模式：

1. 生产模式：粘贴上游 SSO 已签发的 Core JWT；未来只替换 `AdminLoginPage` 的入口适配器。
2. 本地调试模式：显式设置 `VITE_ENABLE_DEV_ACTOR=true`，由 Core 非 release 配置接收 `X-Actor-*` 开发适配头。该模式不会进入生产构建，前端也不会默认发送这些 Header。
3. SSO callback 适配模式：配置 `VITE_ADMIN_SSO_START_URL` 和 `VITE_ADMIN_SSO_EXCHANGE_URL` 后，浏览器携带 `state` 跳转到企业微信并回到 `/sso/callback`；前端只把一次性 `code` 交给后端交换 Core 会话，不在 URL 或浏览器端保存第三方密钥。Core 已提供 `/api/v1/admin/auth/sso/start`、`/exchange`、`/me`、`/logout`，但上线前仍需应用 `000005_admin_sso` migration、预置 `admin_identity` 映射并注入真实企业微信配置。

员工真实 Provider 的流程、配置和安全边界见 [`identity-provider-integration.md`](./identity-provider-integration.md)。个人微信小程序需要原生 `wx.login`，企业微信可以采用 OAuth/H5 code 或企业微信容器内的小程序入口；两者最终都由 Core 映射到同一个 `Staff`。

## 员工会话与命令

员工 Web 登录请求发送：

```json
{
  "employee_no": "E000123",
  "password": "...",
  "client": "employee-web"
}
```

Edge 返回的 Access Token 和 Refresh Token 由 `@flight/auth` 管理。Access Token 失效时，恢复流程尝试 Refresh；Refresh 失败则清理本地会话并回到登录页。Token 不进入 URL、日志或诊断文案。

Accept/Complete 为一次逻辑操作生成 UUID 形态的客户端 `command_id`，发送 `expected_sync_version`；网络失败后的同一操作重试会复用该 ID，内容冲突（409）后才重新建立操作。收到 `202` 后只显示 `pending/syncing`，再通过 `GET /api/v1/commands/{commandID}` 读取 `confirmed/failed`。员工 Web 会把当前 Command 收据作为单条 best-effort localStorage 恢复线索，刷新后继续查询；它不是可靠消息队列，服务端状态仍是唯一事实源。前端不会把 `202` 直接改写为任务已完成。

## 员工任务快照与离线恢复

`GET /api/v1/tasks` 始终返回当前员工的完整 Edge Projection 快照，服务端按 JWT Principal 过滤，不接受客户端指定其他员工。响应中的 `snapshot_at` 是本次读取的观测时间，`projection_revision` 是该员工的持久化变化序列；它不是单个任务 `sync_version` 的全局游标。

当前响应固定为 `sync_mode=full_snapshot`、`next_cursor=null`、`reset_required=false`。启动、刷新、重连、收到未来的变化提示以及离线恢复都重新拉取并替换任务集合；HTTP 200 的 `items=[]` 是合法空结果。取消和完成任务以快照中的终态为准，本地缓存只改善体验，不承担可靠事实或离线队列职责。待处理 Command ID 单独保留，并通过 `GET /api/v1/commands/{commandID}` 恢复状态。

管理端 Confirm/Cancel 同样复用一次逻辑操作的 `confirmation_id`/`cancellation_id`；选择候选人、修改取消原因或服务端返回冲突时，前端会重新建立操作 ID。

## 员工 WebSocket 实时提示

Edge 提供 `POST /api/v1/realtime/ticket` 和 `GET /api/v1/ws`。前端先用员工 Bearer JWT 请求短期一次性 ticket，再将 `protocol` 和 `ticket_protocol` 作为 WebSocket 的 `Sec-WebSocket-Protocol` 候选值；不要把 Access Token 或 ticket 放进 URL 查询参数。

连接成功后服务端先发送 `ready`，随后发送 `ping`；客户端应回复 `{"type":"pong"}`。收到 `task_changed` 只代表 Projection 可能变化，前端应按 `notification_id`/任务版本做轻量去重，然后重新调用 `GET /api/v1/tasks` 并替换任务集合。连接断开、ticket 过期、提示丢失、重复或乱序时，继续使用带上限的指数退避重连和完整快照恢复；WebSocket 不可用不能阻塞任务读取或 Command status 查询。

`employee-web` 已由 `@flight/api-client` 的 `RealtimeClient` 执行上述流程：登录会话存在时自动申请 ticket，连接/重连成功以及收到 `task_changed` 都触发页面级完整快照刷新。ticket 获取返回 401 时停止重连并交给会话恢复；其他网络失败使用带抖动的指数退避。当前 `employee-miniapp` 仍只接入 HTTP/微信网络适配器，未假设小程序可以复用浏览器 WebSocket ticket 流程。

## 仍待接入

- 管理端企业微信 SSO 的生产配置、管理员 `admin_identity` 预置和生产会话安全策略；Core 企业微信会话签发代码已接入；
- 真实个人微信、企业微信 Provider 凭据、回调/通信域名和线上 code 联调；
- 通知、异常、历史、人员、规则、报表等没有公开业务接口的模块；
- 503 浏览器级故障注入、服务重启/网络分区、多副本 fan-out 和容量门禁；当前 401/403/404/409 错误契约、客户端错误映射、Scope、Command 生命周期和刷新恢复已有单元/API/真实隔离 E2E 覆盖。验证前必须运行根目录 `scripts/ensure-docker.ps1`。
## 2026-09-03 Core 管理查询接口

管理端现在可以通过 Core JWT 读取 `GET /api/v1/personnel` 和 `GET /api/v1/assignments`。两个接口由 Core RBAC 和服务端 scope 过滤；前端传入的筛选条件不会替代会话中的权限范围。管理端 SSO 的 Core 会话接口也已加入，但实际身份源、凭据、回调域名和管理员预置仍待部署联调。
