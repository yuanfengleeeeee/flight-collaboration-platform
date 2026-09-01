# 前端身份合同提案

> 状态：`PROPOSED / F0 REVIEW`
>
> 更新时间：2026-09-01
>
> 目标：冻结个人微信、企业微信、员工 Web 和管理端进入平台后的身份边界；本文件不接入真实微信账号，也不包含生产密钥。

## 1. 审计结论

当前代码已经具备“校验平台 JWT 并注入 `security.Principal`”的基础，但还没有完成外部身份接入闭环：

- `RequireJWT` 可以校验 Bearer Token，并将解析结果放入 Gin Context；生产入口由 JWT 认证，开发 Header 只是显式非 release 适配器。[`internal/platform/httpauth/middleware.go`](../../internal/platform/httpauth/middleware.go:26)
- `core-api` 和 `edge-api` 当前都直接创建 JWT Authenticator；没有外部 Identity Provider、Staff 绑定流程或已接线的 Principal Resolver。[`cmd/core-api/main.go`](../../cmd/core-api/main.go:60)、[`cmd/edge-api/main.go`](../../cmd/edge-api/main.go:50)
- Edge 员工请求最终使用 `Principal.PublicID` 过滤 Projection；没有 Principal 时才会尝试读取开发 Header。[`internal/edge/application/server.go`](../../internal/edge/application/server.go:285)
- Edge migration 已有 `mobile_session` 表，但当前没有对应的登录换取和会话管理用例。[`migrations/edge/mysql/000001_edge_foundation.up.sql`](../../migrations/edge/mysql/000001_edge_foundation.up.sql:70)

因此，前端不能自行把 `openid`、`unionid`、企业微信用户标识、员工号或姓名转换成 `StaffPublicID`。

## 2. 建议冻结的身份模型

```text
个人微信 code ─┐
企业微信 code ─┼─→ 外部身份适配器 ─→ Core 身份绑定 ─→ StaffPublicID
员工 Web 登录 ─┘                                      │
管理端登录 ─────────→ 管理用户 / Role / Scope ─────────┘
                                                        ↓
                                            平台 Session / Bearer JWT
                                                        ↓
                         admin-web → Core API   员工客户端 → Edge API
```

### 2.1 不可变约束

1. `StaffPublicID` 是员工业务身份；外部平台身份只用于登录和绑定，不进入 Task、Assignment 或 Command 的业务主键。
2. 同一名员工的个人微信和企业微信身份必须绑定到同一个 `Staff`，不能创建两份员工账号、两份 Assignment 或两份任务 Projection。
3. 外部身份绑定由服务端完成并审计；客户端只能提交平台签发的临时凭据或授权码，不能提交自定义员工身份 Header。
4. 员工身份未绑定、Staff 非 active、绑定冲突或会话失效时，客户端不得读取任务 Projection，也不得提交 Command。
5. Core 仍是 Staff、Role、Scope 和绑定关系的事实源；Edge 只保存员工端所需的最小 Session/Projection/Command 数据，不直连 Core DB。
6. `admin-web` 只使用 Core audience 的会话；`employee-miniapp` 和 `employee-web` 只使用 Edge audience 的会话。一个客户端拿到的 Token 不应被另一个 API 接受。
7. JWT 只携带最小会话声明。角色和 Scope 由可信服务端解析或通过受控声明注入，前端展示的角色永远不等于最终授权。

### 2.2 用户需求匹配

| 用户需求 | 合同匹配 | 落地方式 |
| --- | --- | --- |
| 首次在小程序登录 | 支持 | 使用工号 + 密码完成平台账号认证；密码只在 HTTPS 认证请求中传输，服务端只保存密码哈希 |
| 个人微信和企业微信绑定同一账号 | 支持 | 工号密码认证后取得一次性绑定会话，分别完成个人微信和企业微信授权；两条绑定最终指向同一个 `StaffPublicID` |
| 任一入口都能看到最新数据 | 支持，但同步有短暂延迟 | 两个入口使用同一个 Staff 会话访问 Edge；Core 事件更新 Edge Projection，不能依赖浏览器本地数据作为事实 |
| 后续登录简单 | 支持 | 已绑定入口通过微信授权直接换取平台会话；工号密码作为首次绑定、换设备和恢复入口 |
| 长时间保持登录 | 支持，但不可永久有效 | 使用短期 Access Token + 可撤销 Refresh/Session；会话失效、Staff 停用或管理员撤销后必须重新认证 |

