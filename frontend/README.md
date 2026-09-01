# 前端工作区

> 状态：目录骨架已创建；前端应用、依赖、构建配置和业务页面尚未实现。

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

当前根目录的 `web-admin`、`miniapp` 为空，未作为新的前端工程入口使用。新的前端代码应放入本目录；React、TypeScript、Vite、pnpm workspace 仍以 `memory-bank/frontend-tech-stack.md` 中的 `PROPOSED` 方案为准，待 F0/F1 冻结和初始化后再提交可运行配置。

## 前后端分离硬边界

- 所有前端运行时代码、应用配置、静态资源、Mock、契约测试、组件测试和 E2E 脚本必须位于 `/frontend`；根目录不得重新建立 `src/`、`package.json`、`node_modules/` 或 Web 构建入口。
- `admin-web` 只调用 Core API；`employee-miniapp` 和 `employee-web` 只调用 Edge API。前端不连接 MySQL、Redis、Outbox、Inbox、Worker 或 `/internal/sync/v1/*`。
- 前端共享包只保存 DTO、错误模型、展示状态映射、API client、认证适配接口和 Mock；不得复制 Core 的授权、事务或最终业务状态机。
- 项目级架构、进度和前后端交接文档可以保留在 `memory-bank/` 与根 `docs/`，它们不是前端运行时内容；实现时仍以 `/frontend` 为唯一前端源码边界。
