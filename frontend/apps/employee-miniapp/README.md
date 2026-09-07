# 员工微信小程序

这里是个人微信小程序的原生平台适配层，不复用员工 Web 的 DOM 页面。`src/platform/edge-gateway.ts` 将 `wx.request` 和 `wx` 本地存储适配到共享的 Edge 会话与任务契约，业务映射到项目员工库中的同一个 Staff。企业微信小程序位于同级的 `employee-wecom-miniapp`，是独立发布的客户端。

当前未绑定真实 AppID、通信域名或微信/企业微信 Provider。`src/app/session.ts` 中的地址是占位配置，不能用于生产。

真实登录的最小流程是：

1. 个人微信小程序调用 `wx.login()`，将一次性 code 交给 `exchangeProviderCode(code)`；服务端携带 AppID/AppSecret 调用微信 `code2Session`。
2. 首次使用时先用 `passwordLoginAndBind` 提交工号和密码，再用新 code 完成一次性绑定；绑定后由 Edge 签发可刷新、可撤销的会话。
3. 企业微信身份在 `employee-wecom-miniapp` 中完成；该客户端使用企业微信原生登录 code，不把企业微信 secret 放进小程序。
4. 绑定数据由 Core 以 `provider + provider_app + external_subject` 唯一约束，同一员工可以同时绑定个人微信和企业微信；两个小程序和员工 Web 读取同一份 Edge Projection。

接入前必须配置 HTTPS API 域名、个人微信 AppID/AppSecret、企业微信 CorpID/AgentID/Secret，并由后端启用 `identity.real_providers_enabled`。真实密钥只进入后端环境变量或密钥管理系统，不提交到仓库。页面的 `app.json`、路由和通信域名属于真实小程序工程配置，不能由员工 Web 页面替代。

`app.json` 和 `pages/` 是原生小程序页面注册壳，页面逻辑以 TypeScript 编写。发布前需要在选定的微信开发者工具/小程序构建链中执行 TypeScript 转换，并将 `src` workspace 依赖打包到小程序目录；当前仓库的 pnpm `typecheck` 只检查共享适配层，不把原生 DevTools 产物当成 Web bundle。

本地生成可交给微信开发者工具打开的独立包：

```powershell
$env:MINIAPP_EDGE_API_BASE_URL = "http://127.0.0.1:8082"
pnpm build:miniapps
```

生成目录为 `frontend/dist/employee-miniapp` 和 `frontend/dist/employee-wecom-miniapp`。上线构建必须提供真实 HTTPS Edge 地址并设置 `MINIAPP_RELEASE=true`；脚本会拒绝把占位地址打进发布包。打开生成目录后，将 `project.config.json` 中的 `appid` 替换为对应小程序 AppID，并在微信开发者工具中配置合法 request/socket 域名。