这里的“同一个账号”指同一个 Core `Staff`，不是把个人微信号或企业微信号直接当成账号。员工 Web 也复用工号密码登录和已绑定身份，不再依赖开发 Header。

## 3. 推荐的会话分层

| 层 | 责任 | 前端可见内容 | 前端不可获得/不可依赖 |
| --- | --- | --- | --- |
| 外部身份 | 个人微信、企业微信、企业 SSO 的登录证明 | 一次性授权结果 | openid/unionid/企业微信标识作为业务身份 |
| 平台身份绑定 | 将 `provider + external_subject` 映射到 Core `Staff` 或管理用户 | `binding_required`、`identity_unmapped` 等结果 | 通过姓名、手机号模糊匹配员工 |
| 平台 Session | 生成会话、过期、撤销和 Token 换取 | `session_public_id`、`expires_at`、短期 Bearer Token | 完整 Token 写日志、写业务表或交给其他客户端 |
| 业务 Principal | Core/Edge 从可信 Token 得到 `PublicID`、Human 类型及授权上下文 | 当前用户基本展示信息、有限角色标签 | 客户端自带角色、Scope、员工 ID 覆盖后端结果 |

推荐由一个逻辑上的“平台身份服务/适配层”签发会话；它可以先作为现有 Core/Edge 进程中的 Application/Integration 模块，不要求现在拆出新的微服务。Core/Edge 通过签名和 audience 验证 Token，不共享业务数据库。

## 4. 推荐登录与绑定流程

### 4.1 首次工号密码登录与绑定

员工首次进入小程序时，流程固定为：

```text
输入工号 + 密码
  → Edge 认证入口转交可信身份适配器
  → 服务端按工号找到 Core Staff 并校验密码/Staff 状态
      ├─ 认证成功且已有绑定 → 签发 Edge Session
      ├─ 认证成功但没有当前 provider 绑定 → 返回一次性 binding_ticket
      ├─ 密码错误/账号锁定 → 返回稳定错误，不泄露账号是否存在
      └─ Staff 非 active → 拒绝登录
  → 当前微信入口授权
  → 服务端验证 provider_code 并绑定到同一个 Staff
  → 签发 Edge Session
```

工号密码校验和外部身份绑定必须由服务端完成。Edge 可以作为员工认证的公开 API 外观，但不得读取 Core 数据库；实际实现应通过可信的身份适配器/内部 Auth Port 获取 Core 的认证结果。Edge 不保存原始密码，也不把密码或外部平台标识写入 Command、Projection 或日志。

绑定个人微信后，员工还可以在“绑定企业微信”入口重复同一流程；绑定企业微信后也可以反向绑定个人微信。若外部身份已指向其他 Staff，必须返回冲突并停止自动迁移。

### 4.2 员工微信小程序

个人微信和企业微信都进入同一套小程序业务壳层，但由不同 `AuthAdapter` 获取入口授权结果：

```text
小程序启动
  → 判断入口 provider（personal_wechat / wecom）
  → 获取一次性 provider_code
  → 调用员工认证换取接口
  → 服务端验证 provider_code
  → 查询外部身份绑定
      ├─ 已绑定且 Staff active → 签发 Edge Session
      ├─ 未绑定 → 返回 binding_required，不读取任务
      ├─ Staff inactive → 返回 staff_inactive
      └─ 冲突/失效 → 返回稳定错误码
  → GET Edge tasks
```

小程序页面不关心不同平台的原始身份字段，只处理统一的 `SessionState`。平台入口差异只能存在于 `AuthAdapter`，不能渗透到 `task-domain` 或共享 DTO。

### 4.3 员工 Web

`employee-web` 是正式支持的员工入口，不默认依赖开发 Header。必须支持工号 + 密码登录；已绑定的微信/企业微信身份可以作为快捷登录入口，是否增加浏览器端个人微信授权 Adapter 只影响入口，不改变 Staff 映射和 Edge API 边界。

