# BVS2-01 业务设计冻结：Flight → Task → Personnel → Leader Confirm → Edge → Employee → Complete

> 阶段：Phase 2 — Business Implementation
> 状态：FROZEN（2026-08-28）
> 前置基线：Architecture v2.0 Foundation ACCEPTED / FROZEN
> BVS2-03：COMPLETED（Flight → Task → Candidate）
> 下一步：BVS2-04 Leader Confirm

本文是 Architecture Foundation 之后第一个真实业务切片的唯一业务基线。旧 B3 业务切片材料、旧 B3 代码和旧业务路由继续保持 legacy/paused，不作为本切片实现入口。

本文件在 BVS2-01 阶段冻结业务规则、状态机、权限、Core/Edge 契约、异常补偿和可执行验收场景；BVS2-03 已按本冻结基线落地 Core Flight → Task → Candidate 实现，后续切片继续受本文件约束。

## 1. 冻结结论

### 1.1 领域所有权

1. **Flight 只提供业务事实和触发条件，不拥有 Task 生命周期。** Flight 状态变化触发 Core Application Use Case；Task 创建后由 Task 自己管理状态。
2. **Candidate 与 Assignment 永远分离。** Candidate 表示系统认为“可选”；Assignment 表示经过 Leader/Manager 确认后的正式分配。
3. **Personnel 状态只能由 Personnel Application/Domain 改变。** Task Application 通过 Personnel Port 请求预留、占用和释放，不直接更新 Personnel 表。
4. **员工 Accept 和 Complete 都是 Edge → Core Command。** Edge 不把 Projection 状态改成最终业务状态，最终状态必须由 Core 确认后再通过 Outbox 回传。
5. **同一 command_id 重放返回第一次结果且不重复执行副作用；不同 command_id 但业务目标已经完成时返回稳定的幂等业务结果，也不重复执行副作用。**
6. **任务取消和人员状态变化先校验、后变更。** 取消、释放 Reservation、更新状态历史、Audit 和 Outbox 必须在 Core 同一事务内完成。
7. **Core 业务事务不依赖 Edge 可用性。** Edge 故障只影响投影和命令送达，不回滚已经提交的 Core 事实。

### 1.2 切片范围

本切片实现一个航班触发的单任务确认闭环：

~~~text
Flight scheduled → arrived
        ↓
Core 判断触发条件并生成 Task Instance
        ↓
生成 Candidate
        ↓
Leader/Manager 在 Scope 内确认一个 Candidate
        ↓
Core 创建正式 Assignment，并预留 Personnel
        ↓
Core Outbox → Edge Task Projection
        ↓
Employee 查看并 Accept
        ↓
Edge Command → Core 校验 → Task in_progress / Personnel busy
        ↓
Employee Complete
        ↓
Edge Command → Core 校验 → Task completed / Personnel idle
        ↓
Core Outbox → Edge Projection 更新
~~~

第一版支持每个触发事件按匹配的有效 Task Template 生成一个 Task Instance；验收夹具使用一个有效模板和一个目标任务。第一版不实现自动调度、多人抢单、自动替代确认、复杂模板编排或跨航班调度。

## 2. 业务规则

### 2.1 Flight 触发规则

#### 2.1.1 Flight 状态和触发条件

本切片只使用以下 Flight 状态：

~~~text
scheduled → arrived → departed
scheduled → cancelled
~~~

- 只有 scheduled → arrived 的有效状态变化触发 Task Generation。
- arrived → arrived 是重复通知，返回 already_processed，不得重复生成 Task。
- scheduled → cancelled 只记录 Flight 状态历史；本切片中该状态不会生成 Task，也不通过 Flight 反向拥有 Task 生命周期。已生成 Task 必须走 Core 授权的 Task 取消用例。
- arrived → departed 不生成新任务。
- cancelled/departed → arrived 是非法逆向状态变化，返回 flight_status_conflict，不生成任务。
- Flight 状态、Flight 状态历史和触发输入均由 Core 保存；Edge 不保存完整 Flight 事实。

#### 2.1.2 到达输入

到达输入至少包含：

~~~text
source_event_id
flight_public_id
occurred_at
actual_arrival_at
source
~~~

source_event_id 是外部来源或人工录入的一次业务事件 ID；它不是传输层 event_id 的替代品。没有外部来源 ID 的测试输入必须由 Core 生成稳定的业务幂等键。

Core 接收输入后，在一个事务内完成：

1. 校验 Flight 当前状态和输入顺序；
2. 写入 Flight 当前状态和 flight_status_history；
3. 按触发条件解析有效模板；
4. 创建 Task Instance 和 Candidate；
5. 写入必要的 Audit 和 Core-local 业务事件/Outbox。

Edge 不参与 Flight 到达判断，也不能通过 Edge 直接创建 Task。

#### 2.1.3 是否生成任务

满足以下全部条件才生成 Task：

- Flight 状态从 scheduled 合法进入 arrived；
- 到达输入的业务幂等键尚未处理；
- 存在启用状态的 flight_arrived Task Template；
- 模板版本在触发时被固定并写入 Task Instance；
- 相同的 generation_key 尚未成功生成 Task。

generation_key 固定为：

~~~text
flight_public_id + trigger_type
~~~

