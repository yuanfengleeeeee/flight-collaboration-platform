# 员工企业微信小程序

这里是企业微信小程序的独立原生入口，不复用个人微信小程序的登录页，也不连接企业 HR/员工数据库。业务数据和员工账号仍来自项目自有员工库，客户端只调用 Edge API。

登录边界：

1. 已绑定员工使用 `wx.qy.login()` 获取原生登录 code，由 Core 通过企业微信小程序 `jscode2session` 换取员工身份，再换取 Edge Session。
2. 首次使用通过项目员工工号和密码建立身份，再用企业微信 code 完成绑定。
3. 工号密码也可以作为恢复入口；绑定关系最终指向 Core 的同一条 `personnel` 记录。
4. 企业微信 Secret、Core 数据库凭据和管理端 SSO 凭据都不进入小程序包。

`employee-wecom-miniapp` 与 `employee-miniapp` 共享任务、会话和错误契约，但使用不同的 `client` 标识，便于按平台撤销、审计和配置通信域名。

当前 `src/app/session.ts` 中的 Edge 地址是占位配置，发布前必须替换为 HTTPS 域名，并在企业微信管理后台配置可信域名。

本地生成可交给企业微信开发者工具打开的独立包：

```powershell
$env:MINIAPP_EDGE_API_BASE_URL = "http://127.0.0.1:8082"
pnpm build:miniapps
```

生成目录为 `frontend/dist/employee-wecom-miniapp`。上线构建必须提供真实 HTTPS Edge 地址并设置 `MINIAPP_RELEASE=true`；脚本会拒绝把占位地址打进发布包。打开生成目录后，将 `project.config.json` 中的 `appid` 替换为企业微信小程序 AppID，并在企业微信管理后台配置合法 request/socket 可信域名。
