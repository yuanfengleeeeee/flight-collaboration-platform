# 前端身份合同提案

> 状态：`PARTIAL / BVS2-07 IMPLEMENTED / F0 INTEGRATION REVIEW`
>
> 更新时间：2026-09-01
>
> 目标：冻结个人微信、企业微信、员工 Web 和管理端进入平台后的身份边界；本文件不接入真实微信账号，也不包含生产密钥。

## 1. 审计结论

当前代码已经完成员工身份基础闭环的本地实现，但真实微信/企业微信 Provider 和线上环境验证仍未完成：

- Core 已持有员工凭证、外部身份绑定和一次性绑定票据；密码使用 bcrypt，失败次数受限并支持停用员工校验。[`internal/core/application/identity`](../../internal/core/application/identity)
- Edge 已提供工号密码登录、Provider exchange、绑定完成、Refresh 轮换、当前会话和 Logout；员工 Session 使用 Edge audience 的短期 JWT 和可撤销 Refresh Token。[`internal/edge/application/identity`](../../internal/edge/application/identity)
- Core/Edge 身份数据通过独立的 `000003` migration 落地；Edge 不读取 Core 身份表，而是通过受保护的内部 Identity Port 调用 Core。[`migrations/core/mysql/000003_employee_identity.up.sql`](../../migrations/core/mysql/000003_employee_identity.up.sql)、[`migrations/edge/mysql/000003_employee_session.up.sql`](../../migrations/edge/mysql/000003_employee_session.up.sql)
- 开发 Provider 只接受显式 `mock:<subject>`，真实个人微信/企业微信校验、生产非对称签名和在线双库验证尚未完成。[`internal/integration/identity`](../../internal/integration/identity)

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
6. `admin-web` 只使用 Core audience 的会话；`employee-miniapp`、`employee-wecom-miniapp` 和 `employee-web` 只使用 Edge audience 的会话。一个客户端拿到的 Token 不应被另一个 API 接受。
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

### 4.2 员工微信小程序与企业微信小程序

个人微信小程序和企业微信小程序进入相同的业务页面结构，但由不同客户端和 `AuthAdapter` 获取入口授权结果：

