# BVS2 业务切片 v2：外部航班 → 自动派发 → 员工执行

> 更新时间：2026-09-07
> 状态：当前业务基线
> 前置架构：Architecture v2.0 `ACCEPTED / FROZEN`

完整业务流程见 [`docs/business-process-v2.md`](../docs/business-process-v2.md)。本文件只保留 memory-bank 需要的冻结摘要，不再保留已经淘汰的“Leader Confirm → Employee Accept”历史草案。

## 冻结结论

1. 航班事实由外部 Provider 提供；管理端只能读取，不能自行新增或修改航班。开发种子航班只用于本地验收。
2. 外部 Schedule/Event 先快速、幂等地写入 Core `flight_source_inbox`，再由 Worker 异步应用到航班事实、状态历史、任务和 Outbox。
3. 有效到达事件按启用模板生成 `pending_dispatch` Task 和 Candidate 快照；`generation_key` 保证同一航班同一触发只生成一个任务。
4. 自动派发使用 v1 确定性规则：启用、区域/班组、主班组成员、精确岗位、精确能力、`idle` 且计划时间无冲突；排序为 `last_state_changed_at ASC`、public ID 升序。派发使用 Core Assignment 事务和人员预留，失败候选逐个失效，全部失败则保留缺员事实。
5. 三层握手分别是：Core 系统确认 Assignment、Outbox/Inbox/Projection 投影确认、员工 `received` 收件确认。员工 `received` 不是同意，员工没有拒绝任务的业务动作。
6. 员工 `start` 才进入执行中，`complete` 才完成；旧 `accept` 仅为映射到 `start` 的兼容入口。
7. 冲突、延误、取消、突发事件和改派通过持久化任务变更申请提出。动作限于 `pause`、`reassign`、`reschedule`、`cancel`、`resume`；员工/队长可上报，只有值班经理或管理员审批，应用失败保留为 `failed`。
8. 收件超时默认 300 秒。Worker 释放未收件 Assignment 后，按同一候选规则重新预分配；无候选时不静默取消任务。
9. 航班源状态为 `fresh`、`stale`、`fallback`、`failed`。源不可用时继续读取前一晚/最近一次已预同步的数据库事实，同时记录重试、失败和可告警健康状态。
10. Core 是航班、人员、岗位、能力、任务、Assignment、事件、规则、审计和状态机的唯一事实源；Edge 是员工 Projection/Command 边界。实时 Socket 只作刷新提示，HTTP 快照/分页查询负责恢复。

## 状态摘要

```text
Task:       pending_dispatch → assigned → in_progress → completed
                              └────────────────────────→ cancelled

Assignment: confirmed → accepted → completed
                         └──────────────→ cancelled

Personnel:  idle → reserved → busy → idle
```

历史数据可能含 `awaiting_confirmation`，它仅作为兼容读取状态，不是当前自动流程的正常入口。`completed` 和 `cancelled` 为终态。

## 角色摘要

| 角色 | 允许的业务动作 |
| --- | --- |
| `admin` | 全局管理、字典/人员/模板维护、任务变更审批、诊断 |
| `manager` | 授权范围内查看、补派/确认、任务变更审批 |
| `leader` | 授权区域/班组查看、运行问题上报和跟进；不能审批变更 |
| `supervisor` | 授权范围内只读查看和实时订阅 |
| `staff` | 查看本人任务、收件、开始、完成、异常上报；不能拒绝或直接修改任务 |

## 幂等与验收要点

- 外部记录使用 `provider + record_type + external_record_id`；任务生成使用 `generation_key`；员工动作使用稳定 `command_id`；变更审批使用请求 ID和当前 Task 版本。
- 相同消息重放只能返回原结果；不同内容复用同一 ID 必须返回冲突；乱序 Projection 按 `sync_version` 收敛。
- 验收覆盖源接口失效/fallback、重复消息、Worker 重启、断线恢复、延误、航班取消、候选冲突、收件超时重派、异常审批、员工无拒绝入口、权限越权、分页和实时订阅范围。
