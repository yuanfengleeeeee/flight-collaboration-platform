# 前端设计基线

> 当前前端业务与后端合同的设计基线。任务语义以 [`docs/business-process-v2.md`](../docs/business-process-v2.md) 为准；本文件不再使用旧版“员工 Accept/拒绝”模型。

## 客户端定位

| 客户端 | 用户 | API | 设计重点 |
| --- | --- | --- | --- |
| `admin-web` | admin、manager、leader、supervisor | Core | 高信息密度任务运行台、Scope 内工作、Assignment、变更审批、人员和字典管理 |
| `employee-web` | staff | Edge | 移动优先任务卡片、收件回执、执行状态、异常报告和恢复 |
| `employee-miniapp` | staff | Edge | 个人微信原生登录、任务操作、通知和实时提示 |
| `employee-wecom-miniapp` | staff | Edge | 企业微信原生登录、任务操作、通知和实时提示 |

管理端和员工端是不同的前端产品，但共享 DTO、错误模型、状态映射和 API Client；不共享数据库，不复制 Core 业务规则。

## 业务流程在界面上的表达

```text
pending_dispatch -> assigned -> 我已收到 -> 开始执行 -> 已完成
                         \-> 异常申请 -> manager/admin 审批
```

- `pending_dispatch`：显示“系统正在自动派发”，管理端可以看到候选短缺，员工端不显示可执行按钮。
- `assigned`：员工看到工作地点、岗位、航班和计划时间，操作为“我已收到”。
- `received`：只代表员工已收到通知，不能解释为同意或拒绝。
- `in_progress`：员工点击“开始执行”后显示执行中。
- `completed`：员工点击“完成”并等待服务端确认后显示终态。
- 延误、取消、冲突和突发事件统一进入异常/任务变更申请，不提供员工拒绝按钮。

管理端只在 `manager/admin` 的权限与 Scope 满足时显示审批操作；`leader` 可以查看、报告、跟进；`supervisor` 只读观察。

## 页面工作区

### 管理端

- 任务运行台：按状态、航班、区域、班组和时间筛选；服务端分页。
- 任务详情：显示航班事实、模板快照、候选、Assignment、三层握手、历史、审计和待处理变更申请。
- Assignment/短缺：显示自动派发结果、候选冲突、收件超时和无替补状态。
- 变更审批：显示申请原因、建议动作、版本、申请人和审批结果；审批失败保留原因。
- 人员与字典：员工、岗位、能力、状态、班组和区域；编码只读，删除为停用。
- 航班与运行：只读显示外部航班事实、来源新鲜度和 fallback 诊断，不提供任意新增航班。

### 员工端

- 我的任务：只显示本人 Edge Projection；刷新、重连和恢复读取完整员工快照。
- 任务详情：显示工作位置、岗位要求、航班信息和同步状态；提供“我已收到”“开始执行”“完成”。
- 异常报告：选择保障冲突、延误、取消、突发事件等原因，可提出受控变更建议；提交后显示 Command/申请状态。
- 通知、历史、账号：均从 Edge 分页接口读取；实时 socket 只作为刷新提示。

## 状态展示规则

业务状态和网络/Command 状态必须分开显示：

| 层 | 示例 | 来源 |
| --- | --- | --- |
| 业务状态 | `pending_dispatch`、`assigned`、`in_progress`、`completed`、`cancelled` | Edge Projection / Core 查询 |
| 员工收件 | `assigned` + `receipt_status=received` | Core/Edge 事件投影 |
| Command 状态 | `pending`、`syncing`、`confirmed`、`failed` | Edge Command status |
| 航班源健康 | `fresh`、`stale`、`fallback`、`failed` | Core source health |

HTTP `202` 只能渲染为“已提交/同步中”，不得本地乐观改成业务成功。失败、过期或断线时优先提供重试、刷新和诊断入口，不覆盖服务端事实。

## 实时与分页

- 管理端订阅 Core 的 Scope 过滤 SSE；员工端使用 Edge WebSocket 或原生 socket。
- `task_changed`、`command_changed` 仅为 best-effort hint；收到提示、重连或恢复时重新读取服务端快照。
- 管理集合、历史和通知统一使用 `page/page_size/total` 与稳定排序；禁止在浏览器一次性获取无界列表后过滤。
- 员工任务完整快照是按员工隔离的恢复机制，不是管理端全量接口。

## 设计与性能约束

- 操作反馈使用短时 `transform`/`opacity` 过渡，支持 `prefers-reduced-motion`；关键操作不能等待动画。
- 不使用持续 WebGL/Canvas 粒子、动态渐变、大面积 blur、鼠标视差、全量列表 stagger 或大型动效资源。
- 长列表采用分页和必要的远程搜索；请求支持取消/去重；本地缓存只优化展示，不承担可靠写入。
- 视觉状态不能掩盖同步延迟、接口失败或权限拒绝。

## 当前设计验收边界

已完成：四客户端边界、任务状态映射、收件/开始/完成语义、异常申请入口、分页合同和实时恢复原则。仍需在真实环境验证平台登录、域名/TLS、管理员预置、双入口联调、多副本故障和性能基线。
