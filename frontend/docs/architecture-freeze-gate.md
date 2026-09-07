# 前端工程架构冻结门槛

> 状态：`F0 IN_PROGRESS`
> 更新时间：2026-09-07
> 业务流程以 [`docs/business-process-v2.md`](../../docs/business-process-v2.md) 为准，当前后端交接以 [`docs/frontend-backend-handoff.md`](../../docs/frontend-backend-handoff.md) 为准。

## 1. 已冻结的客户端边界

| 客户端 | 数据边界 | 首期职责 |
| --- | --- | --- |
| `admin-web` | Core API | 任务运行台、Assignment、任务变更审批、人员、岗位/能力、模板、运行监控 |
| `employee-web` | Edge API | 收件回执、开始/完成、异常申请、通知、历史、账号 |
| `employee-miniapp` | Edge API | 个人微信登录、员工任务、异常和原生实时提示 |
| `employee-wecom-miniapp` | Edge API | 企业微信登录、员工任务、异常和原生实时提示 |

浏览器和小程序都不得调用 Core 数据库或 `/internal/sync/v1/*`。共享包只承载 DTO、错误模型、状态映射、API Client、会话和平台适配；权限、事务、最终状态和可靠同步由后端负责。

## 2. 已冻结的业务接入合同

### 航班

航班是外部事实，不允许前端通过管理端任意新增、修改或删除。前端只读取 Core 的分页航班事实；航班同步、到达、健康诊断属于受保护的内部集成入口。

### 任务

```text
外部航班同步 -> 航班到达 -> pending_dispatch -> 自动派发
  -> Core Assignment -> Edge Projection -> received -> start -> complete
```

`received` 只表示员工已收到通知，不代表同意或拒绝；员工没有拒绝操作。如果员工正在保障其他航班，使用异常/任务变更申请报告。任务变更包括 `pause`、`reassign`、`reschedule`、`cancel`、`resume`，只能由 `manager/admin` 审批应用。

HTTP `202` 只表示 Command 已被 Edge 接收。前端必须等待 Command status 和 Projection 更新，不能把本地点击直接渲染为最终成功。

## 3. 当前冻结门槛

| 门槛 | 状态 | 当前事实 | 关闭条件 |
| --- | --- | --- | --- |
| Core/Edge 物理边界 | `DONE` | 管理端走 Core，员工端走 Edge | 持续由 API Client 和 CI 负向检查保护 |
| Task 生命周期与状态映射 | `DONE` | `pending_dispatch`、`assigned`、`received`、`in_progress`、`completed`、`cancelled` 已分层 | 用故障注入覆盖重复消息、断线、延误、取消和超时重派 |
| 自动派发/变更审批合同 | `DONE` | 候选规则、三层握手、超时重派、申请和经理审批已有代码/契约 | 正式环境验证缺员、冲突、审批失败和权限边界 |
| 分页读取 | `DONE` | Core 管理集合、Edge 历史/通知使用 `page/page_size/total` | 用 100+ 数据夹具做浏览器验证；选项超过上限时改远程搜索 |
| 员工 Command 幂等与恢复 | `PARTIAL` | 稳定 `command_id`、Command status 和刷新恢复已接入 | 验证重复提交、超时、失败重试和 Projection 延迟 |
| 工号密码登录 | `PARTIAL` | Core 凭证、bcrypt、Edge 登录/刷新/撤销已实现 | 补齐迁移在线验证、密码修改/重置和生产会话策略 |
| 微信身份映射 | `PARTIAL` | 个人微信/企业微信 Mock Provider 与同一 Staff 绑定已实现 | 接入真实凭据、域名并验证绑定冲突、换绑、离职失效 |
| 管理端 SSO | `PARTIAL` | Core 管理身份、授权码状态和会话代码已实现 | 配置真实身份源、回调域名、管理员预置并联调 |
| 实时提示 | `PARTIAL` | 管理 SSE、员工 WebSocket/原生 socket 已实现；均为提示通道 | 验证重连、服务重启、网络分区、多副本和告警观测 |
| 环境与平台发布 | `BLOCKED` | 真实小程序 HTTPS 域名、Web 域名、回调和证书清单未完成 | 完成 dev/test/prod 域名、TLS、密钥注入和平台配置 |
| 性能基线 | `PENDING` | 已确定可靠性优先和动效限制，尚无代表性业务负载报告 | 固定 p95/p99、长列表、内存、重连和小程序性能门禁 |

## 4. 页面与角色验收重点

- `admin-web`：任务列表/详情、待派短缺、Assignment、任务变更审批；只有 manager/admin 显示可审批操作。
- `leader`：查看自己 Scope 内任务，接收实时提示，报告冲突或异常，不显示审批按钮。
- `supervisor`：只读查看其订阅范围，不显示任务变更审批按钮。
- `employee-*`：展示“我已收到”“开始执行”“完成”和异常申请；不展示“接受/拒绝任务”二选一。
- 所有列表服务端分页，断线/刷新后从服务端快照恢复；本地缓存和实时提示都不是可靠事实。

## 5. F0 关闭条件

F0 关闭前必须完成：

1. Core/Edge OpenAPI、错误码、Bearer JWT、分页和状态映射与当前代码一致。
2. 四个客户端完成 API Contract、Mock fixture 和最小冒烟验证。
3. 真实平台身份、域名、TLS、管理员预置和小程序配置完成隔离环境联调。
4. 覆盖航班接口失效/fallback、重复消息、断线、延误、取消、收件超时、无候选、变更审批失败等场景。
5. 按 `docs/performance-and-reliability-baseline.md` 形成真实数据规模下的性能与可靠性报告。

未实际运行的测试、迁移、Docker/Compose 或平台联调不得标记为通过。
