# 前端任务交接快照

> 更新时间：2026-09-08
> 状态：四端当前页面与 API 接入已落地；真实平台发布和专项验收待完成

业务流程唯一说明：[`docs/business-process-v2.md`](../../docs/business-process-v2.md)。前后端接口边界：[`docs/frontend-backend-handoff.md`](../../docs/frontend-backend-handoff.md)。

## 客户端边界

| 客户端 | 后端入口 | 当前职责 |
| --- | --- | --- |
| `admin-web` | Core API | 管理任务/Assignment、航班只读、组织/人员/模板/字典维护、运行查询和变更审批 |
| `employee-web` | Edge API | 员工登录、本人 Projection、收件/开始/完成、异常上报、通知/历史 |
| `employee-miniapp` | Edge API | 个人微信原生登录、同一员工业务页面和原生实时提示 |
| `employee-wecom-miniapp` | Edge API | 企业微信原生登录、同一员工业务页面和原生实时提示 |

前端不连接 MySQL、Redis、Outbox、Inbox、Worker 或 `/internal/sync/v1/*`。共享包只承载 DTO、错误模型、API Client、认证适配、UI 和展示状态映射，不能复制 Core 权限或状态机。

## 员工任务语义

系统自动生成并派发任务；员工不能拒绝 Assignment。`received` 只表示通知已收到，`start` 表示开始执行，`complete` 表示完成；旧 `accept` 只作为 `start` 兼容入口。

```text
assigned
  → 我已收到（received，收件回执）
  → 开始执行（start）
  → 完成保障（complete）
```

前端对员工 Command 使用稳定 `command_id` 和 `expected_sync_version`。Edge 返回 `202` 时只显示 `pending/syncing`，再通过公开 Command 状态接口恢复 `confirmed/failed`。员工遇到正在保障其他航班、延误、取消或突发事件时，在异常页面提交 `pause/reassign/reschedule/cancel/resume` 请求；员工不能直接改任务，也没有拒绝按钮。

Command 进入 `confirmed` 后，员工端会等待 Edge Projection 的同步版本和目标状态收敛，再刷新任务详情，避免 Worker 已处理命令但 Projection 尚未完成回投时出现旧状态或残留锁。

## 管理端语义

- `pending_dispatch` 表示等待/执行自动派发；`awaiting_confirmation` 仅为历史兼容读取。
- 管理端 Confirm 是自动调度事务的受控补派入口，不是日常员工确认。
- 任务取消保留历史，不是硬删除；变更申请由员工/队长上报，只有值班经理或管理员审批，队长和分管领导不能审批。
- 航班页面只读外部同步事实，不提供航班新增、到达、离港或取消按钮。
- `admin` 具备主数据写权限；`manager` 按授权范围运行管理和审批；`leader` 按区域/班组查看和上报；`supervisor` 只读；`staff` 不进入管理端。
- 菜单、路由和写按钮按当前 Core Scope 提示，但 Core 是最终授权边界；越权必须显示 403/无权限，不能伪装空数据。

## 页面和接口范围

已接入的 Admin Web 页面包括：运行总览、航班只读、任务/详情、Assignment、历史、组织/班组、人员、岗位、能力、模板、规则视图、人员状态、事件、审计、报表、Scope、诊断、任务变更审批和管理员身份。

员工 Web 与两个小程序包括：登录/绑定、任务、任务详情、通知、历史、异常和账号。个人微信与企业微信保持独立平台适配器和发布配置，共享 Edge Projection/Command 合同并映射到同一个 Core Staff。

## 分页、实时和恢复

- Core 管理集合、Edge 历史/通知均使用服务端 `page/page_size/total`、有界 page size 和稳定排序；筛选变化回到第 1 页，并取消旧请求。
- 员工 `GET /api/v1/tasks` 是按登录 Principal 返回的完整个人 Projection 快照，用于启动、刷新、重连和离线恢复，不是管理端全量列表。
- 管理 SSE `GET /api/v1/realtime/management` 面向 `admin/manager/leader/supervisor`，按 Scope 提供提示；员工 WebSocket/小程序 Socket 只提示本人 Projection 变化。
- Socket、缓存、localStorage 和小程序存储都不能作为可靠事实或离线队列；断线、乱序、重复和提示丢失时重新拉取 HTTP 快照。

## 当前未完成

- 真实个人微信/企业微信凭据、HTTPS 域名、管理 SSO 正式配置和平台告警/订阅消息。
- 正式员工/管理员主数据预置、两套小程序开发者工具编译预览和发布验收。
- 503、服务重启、网络分区、多副本 fan-out、容量和 p50/p95/p99 性能门禁。
- 最终品牌视觉、无障碍和真实设备体验评审。

## 已执行验证

- 前端 `typecheck`、`lint`、`test`（14/14）和 Web `build` 已通过；小程序打包检查已通过。
- 最新前端复核中，`typecheck`、`test`（5 个文件/16 个测试）和 `build:miniapps` 已通过；Web `build`、`lint` 结果保持通过。
- `admin-pagination.spec.ts` 管理集合分页 Contract 和 `admin-role-matrix.spec.ts` 角色矩阵浏览器验收各通过 1/1。
- 最新验证前均已执行 Docker preflight；本快照不把 Mock Provider、开发种子或单机 Socket 验收写成生产完成。
