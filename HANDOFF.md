# 项目交接总览

> 更新时间：2026-09-07
> 当前分支：`agent/foundation-and-handoff`
> 当前工作区：本地工作区已清洁；当前分支 `HEAD=3690659`，相对 `origin/agent/foundation-and-handoff` 超前 2 个交接状态提交。推送因本机 GitHub 凭据失效未完成，未执行强推、重置或覆盖远端。

## 当前唯一业务口径

详细流程见 [`docs/business-process-v2.md`](docs/business-process-v2.md)。主流程是：

```text
外部航班源 -> source inbox -> Worker 应用航班事实
  -> 航班到达 -> pending_dispatch
  -> 自动候选筛选/预分配 -> Core Assignment
  -> Edge Projection -> 员工 received -> start -> complete
```

`received` 只代表员工已收到通知，不是同意；员工没有拒绝任务。员工正在保障其他航班或遇到延误、取消、突发情况时，提交异常/任务变更申请；只有 `manager/admin` 审批并应用，`leader` 报告跟进，`supervisor` 只读观察。

## 当前架构

- Core 是航班、人员、岗位、能力、任务、Assignment、事件、规则、审计和状态机的唯一事实源。
- Edge 只保存员工端 Projection、Command、Session、Inbox、通知和 delivery；Core/Edge 使用独立 MySQL 和 migration。
- Worker 负责航班源 inbox 应用、Core Outbox → Edge Inbox 投递、Edge Command 拉取和收件超时重派。
- 航班只允许来自外部 Provider；管理端航班页只读，开发种子中的航班仅为测试夹具。
- 航班源状态为 `fresh/stale/fallback/failed`；Provider 失败时继续读取前一晚/最近一次预同步事实，并保留重试和诊断。
- 管理端按 Scope 使用 SSE 提示；员工 WebSocket/小程序原生 socket 只发 best-effort 提示，断线后从分页/完整快照恢复。

## 已落地的业务能力

- 自动 `pending_dispatch` 任务生成、候选快照、冻结的 v1 调度规则、候选冲突回退和无候选短缺可见。
- 系统确认、Edge 投影确认、员工收件回执三层握手；员工 `received/start/complete` Command 和稳定幂等 ID。
- 默认 300 秒收件超时重派；没有替补不静默取消。
- 任务变更申请 `pause/reassign/reschedule/cancel/resume`、经理/管理员审批、版本复核、事务性 History/Audit/Outbox。
- 独立岗位/能力字典 CRUD、安全停用、单岗位/单能力人员约束、工号数字格式校验。
- 管理端角色/Scope、管理实时订阅、Core 管理集合与 Edge 历史/通知分页。
- Admin Web、Employee Web、个人微信小程序、企业微信小程序的当前页面和 API 适配。

## 当前未完成的正式验收

1. 根据真实 AODB/航司资料完成 Provider 字段映射、凭据注入、定时同步、告警渠道和对账。
2. 在隔离环境应用待部署 migration，完成接口失效/fallback、重复/乱序消息、断线/重启、延误、取消、无候选、收件超时重派和变更审批失败验收。
3. 完成真实个人微信/企业微信凭据、HTTPS/回调域名、管理端 SSO、管理员预置、员工导入和平台通知配置。
4. 完成 Gateway/多副本/网络分区/Redis 故障，以及 API/DB/Worker/Projection 的 p50/p95/p99 性能基线。
5. 完成四客户端正式设备、浏览器、权限矩阵、无障碍和视觉收口验收。

## 重要文档入口

- 架构：[`docs/architecture/architecture-v2.md`](docs/architecture/architecture-v2.md)
- 代码导览：[`docs/code-tour-v2.md`](docs/code-tour-v2.md)
- 前后端交接：[`docs/frontend-backend-handoff.md`](docs/frontend-backend-handoff.md)
- 任务合同：[`docs/task-crud-contract-v2.md`](docs/task-crud-contract-v2.md)
- 前端冻结门槛：[`frontend/docs/architecture-freeze-gate.md`](frontend/docs/architecture-freeze-gate.md)
- 当前进度：[`memory-bank/progress.md`](memory-bank/progress.md)
- 后端交接：[`memory-bank/handoffs/backend.md`](memory-bank/handoffs/backend.md)
- 前端交接：[`memory-bank/handoffs/frontend.md`](memory-bank/handoffs/frontend.md)
- 基础设施交接：[`memory-bank/handoffs/infrastructure.md`](memory-bank/handoffs/infrastructure.md)

## 验证与安全

涉及功能、数据库或 Compose 验证前必须先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

统一验证入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

文档修订阶段没有应用 migration、清空/删除数据库或停止已有服务；最近一次代码验证结果以 `memory-bank/progress.md` 最新条目为准。当前 `origin/agent/foundation-and-handoff` 仍为 `f2a8d1e`，本地 `a5be54b`、`3690659` 两个交接状态提交尚未推送。最后一次推送因 GitHub 凭据管理失败（默认 `gh` token 无效、Git 无法读取用户名）而中止；重新认证后可直接重试普通 push。