本切片规定同一 Flight 的同一 trigger_type 只允许生成一个 Task。template_public_id 和 template_version 作为触发时快照写入 Task，但不再进入幂等键；这样即使后续模板版本变化，也不会为同一次到达事实追加第二个 Task。相同 source_event_id 但不同传输 event_id，或相同 generation_key 的重放，都只能返回既有结果，不能创建第二个 Task。

没有有效模板时，Flight 到达事实仍然可以成功保存；Task Generation 结果为 no_active_template，写入 Audit/业务结果，但不创建空 Task。该 trigger 的结果视为已处理，不因后续模板上线而自动补生成；如未来需要补生成，必须新增明确的管理用例和幂等协议。该结果不由 Edge 展示。

### 2.2 Task 生成规则

Task 是独立的业务事实，Task 生命周期不由 Flight 状态自动驱动。Task 至少保存：

- task_public_id；
- flight_public_id；
- template_public_id 和 template_version；
- trigger_type 和 generation_key；
- team_id/area_id；
- planned_at；
- 当前状态和状态版本；
- 取消原因（如有）。

生成后 Task 的第一个持久化业务状态为 awaiting_confirmation。pending 不作为本切片的持久化 Task 状态；模板解析和 Candidate 生成必须在同一 Core 事务内完成，不能留下一个没有候选计算结果的半成品 Task。

### 2.3 Candidate 规则

Candidate 只是推荐记录，不是 Assignment，也不能被 Edge 当作可执行任务。

Candidate 必须同时满足：

- Personnel 属于 Task 要求的 Team；
- Personnel 所属 Area 与 Task Scope 一致；
- 具备模板要求的 Position；
- 具备模板要求的全部 Capability；
- Personnel 当前 work_state = idle；
- 没有不可用标记；
- 在 Task 的计划时间范围内没有冲突的有效 Assignment。

候选排序固定为确定性排序：

1. last_state_changed_at 较早者优先；
2. 同一时间按 personnel_public_id 升序。

Candidate 记录必须保留生成时的岗位/能力/状态快照和排序名次，但 Leader 确认时必须重新读取 Core 当前事实并重新校验，不能只相信快照。

Candidate 状态：

~~~text
proposed → selected
proposed → rejected
proposed → invalidated
~~~

- selected 只在正式 Assignment 创建成功的同一事务中写入；Task 确认后其他候选不再保持 proposed；
- rejected 表示 Leader 明确不选择该候选；
- invalidated 表示状态、能力、班组、时间冲突或 Task 取消使候选失效；
- Task 在 awaiting_confirmation 时取消，所有 proposed Candidate 置为 invalidated；已 selected 的 Candidate 是历史选择结果，随 Assignment 保持 selected，不通过 Candidate 状态伪造取消；
- Candidate 不能自行变成 accepted 或 completed。

没有合格 Candidate 时，Task 仍保持 awaiting_confirmation，候选集合为空并记录 candidate_shortage 结果；本切片不自动替代人员、不自动转派。

### 2.4 Leader/Manager 确认规则

确认操作必须包含：

~~~text
confirmation_id
task_public_id
candidate_public_id
expected_task_version
~~~

confirmation_id 是 Leader/Manager 操作的业务幂等键；HTTP 层也可通过 Idempotency-Key 提供，Core 必须持久化并记录第一次结果。

Core 在确认事务中按以下顺序执行：

1. 校验 Actor 是 Human Principal；
2. 校验 task:assign 权限和 Task 的 Area/Team Scope；
3. 锁定或以乐观版本校验 Task；
4. 重新校验 Candidate 当前仍可用；
5. 通过 Personnel Port 请求 idle → reserved；
6. 创建状态为 confirmed 的 Assignment；
7. 将确认的 Candidate 置为 selected，其余 proposed Candidate 在同一事务中置为 rejected（reason=not_selected）；
8. 将 Task 从 awaiting_confirmation 置为 assigned；
9. 写 Task/Assignment/Personnel 状态历史、Audit 和 task.assigned.v1 Outbox Event；
10. 同一事务提交。

以上任一步失败，整个业务变更回滚，不产生部分 Assignment、不预留 Personnel、不写成功 Outbox。Edge 不可用不属于该事务失败条件。

并发确认时只有一个事务可以成功；其他请求返回 task_already_assigned 或 candidate_no_longer_eligible，不得创建第二个有效 Assignment。

Task 取消是独立的 Core 管理端用例，不由 Flight 状态反向驱动，也不经过 Edge 员工 Command。取消操作必须包含：

~~~text
cancellation_id
task_public_id
expected_task_version
reason
~~~

- Manager/Admin 可以在自身 Scope 内取消任意非终态 Task；Leader 只能在自身 Area/Team Scope 内取消尚未被员工 Accept 的 awaiting_confirmation/assigned Task；Staff 无取消权限；
- Core 在同一事务中校验 Actor、Scope、Task 版本和当前状态；awaiting_confirmation 时使所有 proposed Candidate 失效，assigned/in_progress 时释放对应 Assignment 和 Personnel 占用；
- 同一事务写入 Task/Assignment/Personnel 状态历史、Audit 和 task.cancelled.v1 Outbox Event；Edge 不可用不影响该事务；
- 同一 cancellation_id 重放返回第一次结果；内容不同返回 cancellation_id_conflict；completed/cancelled Task 不产生新的业务副作用。

### 2.5 Personnel 状态规则

