# 航空保障协同平台当前业务流程基线 v2

> 更新时间：2026-09-07
> 状态：当前业务流程与验收口径
> 适用范围：Core、Worker、Edge、管理端 Web、员工 Web、个人微信小程序、企业微信小程序

本文是当前业务流程的统一说明。状态机、接口和验收文档必须与本文保持一致；阶段进度中的旧表述只属于历史记录，不作为当前实现依据。

## 1. 业务原则

1. 航班事实只来自外部航班源。管理端不能自行新增、修改、到达、离港或取消航班；本地数据库中的航班只能是外部同步后的事实或开发验收种子。
2. 航班源接入、任务生成、人员派发和员工执行是连续流程，但每一步都有持久化事实、版本和审计记录。
3. 员工不得拒绝已分配的保障任务。员工端的“确认”是收件回执，表示通知已收到，不是同意或征求意见；员工遇到冲突、延误、取消或突发事件时必须上报受控变更申请。
4. 任务、Assignment、Personnel 状态只能由 Core 状态机和事务用例改变。前端不能直接修改 Core 事实，实时连接也不能作为事实来源。
5. Core 是唯一事实源；Edge 只保存员工所需的 Projection、Command、Session、Inbox 和 delivery。可靠同步依靠 Outbox/Inbox、幂等、版本收敛和重试，Redis/WebSocket 只负责短期提示。

## 2. 主流程

```text
外部航班源同步
  → Core 持久化 flight_source_inbox
  → Worker 应用航班 Schedule/Event
  → 有效到达事件触发生成 pending_dispatch Task
  → 生成并保存 Candidate 快照
  → 自动调度器按规则预分配并完成 Core Assignment 确认
  → Core Outbox → Worker → Edge Inbox/Projection
  → 员工收到任务并提交 received 收件回执
  → 员工到岗后提交 start，任务进入执行中
  → 员工完成保障并提交 complete
  → Core 更新任务、Assignment、Personnel，Outbox 回传 Edge
```

每个箭头都不是内存中的隐式步骤：成功结果写入 Core 或 Edge 的持久化记录后才能进入下一步。HTTP `202` 只表示 Command 已被接收并持久化，不表示业务状态已经完成。

### 2.1 航班同步与数据新鲜度

外部适配器实现 `internal/integration/flight.Provider`，将上游 Schedule/Event 规范化后批量提交到内部同步入口。同步入口只做鉴权、格式校验、业务幂等接收和快速落库，不等待任务生成。

```text
Provider → flight_source_inbox → Worker lease/apply → flight + history + task/outbox
```

`flight_source_inbox` 的幂等键为 `provider + record_type + external_record_id`。同一外部记录重复到达不会重复产生事实；同一外部 ID 的规范化内容发生变化时重新进入 `pending`，由 Worker 按版本和重试规则应用。

航班源状态使用以下枚举：

| 状态 | 含义 | 系统行为 |
| --- | --- | --- |
| `fresh` | 最近一次同步在新鲜度窗口内成功 | 正常使用最新同步事实 |
| `stale` | 超过新鲜度窗口但仍有最近成功数据 | 标记数据陈旧，继续读取已落库事实并告警 |
| `fallback` | 当前源不可用，使用前一晚或最近一次预同步数据 | 继续用数据库事实生成/分配任务，并明确标记降级 |
| `failed` | 当前记录或应用尝试达到失败阈值 | 保留失败原因、重试时间和诊断信息，等待重试或人工处理 |

接口失败时不得回退到浏览器缓存，也不得临时允许管理端伪造航班。数据库中已经预同步的航班继续可读；Worker 负责指数退避、有限重试和失败记录。实时告警的发送渠道属于部署配置，当前代码提供健康状态和诊断出口，不把未配置的平台通知写成已完成。

航班事件的最小规则为：`scheduled → arrived` 触发一次任务生成，`scheduled → cancelled` 只记录航班取消事实，`arrived → departed` 只记录离港事实；`delayed` 必须带新的计划时间并更新外部航班事实。延误、取消或其他外部变化不会绕过任务变更审批，活动任务仍通过变更申请处理。

### 2.2 任务生成

只有有效的外部到达事件可以触发任务生成。Core 在一个事务中保存航班状态、状态历史、模板版本快照、Task、Candidate、业务幂等结果、Audit 和必要的 Outbox。

Task 的正常初始状态是 `pending_dispatch`。它表示任务事实已生成、候选快照已建立，正在等待或执行自动派发；不是等待员工同意。历史数据中的 `awaiting_confirmation` 仍可读，但只作为兼容状态，不是新流程的正常入口。

同一航班同一触发类型通过 `generation_key = flight_public_id + trigger_type` 保证只生成一个 Task。没有启用模板时保留航班到达事实并记录 `no_active_template`，不创建空任务；后续补生成必须使用独立的、可审计的管理用例。

### 2.3 自动预分配

自动调度器使用可替换、可重放的 v1 确定性规则：

硬过滤条件：

- 人员已启用，且属于任务对应的区域和班组；
- 是当前主班组成员；
- 岗位编码与模板要求完全一致；
- 能力编码与模板要求一致；当前产品语义为一人一个入职岗位和一个主能力；
- `work_state = idle`；
- 计划时间没有其他有效 Assignment 冲突。

排序条件：先按 `last_state_changed_at ASC`，再按候选人员 public ID 升序。候选快照必须保存岗位、能力、状态和排序名次，真正预留人员时还要在 Core 事务中重新校验当前事实。

自动派发按候选顺序调用与管理端确认共用的 Assignment 事务：成功后一个 Candidate 为 `selected`、其余候选为 `rejected`，Task 进入 `assigned`，人员进入 `reserved`，并写入 `task.assigned.v1`。某个候选在并发中失效时只使该候选 `invalidated`，继续尝试剩余候选；所有候选都不可用时任务保留为可见的待处理/缺员事实，不静默取消。