### 4.4 管理端 Web

`admin-web` 使用 Core audience 的管理会话。管理用户可以是 `admin`、`manager` 或 `leader`；管理端不通过员工小程序身份冒充管理用户，也不把前端选中的 Role/Scope 作为授权依据。

### 4.5 两个微信入口的数据一致性

个人微信和企业微信绑定完成后，两个入口使用同一个 `StaffPublicID`：

```text
个人微信 Session ─┐
                   ├─→ Edge task_projection(employee_public_id = StaffPublicID)
企业微信 Session ─┘
```

管理端对 Core Task 的确认/取消、员工在任一入口发出的 Accept/Complete，最终都通过既有 Core Event → Edge Projection 链路收敛。用户可能短时间看到旧状态，这是可靠同步的可见延迟，不是两套数据；前端必须展示刷新/同步状态，不能用本地结果覆盖服务端状态。

## 5. 建议的公共认证合同

以下是待后端确认的最小形状，不代表当前接口已经实现：

### 5.1 工号密码首次登录

以下接口是待后端确认的建议合同，员工客户端通过 Edge API 公开入口调用：

```http
POST /api/v1/auth/password/login
Content-Type: application/json
```

```json
{
  "employee_no": "E000123",
  "password": "<password>",
  "client": "employee-miniapp"
}
```

认证成功且尚未完成当前入口绑定时，建议返回受限的一次性 `binding_ticket`，它不能读取任务或提交 Command：

```json
{
  "data": {
    "state": "binding_required",
    "binding_ticket": "<one-time-ticket>",
    "provider": "personal_wechat"
  },
  "request_id": "<request-id>",
  "trace_id": "<trace-id>"
}
```

绑定完成后才签发正常 Edge Session。员工 Web 使用同一接口时，`client` 改为 `employee-web`。

### 5.2 员工会话换取

```http
POST /api/v1/auth/exchange
Authorization: Bearer <provider-issued-or-platform-entry-credential>
Content-Type: application/json
```

```json
{
  "provider": "personal_wechat",
  "provider_code": "one-time-code",
  "client": "employee-miniapp",
  "redirect_uri": "optional-for-web"
}
```

成功响应建议只返回平台会话和业务展示所需的最小身份：

```json
{
  "data": {
    "access_token": "<short-lived-token>",
    "token_type": "Bearer",
    "expires_in": 900,
    "session_public_id": "<uuid>",
    "principal": {
      "public_id": "<staff-public-id>",
      "type": "human",
      "role": "staff"
    }
  },
  "request_id": "<request-id>",
  "trace_id": "<trace-id>"
}
```

`principal.public_id` 只允许是服务端确认后的平台身份；失败时不返回可用于猜测 Staff 的外部标识。

### 5.2 当前会话

```http
GET /api/v1/auth/me
Authorization: Bearer <platform-token>
```

返回当前平台身份、客户端 audience、会话到期时间和展示用角色标签。它不返回完整权限列表、外部平台标识或 Core 敏感人员档案。

### 5.3 登出/撤销

```http
POST /api/v1/auth/logout
Authorization: Bearer <platform-token>
```

服务端撤销当前 `sid` 或使其进入不可继续换取状态。Access Token 过期、Staff 被停用、外部绑定被解除时，后续 API 必须返回稳定的 `session_expired`、`staff_inactive` 或 `identity_unbound`。

## 6. 建议的 Token 与 Session 规则

- `sub`：平台业务身份的 `PublicID`；员工会话为 `StaffPublicID`，管理会话为管理用户的公共身份 ID。
- `sid`：平台 Session 公共 ID，用于撤销、审计和排障；不能使用外部平台 ID。
- `iss` / `aud`：由可信平台签发；建议 Core、Edge 使用区分 audience，防止跨边界复用。
- `iat` / `exp`：短期访问令牌；具体 TTL 由后端安全策略确认。
- `roles` / `scopes`：不由客户端提交；是否进入签名声明由后端确认，服务端仍需执行 RBAC/Scope。
- Access Token 不写入日志、错误消息、Task/Command payload 或浏览器 URL；前端持久化方式按小程序安全存储和 Web 会话策略分别实现。
- 当前本地 `HS256` 配置只用于开发/测试；生产密钥、算法、轮换、JWKS/证书和 Token 撤销策略必须由后端单独冻结。