本切片的 Personnel work_state 固定为：

~~~text
idle → reserved → busy → idle
~~~

取消和不可用规则见第 7 节。

- idle → reserved：Leader/Manager 确认 Assignment 时由 Personnel Application 执行；
- reserved → busy：员工 Accept 被 Core 成功处理时由 Personnel Application 执行；
- busy → idle：员工 Complete 或有效任务取消被 Core 成功处理时由 Personnel Application 执行；
- Task 模块不得直接 UPDATE personnel；必须调用 Personnel Port/Application；
- Personnel 的状态版本和状态历史由 Personnel 自己维护；Task 只消费结果并编排事务；
- reserved 或 busy 状态下，普通人工不可直接改为 unavailable；必须先经过取消/释放流程；
- idle ↔ unavailable 仅由明确的 Personnel Application 用例修改，本切片不实现设备自动上报。

### 2.6 Employee Accept 规则

员工 Accept 必须以 employee_accept_task.v1 Command 进入 Edge Command Store，再由 Core Worker 拉取。

Core 只在以下条件全部满足时应用：

- Actor 是该 Assignment 的员工本人；
- Actor 通过 task:accept 权限和 self/assigned Scope 校验；
- Task 状态为 assigned；
- Assignment 状态为 confirmed；
- Personnel 状态为 reserved；
- Command 的 Assignment/Task 关系和预期版本有效。

成功时在同一 Core 事务中：

- Assignment confirmed → accepted；
- Task assigned → in_progress；
- Personnel reserved → busy；
- 写三类状态历史、Audit 和 task.accepted.v1 Outbox Event。

Accept 不是员工直接修改 Projection 的操作。Edge 在 Core 确认前只能显示 pending/syncing。

### 2.7 Employee Complete 规则

员工 Complete 必须以 employee_complete_task.v1 Command 进入 Edge Command Store，再由 Core Worker 拉取。

Core 只在以下条件全部满足时应用：

- Actor 是该 Assignment 的员工本人；
- Actor 通过 task:complete 权限和 self/assigned Scope 校验；
- Task 状态为 in_progress；
- Assignment 状态为 accepted；
- Personnel 状态为 busy；
- Command 的 Assignment/Task 关系和预期版本有效。

成功时在同一 Core 事务中：

- Assignment accepted → completed；
- Task in_progress → completed；
- Personnel busy → idle；
- 写三类状态历史、Audit 和 task.completed.v1 Outbox Event。

员工不能跳过 Accept 直接 Complete；管理员或 Manager 也不能用普通员工 Command 冒充员工完成任务。

## 3. 状态机冻结

### 3.1 Flight

| 当前状态 | 事件/动作 | 下一状态 | 结果 |
|---|---|---|---|
| scheduled | 有效 flight.arrived.v1 | arrived | 触发一次 Task Generation |
| scheduled | flight.cancelled.v1 | cancelled | 仅记录 Flight 取消事实；本切片不生成 Task |
| arrived | 重复到达输入 | arrived | already_processed，无副作用 |
| arrived | 航班离港 | departed | 不生成任务 |
| cancelled/departed | 到达输入 | 不变 | flight_status_conflict |

Flight 不因 Task completed 而改变状态；Task 也不反向拥有 Flight 状态。

### 3.2 Task Instance

~~~text
awaiting_confirmation → assigned → in_progress → completed
        │                    │           │
        └────────────────────┴───────────┴→ cancelled
~~~

允许迁移：

- awaiting_confirmation → assigned：确认成功；
- assigned → in_progress：员工 Accept 成功；
- in_progress → completed：员工 Complete 成功；
- awaiting_confirmation/assigned → cancelled：Core 授权的 Task 取消；
- in_progress → cancelled：仅授权的显式取消，不能由普通 Flight 取消自动完成；
- completed/cancelled 为终态，不允许反向迁移或重新激活。

### 3.3 Candidate

~~~text
proposed → selected
proposed → rejected
proposed → invalidated
~~~

Candidate 是 Task 下的推荐历史；它不能代替 Assignment，也不接收员工 Command。

### 3.4 Assignment

Assignment 只在 Leader/Manager 确认成功后创建，初始状态直接为 confirmed，不设置 proposed Assignment 状态。

~~~text
confirmed → accepted → completed
     │          │
     └──────────┴→ cancelled
~~~

- confirmed → accepted 只能由被分配员工的 Accept Command 触发；
- accepted → completed 只能由被分配员工的 Complete Command 触发；
- confirmed/accepted → cancelled 只能由 Core 授权的 Task 取消触发；
- 每个 Task 最多一个有效 Assignment；
- completed/cancelled 为终态。

### 3.5 Personnel

~~~text
idle → reserved → busy → idle
idle → unavailable → idle
~~~

严格规则：

- reserved/busy → unavailable 不是普通直接迁移；必须先完成或取消活动 Assignment；
- reserved → idle 只用于未 Accept 前的取消/释放；
- busy → idle 只用于 Complete 或有效取消；
- 不允许 idle → busy、reserved → completed 或任何由 Task 直接修改 Personnel 的迁移。

### 3.6 Edge Task Projection

Projection 维护两个不同概念：

1. business_status：只镜像 Core 已确认的 assigned、in_progress、completed、cancelled；
2. sync_state：absent → applying → confirmed，失败时为 failed，恢复后重新 applying。

