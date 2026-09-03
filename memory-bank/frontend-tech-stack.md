# 前端技术栈与工程结构

> 状态：IMPLEMENTED BASELINE；正式 Web 工程和共享 API/会话层已落地，小程序平台适配层已落地
> 更新时间：2026-09-02

React、TypeScript、Vite、pnpm workspace、共享 contracts/API client/auth/task-domain/ui 和两个 Web 应用壳层已经落地。Node.js v24.20.0、npm 11.19.0、pnpm 9.15.0 已配置，依赖已安装，前端 lint/typecheck/unit/build 已通过；浏览器联调和微信小程序仍需真实 AppID、通信域名和平台构建链。

## 1. 技术选型

| 层 | 选择 | 原因与边界 |
|---|---|---|
| 语言 | TypeScript | 在 Core/Edge DTO、状态枚举和错误码之间提供静态约束；不把类型安全当作后端授权替代品 |
| UI 框架 | React | 管理端 Web 和员工 Web 采用 React；员工小程序只复用纯 TypeScript 业务包，不强行复用 Web 页面 |
| 构建 | Web 使用 Vite；小程序使用独立小程序构建链 | Web 端启动快、配置直接；小程序构建与预览由其平台工具链负责，统一由 `/frontend` 脚本编排 |
| 包管理 | pnpm workspace | 共享 client、contracts、UI 和 Mock；首期不引入额外 monorepo 编排器 |
| 员工端运行时 | 微信小程序 | 首期一套小程序同时承载个人微信和企业微信入口；平台 API、页面生命周期和登录态隔离在客户端适配层 |
| 路由 | React Router + 小程序页面路由 | 管理端/备用 Web 使用 React Router；小程序保持原生页面路由和深链适配 |
| 服务端状态 | TanStack Query 或等价查询适配 | Web 使用 TanStack Query；小程序复用查询模型，但不把缓存当业务事实 |
| 本地状态 | React state/reducer | 页面交互和 Command 显示状态有限，不先引入全局 Redux 状态 |
| IO 校验 | Zod 或等价 schema 校验 | 在 API 边界运行时校验响应；OpenAPI 修正并冻结后再考虑生成类型 |
| HTTP | Web `fetch` + 小程序网络适配器 | 统一 Core/Edge Base URL、request/trace ID、取消、错误归一化和重试策略；小程序底层使用平台网络 API |
| UI 组件 | Web 语义组件 + 小程序平台组件适配 | 共享设计 token、状态语义和交互文案；管理端可采用成熟企业组件库，员工小程序不直接依赖管理端布局 |
| Mock | MSW + 共享 fixture 场景 | Web 使用 MSW，小程序使用等价的 fixture adapter；不能调用数据库或内部同步接口 |
| 单元/组件测试 | Vitest + Testing Library | 覆盖状态机、错误码、按钮禁用和可访问性行为 |
| E2E | Playwright | 覆盖关键管理/员工流程、重复点击、202 pending 和冲突刷新 |
| 规范检查 | ESLint + Prettier + TypeScript strict | 与 Go 的格式/构建检查对应；具体版本在前端工作区初始化时锁定 |

员工端首期采用移动端优先的微信小程序，个人微信和企业微信共用页面、任务模型和 Edge API；同时保留正式可用的 `employee-web` 网页版，供直接浏览器访问或微信入口不可用时使用。管理端保持桌面优先的响应式 Web。可靠的 Accept/Complete 写入仍必须经过 Edge 持久化 Command，不能依赖小程序或浏览器本地缓存/离线队列。

`apps/employee-miniapp` 作为首期独立客户端创建。它复用 `packages/contracts`、`packages/task-domain`、错误归一化和 API 契约，分别适配个人微信和企业微信的启动上下文与登录态；共享业务域不得直接依赖 `wx.*`。具体采用原生小程序 TypeScript 还是 React 小程序跨端方案，放到 F1 结合代码复用收益和包体/运行时成本确认。

## 2. 工作区结构