```text
个人微信小程序或企业微信小程序启动
  → 使用当前客户端固定的 provider（personal_wechat / wecom）
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

管理端对 Core Task 的确认/取消和任务变更审批、员工在任一入口发出的 received/start/complete，最终都通过既有 Core Event → Edge Projection 链路收敛。用户可能短时间看到旧状态，这是可靠同步的可见延迟，不是两套数据；前端必须展示刷新/同步状态，不能用本地结果覆盖服务端状态。received 只表示已收到通知，不是同意；员工没有拒绝任务命令。

## 5. 公共认证合同

以下是 BVS2-07 已落地的公共 Edge 认证接口最小形状；字段和错误码以 `api/edge/openapi.yaml` 与实际实现为准，真实微信/企业微信 Provider 仍使用后续适配器替换：

### 5.1 工号密码首次登录

员工客户端通过 Edge API 公开入口调用：

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

### 5.3 绑定和当前会话

```http
POST /api/v1/auth/bindings/complete
Content-Type: application/json
```

```json
{
  "binding_ticket": "<one-time-ticket>",
  "provider": "wecom",
  "provider_code": "<one-time-code>",
  "client": "employee-miniapp"
}
```

服务端验证 `provider_code` 后将其绑定到 binding_ticket 对应的 Staff，再返回与 5.2 相同的 Edge Session 响应。绑定接口不能接受客户端提供的 `staff_public_id`。

### 5.4 当前会话

```http
GET /api/v1/auth/me
Authorization: Bearer <platform-token>
```

返回当前平台身份、客户端 audience、会话到期时间和展示用角色标签。它不返回完整权限列表、外部平台标识或 Core 敏感人员档案。

### 5.5 登出/撤销

```http
POST /api/v1/auth/logout
Authorization: Bearer <platform-token>
```

服务端撤销当前 `sid` 或使其进入不可继续换取状态。Access Token 过期、Staff 被停用、外部绑定被解除时，后续 API 必须返回稳定的 `session_expired`、`staff_inactive` 或 `identity_unbound`。

### 5.6 长期会话与简易登录

推荐默认值：Access Token 有效期约 15 分钟，Refresh/Session 有效期 30 天并在正常使用时滚动续期，绝对最长 90 天；具体数值由后端安全策略确认。小程序使用平台安全存储，Web 优先使用 `HttpOnly + Secure + SameSite` 会话 Cookie 或等价安全方案，不能把长期 Refresh Token 放进 URL 或普通 `localStorage`。

长期会话续期建议使用受保护的刷新接口：

```http
POST /api/v1/auth/refresh
Authorization: Bearer <refresh-session-credential>
```

刷新成功后轮换会话凭据；旧刷新凭据再次使用时视为重放并撤销该会话。刷新接口不改变 Staff 映射，也不重新创建任务数据。

用户体验表现为：首次需要工号密码，完成绑定后日常从个人微信或企业微信一键进入；换设备、退出登录、密码修改、Staff 停用、管理员撤销或风险控制触发时，再要求工号密码或重新绑定。长期登录必须可按设备/会话撤销，不能通过永久 JWT 实现。

## 6. 建议的 Token 与 Session 规则

- `sub`：平台业务身份的 `PublicID`；员工会话为 `StaffPublicID`，管理会话为管理用户的公共身份 ID。
- `sid`：平台 Session 公共 ID，用于撤销、审计和排障；不能使用外部平台 ID。
- `iss` / `aud`：由可信平台签发；建议 Core、Edge 使用区分 audience，防止跨边界复用。
- `iat` / `exp`：短期访问令牌；具体 TTL 由后端安全策略确认。
- `roles` / `scopes`：不由客户端提交；是否进入签名声明由后端确认，服务端仍需执行 RBAC/Scope。
- Access Token 不写入日志、错误消息、Task/Command payload 或浏览器 URL；前端持久化方式按小程序安全存储和 Web 会话策略分别实现。
- 当前本地 `HS256` 配置只用于开发/测试；生产密钥、算法、轮换、JWKS/证书和 Token 撤销策略必须由后端单独冻结。

## 7. 绑定关系建议

BVS2-07 已在 Core 增加受控的外部身份绑定用例和持久化模型，并在 Edge 增加最小会话模型；本节保留跨平台上线时仍必须遵守的规则：

| 规则 | 建议 |
| --- | --- |
| 唯一键 | `provider + provider_app + external_subject` 唯一；不使用 `tenant_id` 或 `airport_id` |
| 目标 | 只能指向一个 active/可审计的 Core `Staff` 或管理用户 |
| 首次绑定 | 先通过工号 + 密码认证，再由当前微信入口完成显式绑定；禁止按姓名模糊匹配 |
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

前端 `SessionState` 至少覆盖：`unknown`、`authenticating`、`binding_required`、`authenticated`、`expired`、`forbidden`、`unavailable`。未进入 `authenticated` 前，不加载 Task Projection，不渲染可提交的 received/start/complete 或异常申请动作。

## 9. F0 需要确认的决策

| 决策 | 推荐默认值 | 需要确认 |
| --- | --- | --- |
| 小程序项目形态 | 一个业务小程序壳层，内部按 provider 适配个人微信/企业微信 | 平台主体、AppID、企业微信关联和发布主体是否允许这样配置 |
| 员工 Web 登录 | 必须支持工号 + 密码；已绑定身份作为快捷入口 | 是否还要在浏览器内直接接入个人微信授权 |
| 会话签发 | 统一逻辑身份层签发平台 Token，Core/Edge 使用不同 audience | 先落在现有 API 进程，还是已有统一 SSO/Auth 服务承载 |
| Staff 首次绑定 | 工号 + 密码认证后显式绑定；不自动按姓名/手机号匹配 | 业务部门提供绑定办理流程和责任角色 |
| 长期/简易登录 | 短 Access Token + 30 天可撤销 Session，绝对最长 90 天 | 是否需要管理员强制下线和设备列表 |
| 会话撤销 | 服务端按 `sid` 撤销；Staff 停用、改密和解绑自动失效 | 是否需要管理员强制下线和设备列表 |
| Token 算法与轮换 | 生产使用非开发密钥的非对称签名和可轮换公钥 | 既有企业 SSO/OIDC/JWKS 能力及接入方 |

## 10. 验收条件

身份合同关闭前必须完成：

1. 使用脱离真实微信平台的 Mock Provider，个人微信和企业微信测试身份最终得到同一个 `StaffPublicID`。
2. 未绑定、冲突、Staff 停用、Session 过期、错误 audience 和重复换取均有稳定响应。
3. `admin-web` 不能用员工 Token 调用 Core 管理接口；员工客户端不能用 Core Token 调用 Edge。
4. 生产构建不接受 `X-Actor-*` 或 `X-Employee-Public-ID`；开发适配器必须显式开启且不能进入 release。
5. 后端完成工号密码认证、绑定关系、Session、审计、撤销、刷新和 `GET /auth/me` 的真实合同，OpenAPI 与实现一致。
6. 前端 AuthAdapter 只输出统一 `SessionState`，不把外部身份字段传播到共享业务域；个人微信和企业微信测试登录最终得到同一个 `StaffPublicID`。
7. 至少验证首次登录、双入口绑定、任一入口读取更新、长期会话续期、退出/撤销、改密和 Staff 停用场景。

身份合同的页面和本地 Mock 形态已经可以进入前端实现；真实个人微信/企业微信配置、生产签名密钥、在线双库验证、密码管理页面和管理员全设备撤销仍属于 F0/F1 上线门槛。