规则：

- task.assigned.v1 首次创建最小 Projection；确认前不存在可执行 Projection；
- task.accepted.v1、task.completed.v1、task.cancelled.v1 都携带完整的当前最小 Projection 快照；
- Edge 只接受 sync_version 大于当前值的事件；相等且内容相同是幂等重放；相等但内容不同是 projection_version_conflict；较小版本直接忽略为 stale；
- Projection 不是业务事实来源，WebSocket/Push 也不能改变上述规则；
- 如果取消事件先于较早的分配事件到达，Edge 可以创建最终 cancelled Projection，但不会显示可执行状态；随后较小版本事件必须被忽略。

### 3.7 Edge Command

业务可见状态和 Foundation 技术状态映射如下：

| 业务可见状态 | Foundation 持久化状态 | 含义 |
|---|---|---|
| pending | pending | Edge 已持久化，尚未被 Worker 处理 |
| syncing | processing/retry | Worker 正在发送或等待退避重试 |
| confirmed | sent/applied | Core 已返回成功结果，业务事实已确认 |
| failed | failed/rejected | 终止失败或业务拒绝，保留原因和结果码 |

技术迁移：

~~~text
pending → processing → sent/applied
             │
             ├→ retry → processing
             └→ rejected/failed
~~~

- 网络超时、Edge/Core 暂时不可用、HTTP 5xx、可重试数据库错误进入 retry；
- 权限拒绝、状态非法、任务取消、命令 Envelope 非法进入终止 rejected/failed，不自动重试；
- Worker 重启后可重新领取过期 processing；
- 同一 command_id 的最终结果必须持久化，重放返回原结果。

## 4. 权限和 Scope 冻结

### 4.1 Permission

本切片使用既有 resource:action 格式，细化为：

~~~text
flight:read
task:read
task:candidate:read
task:assign
task:accept
task:complete
task:cancel
personnel:read
~~~

已有 Foundation RBAC 的粗粒度权限不是业务最终授权结果；所有业务写操作还必须经过对象状态、Actor 身份和 Scope 校验。

### 4.2 Role 矩阵

| Role | 可查看 | 可修改 | Scope 与额外限制 |
|---|---|---|---|
| admin | 全部切片数据 | 确认、取消、管理和受控应急操作 | global；不能伪造员工本人 Accept/Complete |
| manager | 其配置运行范围内 Flight/Task/Personnel | 确认分配、取消任务 | global/area/team 按配置；默认不自动拥有全局 |
| leader | 其负责 Area/Team 的 Flight、Task、Candidate、Personnel 摘要 | 确认/拒绝 Candidate；Accept 前取消任务 | 必须通过 area/team Scope；不能操作其他 Team |
| staff | 自己的 Edge Projection 和相关任务摘要 | 仅 Accept/Complete 自己的有效 Assignment | assigned/self；不能查看候选全表、确认、取消或改人员 |

manager 即使基础角色拥有 task:complete，也不能用员工 Complete Command 完成非本人 Assignment；是否提供独立的管理代办完成用例不在本切片范围。

### 4.3 Scope 落地

- global：仅由明确授予 Global Scope 的 Admin/受控主体使用；
- area：通过结构化 AreaIDs 过滤；
- team：通过结构化 TeamIDs 过滤；
- assigned/self：将 UserID 绑定到认证 Principal，不接受请求体中的任意员工 ID；
- Repository 接收结构化 AccessScope 并使用参数化条件；禁止拼接任意 SQL 的 Scope Resolver；
- Leader 确认时同时检查 Task Team/Area 与 Actor Scope；Staff Command 同时检查 actor_public_id、Assignment Personnel 和 UserID。

Machine Principal 只能用于既有 sync:* / device:* 机器权限，不能充当 Leader 或 Staff 执行本切片业务动作。

## 5. Core / Edge 契约冻结

### 5.1 通用 Envelope

所有跨 Core/Edge 消息继续使用既有 Envelope，字段不可省略。

Event：

~~~text
event_id
event_type
schema_version
aggregate_type
aggregate_id
occurred_at
producer
correlation_id
trace_id
payload
~~~

Command：

~~~text
command_id
command_type
schema_version
actor_public_id
aggregate_id
occurred_at
trace_id
payload
~~~

所有类型必须带 .v1；不兼容字段变化必须使用 .v2，不得复用 .v1。

### 5.2 Core-local Flight 输入

flight.arrived.v1 是 Core Application 的业务输入/内部领域事件，不向 Edge 同步完整 Flight 数据。

Payload：

~~~json
{
  "source_event_id": "source-event-public-id",
  "flight_public_id": "flight-public-id",
  "occurred_at": "2026-08-13T08:00:00Z",
  "actual_arrival_at": "2026-08-13T08:00:00Z",
  "source": "manual"
}
~~~

Core 以 source_event_id 和 generation_key 做业务幂等，以 Envelope event_id 做传输幂等。同一 source_event_id 再次到达且规范化 Payload 相同，返回 already_processed；同一 source_event_id 对应不同规范化 Payload，返回 source_event_id_conflict，不修改 Flight、Task、Candidate、Audit 或 Outbox。

### 5.3 Core → Edge Events

本切片只定义以下四类跨边界业务 Event：