## 7. 绑定关系建议

建议 Core 增加受控的外部身份绑定用例和持久化模型，但本轮不直接编写 migration：

| 规则 | 建议 |
| --- | --- |
| 唯一键 | `provider + provider_app + external_subject` 唯一；不使用 `tenant_id` 或 `airport_id` |
| 目标 | 只能指向一个 active/可审计的 Core `Staff` 或管理用户 |
| 首次绑定 | 通过预登记邀请码、员工号+二次校验或管理员确认；禁止按姓名模糊匹配 |
| 双入口 | 同一 Staff 可绑定个人微信和企业微信两条外部身份 |
| 冲突 | 一个外部身份已绑定其他 Staff 时返回 `identity_binding_conflict`，不自动迁移 |
| 换绑/解绑 | 受权限控制，写 Audit，使旧 Session 失效；不删除历史绑定记录 |
| 离职/停用 | Staff 非 active 时拒绝换取新会话，并使现有会话在服务端失效 |

## 8. 稳定错误码与客户端状态

| 错误码 | HTTP 建议 | 客户端处理 |
| --- | ---: | --- |
| `provider_code_invalid` | 401 | 重新发起对应平台登录 |
| `identity_unmapped` | 403 | 展示绑定引导，不请求任务 |
| `identity_binding_conflict` | 409 | 停止自动重试，转人工处理 |
| `staff_inactive` | 403 | 清理会话，展示账号不可用 |
| `session_expired` | 401 | 重新换取会话；不重复提交业务 Command |
| `client_not_allowed` | 403 | 检查当前客户端与 Token audience |
| `authentication_unavailable` | 503 | 展示认证服务暂不可用，可安全重试登录 |

前端 `SessionState` 至少覆盖：`unknown`、`authenticating`、`binding_required`、`authenticated`、`expired`、`forbidden`、`unavailable`。未进入 `authenticated` 前，不加载 Task Projection，不渲染可提交的 Accept/Complete 动作。

## 9. F0 需要确认的决策

| 决策 | 推荐默认值 | 需要确认 |
| --- | --- | --- |
| 小程序项目形态 | 一个业务小程序壳层，内部按 provider 适配个人微信/企业微信 | 平台主体、AppID、企业微信关联和发布主体是否允许这样配置 |
| 员工 Web 登录 | 企业内部登录/扫码为主；个人微信 Web 作为明确需求再启用 | 员工 Web 是否必须同时覆盖个人微信浏览器登录 |
| 会话签发 | 统一逻辑身份层签发平台 Token，Core/Edge 使用不同 audience | 先落在现有 API 进程，还是已有统一 SSO/Auth 服务承载 |
| Staff 首次绑定 | 预登记或管理员确认，不自动按姓名/手机号匹配 | 业务部门提供绑定办理流程和责任角色 |
| 会话撤销 | 服务端按 `sid` 撤销；Staff 停用自动失效 | 是否需要管理员强制下线和设备列表 |
| Token 算法与轮换 | 生产使用非开发密钥的非对称签名和可轮换公钥 | 既有企业 SSO/OIDC/JWKS 能力及接入方 |

## 10. 验收条件

身份合同关闭前必须完成：

1. 使用脱离真实微信平台的 Mock Provider，个人微信和企业微信测试身份最终得到同一个 `StaffPublicID`。
2. 未绑定、冲突、Staff 停用、Session 过期、错误 audience 和重复换取均有稳定响应。
3. `admin-web` 不能用员工 Token 调用 Core 管理接口；员工客户端不能用 Core Token 调用 Edge。
4. 生产构建不接受 `X-Actor-*` 或 `X-Employee-Public-ID`；开发适配器必须显式开启且不能进入 release。
5. 后端完成绑定关系、Session、审计、撤销和 `GET /auth/me` 的真实合同，OpenAPI 与实现一致。
6. 前端 AuthAdapter 只输出统一 `SessionState`，不把外部身份字段传播到共享业务域。

身份合同未获得产品/后端确认前，本文件保持 `PROPOSED`，不修改现有 migration，不接入真实第三方账号。