公平性、班次负载、休息时间、交接班和跨航班全局优化属于后续版本规则，必须以版本化 selector、回放测试和业务评审方式引入，不能隐藏在 Handler 中。

### 2.4 三层握手

三层确认解决的是“任务是否被正确分配、是否可靠同步、员工是否收到通知”三个不同问题：

| 层次 | 事实 | 责任主体 | 不代表什么 |
| --- | --- | --- | --- |
| 系统确认 | Core 完成候选复核、Assignment 创建和人员预留 | 自动调度器或有权限的管理人员 | 不代表 Edge 已收到 |
| 投影确认 | Outbox 已投递，Edge Inbox 去重并提交 Projection | Worker/Edge | 不代表员工已经看到 |
| 员工业务确认 | 员工提交 `received` 收件回执 | 被分配员工 | 不代表员工同意，也不能拒绝任务 |

员工端动作如下：

| 动作 | 作用 | 允许结果 |
| --- | --- | --- |
| `received` | 确认通知已经收到；推进收件状态和同步版本 | `received` |
| `start` | 员工到岗并开始执行 | Task `assigned → in_progress`，Assignment `confirmed → accepted`，Personnel `reserved → busy` |
| `complete` | 员工完成保障 | Task/Assignment 完成，Personnel `busy → idle` |
| 异常上报 | 报告冲突、延误、取消、突发事件或执行障碍 | 只创建待审批的变更申请，不直接改变任务 |

旧 `accept` 路由只作为兼容入口映射到 `start`，不再解释为员工同意。员工未收到回执时，Worker 默认在 300 秒后扫描并重新预分配；旧 Assignment 先释放，再按同一候选规则尝试剩余候选。没有候选时保留缺员事实并通知管理侧处理。

### 2.5 任务变更申请与审批

延误、取消、突发事件、员工正在保障其他航班或需要重新预分配时，员工、队长或授权外部指令只能提出变更申请。申请保存请求人、任务版本、原因、来源、目标时间/候选人（如有）和审计关联。

受控动作只有：`pause`、`reassign`、`reschedule`、`cancel`、`resume`。员工提交的是请求，不是执行结果；`reschedule` 必须填写目标计划时间。队长可以上报和查看，但不能批准；值班经理或管理员负责审批。审批时重新校验当前 Task 版本、状态、人员资格和时间冲突，批准后的任务、Assignment、人员释放/预留、历史、Audit 和 Outbox 在一个 Core 事务内完成。

审批结果为 `pending`、`approved`、`rejected` 或 `failed`。版本冲突、人员已被占用等应用失败不能伪装成成功，必须保留 `failed` 申请供管理端继续处理。已完成任务不能重新激活；取消不是员工拒绝，而是授权的运行变更。

### 2.6 任务和人员状态机

```text
Task:       pending_dispatch → assigned → in_progress → completed
                 └──────────────────────────────→ cancelled

Assignment: assigned/confirmed → accepted → completed
                 └──────────────────────────────→ cancelled

Personnel:  idle → reserved → busy → idle
            idle ↔ unavailable（独立人员状态用例）
```

当前代码兼容读取 `awaiting_confirmation`，但新任务不以它作为自动流程入口。`completed`、`cancelled` 是终态；任务取消时按当前状态释放 Candidate/Assignment/Personnel，迟到的员工 Command 返回稳定业务错误，不重试、不产生新业务副作用。

## 3. 角色与实时同步

| 角色 | 当前职责 |
| --- | --- |
| 管理员 `admin` | 全局管理、模板/字典/人员等主数据维护、受控任务变更审批和系统诊断 |
| 值班经理 `manager` | 其授权范围内查看运行数据、补派/确认和审批任务变更 |
| 队长 `leader` | 其区域/班组范围内查看任务、人员和状态，提出运行变更；不能审批任务变更 |
| 分管领导 `supervisor` | 按授权范围只读查看管理订阅和运行态势 |
| 员工 `staff` | 查看本人 Projection、提交收件回执/开始/完成/异常上报；不能拒绝或直接修改任务 |

管理端实时订阅 `GET /api/v1/realtime/management` 只向 `admin`、`manager`、`leader`、`supervisor` 按 Scope 推送提示；员工不接收管理订阅。员工 WebSocket/小程序原生 Socket 只发送任务变化提示。所有客户端断线、乱序、丢提示或 Redis 不可用时，都通过分页 HTTP/员工完整任务快照恢复；实时连接不是可靠队列，也不能伪造同步成功。

管理集合、历史、异常、事件、审计和字典使用服务端 `page/page_size/total` 分页。员工任务读取保留本人完整 Edge Projection 快照，因为它是重连恢复合同，不是一次性返回全部管理数据。

## 4. 当前实现与验收边界

已在当前代码/契约中落地的流程能力包括：外部航班源适配边界与源 inbox、预同步数据和源健康状态、`pending_dispatch` 自动派发、三层握手、收件超时重新分配、任务变更申请/经理审批、岗位/能力独立字典 CRUD、服务端分页、管理角色 Scope 和实时订阅范围。

仍不能写成生产验收完成的内容包括：真实 AODB/航司 Provider、真实微信/企业微信凭据与域名、平台告警/订阅消息、多副本和网络分区故障门禁、代表性性能基线，以及正式员工/管理员主数据预置。开发种子数据只用于本地验证。

正式验收至少覆盖：源接口失效与 fallback、重复/内容冲突消息、Worker 重启和断线恢复、延误、航班取消、候选并发冲突、收件超时重派、员工异常上报与经理审批、员工不具备拒绝入口、Scope 越权、分页空结果和 Projection 版本乱序。