| Event | 触发 | Aggregate | Edge 作用 |
|---|---|---|---|
| task.assigned.v1 | Assignment 确认成功 | task | 创建员工最小 Task Projection，business_status=assigned |
| task.accepted.v1 | Accept Command 被 Core 应用 | task | 更新 Projection 为 in_progress |
| task.completed.v1 | Complete Command 被 Core 应用 | task | 更新 Projection 为 completed |
| task.cancelled.v1 | Core 授权 Task 取消成功 | task | 创建/更新 Projection 为 cancelled，禁止执行 |

四类 Event 的 Payload 都必须包含同一份最小投影快照：

~~~json
{
  "task_public_id": "task-public-id",
  "assignment_public_id": "assignment-public-id",
  "employee_public_id": "employee-public-id",
  "flight_display_no": "CA1234",
  "task_name": "到达保障",
  "area_name": "到达区",
  "planned_at": "2026-08-13T08:15:00Z",
  "business_status": "assigned",
  "message": "请前往到达区执行保障任务",
  "sync_version": 1
}
~~~

字段约束：

- flight_display_no 只作为员工显示摘要，不同步完整航班、机型、机位、旅客或事件敏感明细；
- employee_public_id 只用于 Edge 对当前登录员工过滤，不同步完整人员档案；
- sync_version 按 Task Aggregate 单调递增，从 task.assigned.v1 开始；
- correlation_id 对确认事件关联 confirmation_id，对员工 Command 产生的事件关联 command_id；
- Event 写入必须与对应 Core 业务事实、状态历史和 Audit 在同一事务内完成。

本切片不向 Edge 发布 flight.arrived.v1、完整 Personnel、Candidate 列表、规则命中或完整 Event 记录。

### 5.4 Edge → Core Commands

本切片只定义以下两类员工 Command：

| Command | Actor | 前置状态 | 成功结果 |
|---|---|---|---|
| employee_accept_task.v1 | 被分配员工 | Task assigned + Assignment confirmed + Personnel reserved | Task in_progress + Assignment accepted + Personnel busy |
| employee_complete_task.v1 | 被分配员工 | Task in_progress + Assignment accepted + Personnel busy | Task completed + Assignment completed + Personnel idle |

Command Payload：

~~~json
{
  "assignment_public_id": "assignment-public-id",
  "expected_sync_version": 1,
  "client_occurred_at": "2026-08-13T08:20:00Z",
  "note": "optional employee note"
}
~~~

- aggregate_id 固定为 task_public_id；
- actor_public_id 只能从 Edge 认证 Principal 生成，不能信任请求体；
- command_id 必须是客户端/Edge 请求的幂等键，Edge API 重试不能生成新的业务意图；
- expected_sync_version 用于检测陈旧 Projection；Core 必须重新读取事实后返回稳定结果，不能盲目覆盖；
- client_occurred_at 只作客户端行为参考，Core 状态历史使用 Core 接收/处理时间为权威时间；
- Edge API 先持久化 Command，再返回 202 Accepted 和 pending，不等待 Core 业务完成。

Leader/Manager 确认和取消使用 Core 管理端 Application API，不经过 Edge Command；其业务幂等分别使用 confirmation_id 和 cancellation_id。

### 5.5 Command 结果语义

Core 必须把第一次处理结果与 command_id 一起持久化：

| 结果码 | 类别 | 是否重复副作用 |
|---|---|---|
| applied | 成功 | 否，第一次执行一次 |
| already_accepted | 幂等业务成功 | 否 |
| already_completed | 幂等业务成功 | 否 |
| task_cancelled | 终止业务拒绝 | 否，不自动重试 |
| not_assigned_to_actor | 权限/业务拒绝 | 否，不自动重试 |
| invalid_state | 状态拒绝 | 否，不自动重试 |
| stale_assignment | 陈旧目标 | 否，不自动重试 |
| command_id_conflict | 同 ID 不同内容 | 否，记录安全/一致性失败 |
| source_event_id_conflict | 同来源 ID 不同内容 | 否，不改变业务事实 |
| confirmation_id_conflict | 确认 ID 不同内容 | 否，不改变 Assignment/Personnel |
| cancellation_id_conflict | 取消 ID 不同内容 | 否，不改变 Task/Assignment/Personnel |

相同 command_id 重放必须返回第一次的 result_code、最终状态和错误信息；不同 command_id 在 Task 已 completed 时返回 already_completed，视为无副作用的业务幂等成功，不重复写状态历史、Audit 或 Outbox。

## 6. 幂等键冻结

| 层次 | 唯一键 | 重放行为 |
|---|---|---|
| Event 传输 | event_id | Edge Inbox 只应用一次；同 ID 不同 Payload 为冲突失败 |
| Flight 业务输入 | source_event_id | 返回 already_processed，不重复改变 Flight/Task |
| Task 生成 | generation_key | 返回原 Task，不重复生成 |
| Leader 确认 | confirmation_id | 返回第一次确认结果，不重复 Assignment/Reservation |
| Task 取消 | cancellation_id | 返回第一次取消结果，不重复释放/历史/Outbox |
| Edge Command | command_id | 返回原 Command 结果，不重复状态副作用 |
| Edge Projection | (task_public_id, sync_version) | 新版本覆盖；旧版本忽略；同版本冲突失败 |
| 业务目标 | task + assignment + actor + operation | 已完成/已接受时返回稳定 already_* 结果 |

