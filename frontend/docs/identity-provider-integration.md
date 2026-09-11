# 身份与 SSO 接入方案

## 结论

需要开发两个员工小程序：个人微信小程序和企业微信小程序。它们只负责员工端的平台入口和页面，不是整个认证系统。

- 个人微信：原生小程序调用 `wx.login()` 获取一次性 code，业务后端向微信 `code2Session` 换取 OpenID/UnionID，再在 Core 的绑定表中映射到员工 `Staff`。
- 企业微信：员工端使用独立的企业微信小程序入口，调用 `wx.qy.login()` 取得一次性 code；后端使用企业微信 access token 调用小程序 `jscode2session` 解析企业成员 `UserID`。这条路径与管理端浏览器 OAuth 的 `getuserinfo` 兑换接口分开。
- 管理端：浏览器 Web 不需要再开发一个小程序。当前项目没有 OIDC，管理端使用企业微信 OAuth；浏览器回调一次性授权码，后端验证后签发 Core 管理会话。未来如果企业已有其他身份源，再单独增加 Provider。

因此，员工端有三个独立入口（员工 Web、个人微信小程序、企业微信小程序），管理端是另一个独立的 Web 客户端。它们共用“服务端身份映射和会话”能力，但不共用平台页面：

```text
个人微信小程序 ───────┐
企业微信小程序 ───────┼─> Edge ─> Core 身份 Provider ─> 项目员工库 Staff / Session
员工 Web ────────────┘

管理端 Web ─> 管理 SSO 回调 ─> Core 管理会话 ─> Core Task API
```

## 已实现的工程边界

### 员工身份

Core 新增真实 Provider 适配器，默认仍关闭：

- `personal_wechat`：调用 `https://api.weixin.qq.com/sns/jscode2session`，仅保存绑定所需的 OpenID，不把 `session_key` 下发给 Edge 或客户端。
- `wecom` 浏览器授权适配：调用企业微信 `gettoken` 和 `getuserinfo`，只接受企业成员 `UserId` 映射到内部员工，不把企业微信 access token 下发给客户端。
- 企业微信小程序的 code 使用单独的 `miniprogram/jscode2session` 接口兑换；后端只保留必要的成员标识，不把 `session_key` 下发给客户端。
- Provider application scope 由后端配置决定，客户端不能伪造 AppID、CorpID 或绑定范围。
- 企业微信 access token 只做短期内存缓存；Core 的 Staff、绑定、Audit 和 Edge Session 仍是持久事实。

核心配置位于 `configs/config.v2.yaml` 的 `identity`：

```yaml
identity:
  real_providers_enabled: false
  personal_wechat_app_id: ""
  personal_wechat_secret: ""
  wecom_corp_id: ""
  wecom_agent_id: ""
  wecom_secret: ""
  wecom_miniapp_code2session_url: ""
```

生产环境应通过 `FLIGHT_IDENTITY_*` 环境变量或密钥管理系统注入 Secret，并将 `real_providers_enabled` 显式设为 `true`。不允许把 Secret 放入 `frontend`、小程序包、Git 或浏览器地址栏。

个人微信小程序原生壳位于 `frontend/apps/employee-miniapp`，企业微信小程序原生壳位于 `frontend/apps/employee-wecom-miniapp`；两者都提供登录、任务、详情、账号页。它们复用现有 Edge Session 协议：首次绑定使用项目员工工号密码加当前平台的 Provider code，后续使用当前平台 code 直接换取 Edge Session。received/start/complete 仍通过 Edge Command；received 只表示已收到通知，HTTP 202 不是最终业务成功。

### 员工数据库边界

因为没有 OIDC，平台使用项目自有的员工主数据和凭证库作为登录事实源。这套员工库独立于企业现有 HR/员工数据库，不做同步，也不要求企业 HR 系统提供员工账号；在当前冻结架构中，它作为 Core MySQL 内的项目员工主数据模块落地，不增加第三套物理数据库。Core 已拥有员工库的逻辑表：

- `operation_area`、`team`、`team_member`：组织、区域和团队关系。
- `personnel`：员工工号、姓名、团队、岗位、能力、工作状态和启用状态。
- `employee_credential`：员工密码 bcrypt 摘要、失败次数和锁定时间。
- `external_identity_binding`：个人微信/企业微信与同一 `personnel` 的绑定关系。
- `admin_identity`：管理端企业微信身份到 `admin/manager/leader` 及 Scope 的映射。

