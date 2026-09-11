# 前端工作区

> 状态：管理端、员工 Web、个人微信小程序和企业微信小程序的当前业务页面与适配器已落地；真实平台凭据、域名、告警和发布配置属于部署验收阶段。

当前业务流程以根目录 [`docs/business-process-v2.md`](../docs/business-process-v2.md) 为准：外部航班同步后自动生成 `pending_dispatch` 任务，系统自动派发，员工通过 `received` 回报收件、通过 `start` 开始执行；员工不能拒绝任务，冲突和异常通过变更申请上报。

本目录与 Go 后端分开维护，但仍属于同一个仓库。前端通过 HTTP API 访问航空客运地面代理后端，不连接 MySQL、Redis、Outbox、Inbox 或 Worker。

```text
frontend/
├── apps/
│   ├── admin-web/
│   │   └── src/              # 管理端源码，只调用 Core API
│   ├── employee-miniapp/
│   │   └── src/              # 个人微信员工小程序，只调用 Edge API
│   ├── employee-wecom-miniapp/
│   │   └── src/              # 企业微信员工小程序，只调用 Edge API
│   └── employee-web/
│       └── src/              # 正式员工 Web 入口，只调用 Edge API
├── packages/
│   ├── contracts/src/        # DTO、枚举、schema、错误码
│   ├── api-client/src/       # Core/Edge HTTP Client
│   ├── auth/src/              # 认证适配接口
│   ├── ui/src/                # 共享 UI 基础组件
│   ├── task-domain/src/       # 前端展示状态映射
│   └── mock/src/              # fixture 和 Mock handler
├── e2e/                       # Playwright 端到端场景
└── docs/                      # 前端本地开发和联调说明
```

首期同时建设管理端和员工端：`admin-web` 为桌面优先的管理后台 Web；`employee-web` 为正式可用的员工网页版；`employee-miniapp` 和 `employee-wecom-miniapp` 分别是个人微信小程序和企业微信小程序。四个入口复用共享契约和任务域，但保持平台运行时与登录入口独立。

当前根目录的 `web-admin`、`miniapp` 为空，未作为新的前端工程入口使用。新的前端代码全部放入本目录；正式 Web 工程已采用 React、TypeScript、Vite 和 pnpm workspace。微信小程序保留独立原生平台适配层，不复用 Web DOM 页面。

页面路由、Web 视觉 Token、认证/绑定、任务工作台、员工任务详情、同步状态、响应式和可访问性基线见 [`docs/page-design.md`](docs/page-design.md) 与 [`docs/web-design-system.md`](docs/web-design-system.md)。原生小程序页面地图和平台适配基线见 [`docs/mobile-design-system.md`](docs/mobile-design-system.md)。接口和状态机以 [`../docs/frontend-backend-handoff.md`](../docs/frontend-backend-handoff.md) 为准。

## 已接入的真实功能

- `admin-web`：通过 Core API 读取任务/Assignment/运行工作台，执行受控补派、Confirm、Cancel 和任务变更审批；管理端不在前端本地扩大任务 Scope。
- `employee-web`：通过 Edge API 使用工号密码登录，恢复可刷新长期会话，读取本人 Projection，提交 received/start/complete 和异常申请，并查询 Command 状态。
- `employee-miniapp`：个人微信小程序，已提供完整任务/通知/历史/异常/账号页面、`wx.login` 快捷登录、工号密码绑定、`wx.request`、微信存储和原生实时刷新适配层。
- `employee-wecom-miniapp`：企业微信小程序，已提供完整员工页面、独立的 `wx.qy.login` 入口、`wx.request`、会话存储和原生实时刷新适配层；与个人微信小程序使用不同 `client` 标识，但映射到同一项目员工库。
- 所有 API 请求、Token、Command ID 和状态都由共享包统一处理；前端不连接数据库，也不调用内部同步接口。

详细联调说明见 [`docs/api-integration.md`](docs/api-integration.md)。

## 正式 Web 工程本地运行

```powershell
Copy-Item .env.example .env.local
pnpm.cmd install
pnpm.cmd dev:admin
pnpm.cmd dev:employee
```

当前 PowerShell 执行策略会拦截 `npm.ps1`/`pnpm.ps1` 包装脚本时，请使用对应的 `.cmd` 命令；Node.js、npm 和 pnpm 已配置到当前用户 PATH，无需修改执行策略。

管理端默认使用显式开发 Actor 或粘贴已签发的 Core JWT；生产环境可通过企业微信 OAuth callback 换取 Core 管理会话。员工 Web、个人微信小程序和企业微信小程序都使用项目员工工号密码及已绑定平台身份登录。

## F0 Mock 页面预览

当前可以直接查看 [`preview/index.html`](preview/index.html) 页面地图，再进入独立的管理端、员工端和登录页。预览覆盖完整管理端导航骨架、主任/队长 Scope 视图、任务工作台与详情、员工端任务/通知/异常/历史/账号，以及关键 Mock 状态迁移。

```powershell
python -m http.server 4173 --directory frontend/preview
```

启动后打开 <http://127.0.0.1:4173>。预览说明见 [`preview/README.md`](preview/README.md)。该目录仍是脱离后端的探索性 Mock，不是正式运行入口；正式入口是 `apps/admin-web` 和 `apps/employee-web`。

## 前后端分离硬边界

- 所有前端运行时代码、应用配置、静态资源、Mock、契约测试、组件测试和 E2E 脚本必须位于 `/frontend`；根目录不得重新建立 `src/`、`package.json`、`node_modules/` 或 Web 构建入口。
- `admin-web` 只调用 Core API；`employee-miniapp`、`employee-wecom-miniapp` 和 `employee-web` 只调用 Edge API。前端不连接 MySQL、Redis、Outbox、Inbox、Worker 或 `/internal/sync/v1/*`。
- 前端共享包只保存 DTO、错误模型、展示状态映射、API client、认证适配接口和 Mock；不得复制 Core 的授权、事务或最终业务状态机。
- 性能优先于视觉装饰：关键操作不能等待动画，默认只使用短时 `transform`/`opacity` 动效；高成本持续特效默认禁止，具体规则见 `../docs/performance-and-reliability-baseline.md`。
- 项目级架构、进度和前后端交接文档可以保留在 `memory-bank/` 与根 `docs/`，它们不是前端运行时内容；实现时仍以 `/frontend` 为唯一前端源码边界。