同一个 event_id 或 command_id 如果再次携带不同 Payload，不能按幂等成功处理，必须记录 *_id_conflict，不执行第二份内容。source_event_id 也遵循同一原则：它是业务输入幂等键，不是传输 event_id；同一 source_event_id 的内容冲突必须单独记录 source_event_id_conflict。

## 7. 异常与补偿规则

### 7.1 Edge 暂不可用

- Core 业务事务、状态历史、Audit 和 Outbox 正常提交；
- Outbox 状态为 pending/retry，记录 attempts、next_attempt_at 和 last_error；
- Edge 仍可查看已有 Projection，新增任务暂时不可见；
- Edge 恢复后 Worker 补发，Inbox 按 event_id 幂等，Projection 按 sync_version 收敛；
- 禁止把 Edge HTTP 失败变成 Core 事务回滚。

### 7.2 Worker 中途退出

- 已提交的 Outbox/Command 不依赖 Worker 内存；
- processing 超过租约后可被新 Worker 重新领取；
- 重启后的重复发送由 Inbox/Command ID 幂等保护；
- 不允许通过 Redis 作为唯一可靠队列。

### 7.3 员工重复点击

- 同 command_id：返回原结果，不重做状态、Audit、历史或 Outbox；
- 不同 command_id、同员工、目标已 accepted：already_accepted；
- 不同 command_id、目标已 completed：already_completed；
- 不同员工或非 Assignment Actor：not_assigned_to_actor；
- 客户端不能因收到 202 pending 就把 Projection 写成 Core 最终状态。

### 7.4 任务已取消、Command 迟到

- awaiting_confirmation 取消：所有 proposed Candidate 置 invalidated，不创建 Edge 可执行 Projection；
- assigned 取消：已 selected Candidate 保留为历史选择，Assignment 置 cancelled，Personnel reserved → idle，发 task.cancelled.v1；
- in_progress 的 Task 不因 Flight 状态输入自动终止；Task 保持 in_progress，等待 Manager/Admin 的显式取消；
- 显式授权取消 in_progress：Assignment accepted → cancelled，Task in_progress → cancelled，Personnel busy → idle，写原因和 Audit；
- 取消后迟到的 Accept/Complete 返回 task_cancelled，不重试、不产生新 Outbox；
- completed 不允许被取消或回退。

### 7.5 Personnel 状态变化

- Candidate 生成后 Personnel 变为 busy/unavailable：Candidate 仍保留快照但被确认时重新校验；
- Leader 确认时发现 Personnel 不再 idle：返回 candidate_no_longer_eligible，Candidate 置 invalidated，Task 保持 awaiting_confirmation；
- Reservation、Assignment 和 Task 更新必须同一事务完成，不能先写 Assignment 再异步预留 Personnel；
- 已 reserved/busy 的 Personnel 不能被普通状态接口直接改为 unavailable；必须先取消/完成活动 Assignment；
- RFID/UWB/设备状态不在本切片内，不能绕过 Personnel Application。

### 7.6 重复/乱序 Event

- Edge Inbox 先按 event_id 幂等；
- 同一 Task 的 Event 按 sync_version 收敛；
- 低版本事件忽略并记录 stale，不回退 Projection；
- 同版本不同 Payload 进入 projection_version_conflict，保留失败记录等待人工/版本化修复；
- Edge Projection 写失败只影响该 Event 的同步状态，不改变 Core 事实。

### 7.7 权限、格式和业务拒绝

- Envelope 缺字段、版本不支持、Payload 非法：记录失败，不执行业务；
- RBAC/Scope 不通过：写拒绝 Audit，返回 forbidden/not_assigned_to_actor，不自动重试；
- 状态机不允许：返回稳定 invalid_state，不产生状态副作用；
- 网络、HTTP 5xx、暂时数据库不可用：进入 retry/backoff；
- 失败记录必须保留 request_id、trace_id、command/event ID、result_code 和安全的错误摘要，不记录密码或完整 Token。

## 8. 验收场景冻结

以下场景必须在 BVS2-03 至 BVS2-06 转换为可执行的单元、HTTP、双库和 Compose 集成测试。测试使用隔离 fixture 和显式清理，不使用 TRUNCATE 清空用户数据库。

基础 Fixture：

~~~text
Flight F1：scheduled，目标 Team T1，Area A1
Template TPL1：enabled，trigger_type=flight_arrived，version=1
Personnel P1：T1，能力满足，idle
Personnel P2：T1，能力不足，idle
Personnel P3：T1，能力满足，busy
Personnel P4：T2，能力满足，idle
Leader L1：T1 Scope
Leader L2：T2 Scope
Staff S1：对应 P1
Staff S2：对应 P4
~~~