管理后台的人员管理模块应通过受权限保护的 Core API 新增员工、维护员工工号/姓名/岗位、创建团队/区域并把员工加入对应分工组；员工主数据和凭证的新增、停用、重置密码以及企业微信 UserId 绑定都必须在项目员工库内完成。当前代码已提供人员/Assignment 查询，人员写入、分工组维护和受控导入仍属于下一业务切片，前端页面暂显示明确空态，不伪造写入成功。个人微信小程序、企业微信小程序和员工 Web 只通过 Edge 登录，Edge 不直连这些表。

### 管理端 SSO

`admin-web` 已加入可配置的浏览器适配器：

- `VITE_ADMIN_SSO_START_URL`：后端 SSO 授权起始地址。
- `VITE_ADMIN_SSO_EXCHANGE_URL`：后端一次性 code 交换地址。
- `admin-web` 生成并保存 `state`，回调校验 `state`，将 `code` POST 到后端；Core JWT 不放在 URL 中。
- 生产后端必须校验 `state`、授权码有效期、redirect URI、企业归属、用户启用状态和管理角色，并返回 Core audience 的管理 JWT；前端不自行决定 `admin/manager/leader` 权限。当前 Core 已有 `admin_identity`、`admin_sso_state`、`admin_session` 迁移、企业微信 authorization-code Provider、一次性 state 消费、管理会话解析和公开路由：`GET /api/v1/admin/auth/sso/start`、`POST /api/v1/admin/auth/sso/exchange`、`GET /api/v1/admin/auth/me`、`POST /api/v1/admin/auth/logout`。

这部分已经是可配置的真实接入骨架，但默认关闭，不能直接视为已上线。没有 OIDC 时可将 `identity.admin_sso_provider` 设置为 `wecom`，复用企业微信 CorpID/AgentID/Secret，预置 `admin_identity` 映射，配置 HTTPS 回调地址和安全密钥后完成线上联调；如果未来接入其他身份源，再增加对应 Provider。

## 联调前必须准备

1. 注册个人微信小程序并配置服务器 HTTPS request 域名；准备 AppID/AppSecret。
2. 在企业微信管理后台创建企业微信小程序/应用，准备 CorpID、AgentID、Secret，并配置企业微信小程序合法域名；管理端将 `admin_sso_provider` 设置为 `wecom`，管理端回调使用 HTTPS 可信域名。
3. 提供 Core 可访问微信和企业微信接口的出网 HTTPS、超时、代理和证书策略。
4. 准备一条已启用 Staff 和工号密码；第一次登录只用于完成绑定，之后由 Core 绑定表复用同一员工身份。
5. 确认管理端身份源及管理员映射规则；在没有这一步前，只能继续使用开发 Actor 或人工注入 Core JWT。启用 SSO 时还要先应用 Core `000005_admin_sso` migration，并预置已启用的 `admin_identity`。
6. 将真实 Provider、SSO 回调、通信域名和生产环境错误/重试测试加入 CI 和发布门禁。

官方文档入口：

- [微信小程序登录](https://developers.weixin.qq.com/miniprogram/dev/framework/open-ability/login.html)
- [微信 `code2Session`](https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-login/code2Session.html)
- [企业微信开发者文档](https://developer.work.weixin.qq.com/)
## 2026-09-03 管理端 SSO 后端状态

Core 已提供管理端 SSO 的后端合同和会话实现：`GET /api/v1/admin/auth/sso/start`、`POST /api/v1/admin/auth/sso/exchange`、`GET /api/v1/admin/auth/me` 和 `POST /api/v1/admin/auth/logout`。Core 会校验 redirect allow-list、一次性消费 state、查找预置的 `admin_identity`，并签发 Core audience JWT；角色和 Team/Area/User scope 在后端会话中恢复，前端不能自行扩大权限。

当前实现默认关闭。上线前仍需提供企业微信 CorpID/AgentID/Secret、HTTPS 回调域名、Secret 管理和 `admin_identity` 预置；这些生产配置与平台联调没有在本地伪造完成。当前不依赖 OIDC；若未来增加其他身份源，再新增对应 Provider。