```text
frontend/
├── apps/
│   ├── admin-web/
│   │   └── src/
│   │       ├── app/              # 路由、Session、QueryClient、运行时配置
│   │       ├── features/tasks/   # 列表、详情、候选确认
│   │       └── pages/
│   ├── employee-miniapp/          # 首期员工小程序，个人微信/企业微信共用
│   │   └── src/
│   │       ├── app/              # 启动上下文、Session、平台适配
│   │       ├── features/tasks/   # Projection、Accept、Complete
│   │       └── pages/
│   └── employee-web/              # 正式员工 Web 入口
│       └── src/
│           ├── app/              # 路由、Session、QueryClient
│           ├── features/tasks/   # Projection、Accept、Complete
│           └── pages/
├── packages/
│   ├── contracts/                 # 稳定 DTO、枚举、schema、错误码
│   ├── api-client/                # coreClient、edgeClient、request context
│   ├── auth/                      # SessionProvider、AuthAdapter 接口
│   ├── ui/                        # 状态徽标、错误面板、确认弹窗、布局原语
│   ├── task-domain/               # 前端展示状态映射和不可变状态机辅助
│   └── mock/                      # fixture、MSW handlers、故障场景
├── e2e/                           # Playwright 场景
└── docs/                          # 前端本地运行和联调说明
```

`packages/task-domain` 只能表达展示和交互约束，不复制 Core 的授权或事务规则。任何需要决定业务最终状态的逻辑都必须回到后端。

## 3. 运行时配置

开发环境建议提供不含密钥的配置：

```text
VITE_CORE_API_BASE_URL=http://127.0.0.1:8081
VITE_EDGE_API_BASE_URL=http://127.0.0.1:8082
VITE_FRONTEND_MODE=mock|local-api
VITE_ENABLE_DEV_ACTOR=false
```

真实凭据不能进入仓库或 Vite 构建产物。Base URL 可以公开，Token、JWT secret、数据库密码和内部同步认证材料不能以 `VITE_` 变量暴露。

## 4. 依赖与实现约束

- 依赖版本在 `frontend/package.json` 和 lockfile 中固定；设计阶段不宣称某一未来版本已安装或验证；
- contracts 先以实际代码和冻结业务契约维护，OpenAPI 与实际路由对齐后再接入代码生成；
- 不能把 `X-Actor-*` 或 `X-Employee-Public-ID` 默认写进生产请求；只能由显式开发适配器注入；
- 不在浏览器实现 Outbox/Inbox，不轮询内部同步接口，不直连 Core/Edge MySQL；
- 共享包只共享无副作用的 DTO、UI 和状态映射，不能让 admin-web 通过共享包间接依赖 Edge 数据库边界。

## 5. 性能与动效约束

- 关键操作不等待动画；Confirm、Cancel、Accept、Complete、Retry 必须立即发起请求并显示真实 pending/syncing 状态。
- 默认只使用短时 `transform`/`opacity` 过渡，UI 动画原则上不超过 300ms；必须支持 `prefers-reduced-motion`。
- 禁止默认加入全屏视频/WebGL/Canvas 粒子、视差/鼠标跟随、复杂 3D、大面积 blur/backdrop-filter、动态渐变、无限循环装饰、全量列表 stagger 和大型 Lottie/SVG 动效。
- 不动画化 `width`、`height`、`margin`、`padding`、`top`、`left` 等布局属性；不使用 `transition: all`，不无上限使用 `will-change`、`requestAnimationFrame` 或页面级定时器。
- 管理端和员工端按路由分包，长列表分页/窗口化，请求支持去重、取消和有限退避；不把浏览器缓存或小程序本地缓存当作可靠 Command 队列。
- 目标指标（待 F1/F8 在代表性设备和真实数据量上验证）：Web INP ≤ 200ms、LCP ≤ 2.5s、CLS ≤ 0.1；关键同步操作不能因动画或渲染阻塞。
- 详细限制、后端性能待办和验收方法见 `docs/performance-and-reliability-baseline.md`。

## 6. 构建与验证门

前端工程创建后应提供以下脚本，并在 `frontend/` 工作区执行：

```text
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm e2e
```

若执行会访问真实 Core/Edge、数据库或 Compose 的联调命令，必须先遵守项目根目录 `scripts/ensure-docker.ps1` 的 Docker 前置规则；纯 Mock 单元/组件测试不需要启动后端。