| ID | 场景 | Given / When | 必须断言 |
|---|---|---|---|
| BVS2-AT-01 | 正常生成 | F1 scheduled，提交一次有效到达 | F1=arrived；Task 唯一且 awaiting_confirmation；P1 为 Candidate；有历史/Audit/Outbox |
| BVS2-AT-02 | 到达重复 | 用不同传输 ID 重复同一 source_event_id | 返回 already_processed；Flight、Task、Candidate、Audit 成功副作用不重复 |
| BVS2-AT-03 | 生成幂等 | 同一到达触发在模板版本变化后以新来源 ID重放，generation_key 保持不变 | 只保留一个 Task Instance；保留首次触发时固定的模板版本，不追加第二个 Task |
| BVS2-AT-04 | 候选过滤 | 同时存在 P1/P2/P3/P4 | 只有 P1 为 eligible/proposed；P2 能力不足、P3 busy、P4 跨 Team 被排除 |
| BVS2-AT-05 | 确认成功 | L1 在 T1 Scope 确认 P1 | 一个 Assignment=confirmed；Task=assigned；P1=reserved；P1 Candidate=selected、其他 proposed Candidate=rejected；一条 task.assigned.v1 |
| BVS2-AT-06 | 领导越权 | L2 确认 T1 的 P1 | 返回 forbidden；无 Assignment/Reservation/Outbox；写拒绝 Audit |
| BVS2-AT-07 | 并发确认 | L1/Manager 同时确认不同 Candidate | 只有一个成功；Task 一个有效 Assignment；失败方得到稳定冲突结果 |
| BVS2-AT-08 | Candidate 失效 | 生成 Candidate 后 P1 先变 busy，再确认 | 返回 candidate_no_longer_eligible；Candidate invalidated；Task 仍 awaiting；无部分 Assignment |
| BVS2-AT-09 | 确认幂等 | 重放同一 confirmation_id | 返回第一次结果；不重复 Reservation、历史、Audit、Outbox |
| BVS2-AT-10 | Edge 不可用 | Core 确认成功时停止 Edge API | Core 事务成功；Outbox pending/retry；恢复 Edge 后 Projection 收敛到 assigned |
| BVS2-AT-11 | 确认前投影 | Task 尚未确认时读取 Staff Projection | 员工无可执行 Task Projection；Candidate 不出 Edge |
| BVS2-AT-12 | Accept 成功 | S1 对已分配 Task 发 Accept Command | Edge 先返回 pending；Core 处理后 Task=in_progress、Assignment=accepted、P1=busy；产生 accepted Event |
| BVS2-AT-13 | Accept 重复同 ID | 同一 Accept Command 投递两次 | Core Inbox 一条；业务副作用一次；返回同一 applied 结果 |
| BVS2-AT-14 | Accept 重复不同 ID | Task 已 in_progress，再发新 Accept Command | 返回 already_accepted；不重复历史/Audit/Outbox |
| BVS2-AT-15 | 员工越权 | S2 对不属于自己的 Task 发 Accept/Complete | not_assigned_to_actor/forbidden；不修改 Task、Assignment、Personnel |
| BVS2-AT-16 | Complete 成功 | S1 对 in_progress Task 发 Complete Command | Task=completed；Assignment=completed；P1=idle；产生 completed Event 和三类历史 |
| BVS2-AT-17 | Complete 重复 | 同 ID 重放，及不同 ID 在 completed 后重放 | 同 ID 返回原结果；不同 ID 返回 already_completed；无重复副作用 |
| BVS2-AT-18 | 跳过 Accept | Assignment confirmed 时直接 Complete | 返回 invalid_state；Task/Assignment/P1 不变；记录拒绝 Audit |
| BVS2-AT-19 | 任务取消未确认 | Manager/Leader 在 Scope 内取消 awaiting_confirmation Task | Task=cancelled；所有 proposed Candidate=invalidated；不创建可执行 Projection |
| BVS2-AT-20 | 任务取消已分配 | Manager/Leader 在 Scope 内取消 assigned Task、P1 reserved，并重放同一 cancellation_id | Task/Assignment=cancelled；selected Candidate 保留历史；P1=idle；Edge 最终 Projection=cancelled；重放不重复释放/历史/Outbox |
| BVS2-AT-21 | Flight 取消输入 | 已生成 in_progress Task 时收到 flight.cancelled.v1 | Flight 状态输入返回 flight_status_conflict；不自动取消 Task，Task 保持 in_progress |
| BVS2-AT-22 | 显式取消执行中 | 有权限 Manager/Admin 显式取消 | Task/Assignment=cancelled；P1 busy→idle；写原因、历史、Audit、cancelled Event |
| BVS2-AT-23 | 迟到 Command | Task 已 cancelled 后投递 Accept/Complete | 返回 task_cancelled；不重试、不产生新业务 Event |
| BVS2-AT-24 | Worker 重启 | Command/Outbox 在 processing 时停止 Worker 后重启 | Pending/过期 processing 被重新领取；最终只产生一次业务效果 |
| BVS2-AT-25 | Edge Projection 乱序 | 先投递高 sync_version，再投递低版本 | Projection 保留高版本；低版本 ignored/stale；无状态回退 |
| BVS2-AT-26 | ID 内容冲突 | 同一 command_id/event_id 使用不同 Payload | 返回 ID conflict；不执行第二份 Payload；保留失败记录 |
| BVS2-AT-27 | 权限与 Scope | manager、leader、staff 分别访问跨 Area/Team/他人任务 | 只允许矩阵规定的范围；Repository 使用结构化 Scope；越权有稳定码和 Audit |
| BVS2-AT-28 | 事务原子性 | 在 Task/Personnel/Audit/Outbox 任一步注入失败 | Core 事实、历史、Reservation、Audit、Outbox 全部回滚；不留下半分配 |
| BVS2-AT-29 | 来源 ID 内容冲突 | 同一 source_event_id 再次提交不同规范化 Payload | 返回 source_event_id_conflict；不修改 Flight、Task、Candidate、Audit 或 Outbox；保留冲突记录 |
| BVS2-AT-30 | Flight 取消未到达 | F1 仍为 scheduled，提交 flight.cancelled.v1 | F1=cancelled；不生成 Task；写 Flight history/Audit；不创建 Task Projection |

