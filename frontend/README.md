# 前端工作区

> 状态：F2 首个真实 API 业务闭环已落地；视觉方案仍待评审，微信小程序页面和管理端正式认证仍待平台条件完善。

本目录与 Go 后端分开维护，但仍属于同一个仓库。前端通过 HTTP API 访问后端，不连接 MySQL、Redis、Outbox、Inbox 或 Worker。

```text
frontend/
├── apps/
│   ├── admin-web/
│   │   └── src/              # 管理端源码，只调用 Core API
│   ├── employee-miniapp/
│   │   └── src/              # 首期员工小程序，只调用 Edge API
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

首期同时建设管理端和员工端：`admin-web` 为桌面优先的响应式 Web，`employee-miniapp` 为同时支持个人微信和企业微信入口的员工端小程序；`employee-web` 为正式可用的员工网页版和备用入口，复用共享契约和任务域。

当前根目录的 `web-admin`、`miniapp` 为空，未作为新的前端工程入口使用。新的前端代码全部放入本目录；正式 Web 工程已采用 React、TypeScript、Vite 和 pnpm workspace。微信小程序保留独立原生平台适配层，不复用 Web DOM 页面。

页面路由、视觉 Token、认证/绑定、任务工作台、员工任务详情、同步状态、响应式和可访问性基线见 [`docs/page-design.md`](docs/page-design.md)。本轮实现优先落地真实功能状态，视觉美化仍作为后续修改项。

## 已接入的真实功能

- `admin-web`：通过 Core API 读取任务列表/详情，并提交 Confirm、Cancel；管理端不在前端本地扩大任务 Scope。
- `employee-web`：通过 Edge API 使用工号密码登录，恢复可刷新长期会话，读取本人 Projection，提交 Accept/Complete，并查询 Command 状态。
- `employee-miniapp`：已提供 `wx.request`、微信存储和 Edge 会话适配层；真实 AppID、通信域名、Provider code 回调和小程序页面注册待平台配置后接入。
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

管理端默认使用显式开发 Actor 或粘贴已签发的 Core JWT；员工端使用真实 Edge 工号密码接口。当前后端没有公开的管理用户登录路由，因此管理端 SSO 适配器仍是明确的后续接入点。

## F0 Mock 页面预览

当前可以直接查看 [`preview/index.html`](preview/index.html) 页面地图，再进入独立的管理端、员工端和登录页。预览覆盖完整管理端导航骨架、主任/队长 Scope 视图、任务工作台与详情、员工端任务/通知/异常/历史/账号，以及关键 Mock 状态迁移。

```powershell
python -m http.server 4173 --directory frontend/preview
```

启动后打开 <http://127.0.0.1:4173>。预览说明见 [`preview/README.md`](preview/README.md)。该目录仍是脱离后端的探索性 Mock，不是正式运行入口；正式入口是 `apps/admin-web` 和 `apps/employee-web`。

## 前后端分离硬边界

- 所有前端运行时代码、应用配置、静态资源、Mock、契约测试、组件测试和 E2E 脚本必须位于 `/frontend`；根目录不得重新建立 `src/`、`package.json`、`node_modules/` 或 Web 构建入口。
- `admin-web` 只调用 Core API；`employee-miniapp` 和 `employee-web` 只调用 Edge API。前端不连接 MySQL、Redis、Outbox、Inbox、Worker 或 `/internal/sync/v1/*`。
- 前端共享包只保存 DTO、错误模型、展示状态映射、API client、认证适配接口和 Mock；不得复制 Core 的授权、事务或最终业务状态机。
- 性能优先于视觉装饰：关键操作不能等待动画，默认只使用短时 `transform`/`opacity` 动效；高成本持续特效默认禁止，具体规则见 `../docs/performance-and-reliability-baseline.md`。
- 项目级架构、进度和前后端交接文档可以保留在 `memory-bank/` 与根 `docs/`，它们不是前端运行时内容；实现时仍以 `/frontend` 为唯一前端源码边界。