## 9. BVS2-02 数据冻结边界

BVS2-02 只能根据本文件建立 Core 业务表和历史表，不得提前增加未被切片使用的业务能力。

Core 预计需要：

~~~text
operation_area
team
personnel
team_member
flight
flight_status_history
task_template
task_instance
task_status_history
task_candidate
task_assignment
task_assignment_status_history
personnel
personnel_status_history
business_idempotency_record（或等价的按业务操作持久化记录）
~~~

`operation_area`、`team`、`personnel` 和 `team_member` 是本切片所需的最小组织前置事实，用于表达单机场的 Area/Team Scope 和候选人归属；它们不是多机场或多租户隔离模型。`personnel.team_id` 表示当前主班组，`team_member` 保留成员关系记录，候选筛选通过 Core 的组织事实完成。

命令结果必须可持久化重放，不能只依赖 Foundation `core_inbox` 的处理状态。BVS2-02 必须在 Core 数据库中为 `command_id` 提供等价的结果记录能力（可扩展 `core_inbox`，也可使用独立记录），至少保存：规范化请求摘要/hash、command 类型和版本、处理结果码、最终业务状态、可安全返回的结果 Payload/错误摘要、首次处理时间以及冲突标记。相同 `command_id` 且请求内容相同返回原结果；相同 `command_id` 但内容不同返回 `command_id_conflict`。

要求：

- 所有 API/同步/日志/审计/外部集成使用 public_id；内部 BIGINT 只用于 FK/JOIN；
- Flight、Task、Candidate、Assignment、Personnel 的唯一键和状态版本必须支持上述幂等/并发规则；
- generation_key、source_event_id、confirmation_id、cancellation_id、command_id、event_id 的唯一性必须在 Core/Edge 正确的数据库中落实；
- Task/Assignment/Personnel 状态历史为追加记录，不用覆盖当前状态替代审计；
- Edge 只扩展最小 Projection/Command 字段，不创建完整 flights、personnel、task_instances 或 events 事实副本；
- BVS2-02 不修改 Foundation migration，不执行 destructive migration，不连接或修改 Edge 业务事实。

## 10. BVS2-01 冻结表

| 对象 | 本次必须冻结的决定 |
|---|---|
| Flight | scheduled → arrived 是唯一任务触发；Flight 不拥有 Task 生命周期；source_event_id 幂等，generation_key=flight_public_id + trigger_type |
| Task | awaiting_confirmation → assigned → in_progress → completed；非终态可按规则取消；完成/取消终态 |
| Candidate | 由班组/岗位/能力/idle/冲突规则生成；确定性排序；必须在确认时重新校验；不等于 Assignment |
| Assignment | Leader/Manager 确认后才创建，初始 confirmed；confirmed → accepted → completed；每 Task 一个有效 Assignment |
| Personnel | idle → reserved → busy → idle；Personnel Application 拥有状态；Task 不直接 UPDATE Personnel |
| Leader/Manager | task:assign 在 area/team Scope 内确认；确认是 Core 事务，使用 confirmation_id 幂等 |
| Employee | 只能操作本人有效 Assignment；Accept/Complete 都是 Edge Command，不能直接改 Projection |
| Core Event | task.assigned.v1、task.accepted.v1、task.completed.v1、task.cancelled.v1；最小投影 Payload；sync_version 单调 |
| Edge Command | employee_accept_task.v1、employee_complete_task.v1；aggregate 为 Task；Core 最终确认 |
| Idempotency | 传输 ID、来源 ID、生成键、确认/取消 ID、command_id、projection sync_version 和业务目标幂等分别定义 |
| Failure | 传输/暂时错误 retry；权限/状态/格式错误终止；重复/迟到/取消按稳定结果码处理；Worker/Edge 恢复不丢事实 |
| Acceptance | BVS2-AT-01 至 BVS2-AT-30 为后续可执行测试清单；正常、重复、内容冲突、越权、并发、取消、迟到、乱序、恢复均覆盖 |

## 11. BVS2-01 完成条件

- 六组交付物已冻结：业务规则、状态机、权限/Scope、Core/Edge 契约、异常/补偿、可执行验收场景；
- Flight/Task、Candidate/Assignment、Personnel 状态拥有权和取消行为没有未定义分支；
- 同一 ID 重放与不同 ID 的业务目标幂等已明确区分；
- BVS2-01 阶段只冻结 BVS2-02 的数据和 Migration 边界；后续 BVS2-02 已按该边界建立 Core 表，BVS2-03 在此基础上实现首个 Core 业务用例；
- Architecture Foundation 的 Core/Edge、双 MySQL、Outbox/Inbox、RBAC/Scope、Principal、Audit 和 Migration 边界没有被本设计改写。

因此 BVS2-01 状态为 FROZEN；BVS2-02 数据基线和 BVS2-03 Flight → Task → Candidate 已完成，下一步进入 BVS2-04 Leader Confirm。
