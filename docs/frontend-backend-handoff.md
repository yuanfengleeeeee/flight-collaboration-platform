# 前后端交接：当前 v2 业务与接口边界

> 更新时间：2026-09-07
> 状态：当前交接基线
> 详细业务流程：[`business-process-v2.md`](business-process-v2.md)

本文只保留当前可执行的前后端交接规则。已经淘汰的 B3 草案、旧阶段进度和旧接口示例不再作为交接内容；如需了解历史，查看 `memory-bank/progress.md`，但历史记录不能指导开发或验收。

## 1. 物理边界

```text
admin-web ───────────────→ Core API ─→ Core MySQL
                                      └→ Core Outbox
                                              ↓ Worker
employee-web ─┐                         Edge Inbox/Projection ─→ Edge MySQL
personal miniapp ─┤→ Edge API ─→ Edge Command ─→ Worker ─→ Core
WeCom miniapp ─┘
```

| 客户端/组件 | 允许调用 | 不允许调用 |
| --- | --- | --- |
| `frontend/apps/admin-web` | Core 公开管理 API、管理 SSE | Edge、数据库、内部同步接口 |
| `frontend/apps/employee-web` | Edge 员工 API、员工 WebSocket | Core、数据库、内部同步接口 |
| `frontend/apps/employee-miniapp` | Edge 员工 API、原生实时提示 | Core、数据库、内部同步接口 |
| `frontend/apps/employee-wecom-miniapp` | Edge 员工 API、原生实时提示 | Core、数据库、内部同步接口 |
| `cmd/worker` | Core DB、受保护的 Edge 内部同步 API | 前端直连、把 Redis 当可靠队列 |

Core 是航班、人员、岗位、能力、任务、Assignment、事件、规则、审计和状态机的唯一事实源。Edge 只保存员工 Projection、Command、Session、Inbox 和 delivery。两边使用独立 MySQL/schema/migration，服务启动不执行 `AutoMigrate`。

## 2. 当前业务流程

```text
外部航班源
  → Core flight_source_inbox（快速、幂等落库）
  → Worker 应用 Schedule/Event
  → 到达事件 + 启用模板生成 pending_dispatch Task/Candidate 快照
  → 自动调度器选择人员、预留 Assignment
  → Core Outbox → Edge Inbox/Projection
  → 员工 received（收件回执）
  → 员工 start（开始执行）
  → 员工 complete（完成保障）
```

员工不得拒绝任务。`received` 只证明通知已收到，不是员工同意；已在其他航班保障时，应提交异常/变更申请。旧 `accept` 仅为兼容入口，等价于 `start`，不得在新前端中当作“接受任务”文案或权限。

## 3. 航班事实与同步

航班只能由外部 Provider 提供。Provider 适配器位于 `internal/integration/flight.Provider` 边界，当前代码提供同步接收位置，但真实 AODB/航司适配器、凭据和生产定时配置仍待外部资料。

当前内部入口：

```text
POST /internal/integration/v1/flight-source/sync
POST /internal/integration/v1/flights/{flightPublicID}/arrival
GET  /internal/integration/v1/flight-source/health
```

同步入口使用 `X-Flight-Source-Key`（本地显式开发配置除外），只做鉴权、规范化校验、幂等接收和 `flight_source_inbox` 落库，不等待任务生成。批量记录唯一键为 `provider + record_type + external_record_id`。Worker 负责租约领取、应用、指数退避、失败阈值和诊断。

航班源健康状态为：

| 状态 | 读取/处理语义 |
| --- | --- |
| `fresh` | 在新鲜度窗口内使用最近成功同步数据 |
| `stale` | 数据过期但仍有可读的最近事实，显示陈旧状态并进入告警观察 |
| `fallback` | 当前源不可用，使用前一晚或最近一次预同步数据库事实继续任务处理 |
| `failed` | 当前记录/应用达到失败阈值，保留重试时间、错误摘要和诊断信息 |

管理端的航班页面只读 `GET /api/v1/flights`。开发种子航班不是外部 Provider 接入的替代品。航班延误必须携带新计划时间；航班取消/延误不会直接覆盖活动任务，活动任务变更必须走变更申请和审批。

## 4. Task、Assignment 和三层握手

### 4.1 状态

```text
Task:       pending_dispatch → assigned → in_progress → completed
                              └────────────────────────→ cancelled

Assignment: confirmed → accepted → completed
                         └──────────────→ cancelled

Personnel:  idle → reserved → busy → idle
```

`awaiting_confirmation` 只作为历史兼容读取状态；新任务不以它作为自动派发入口。`completed` 和 `cancelled` 是终态。

### 4.2 自动派发 v1

候选硬条件：人员启用、区域/班组匹配、当前主班组成员、精确岗位和能力匹配、`work_state=idle`、计划时间无活动 Assignment 冲突。排序为 `last_state_changed_at ASC`，再按候选人员 public ID 升序。候选快照写入 Core，预留时再次校验当前事实。

自动调度器按排序顺序复用 Core Assignment 确认事务。候选因并发冲突失效后继续下一个；全部失败时保留任务和缺员状态，不静默取消。公平性、班次、休息、交接班和跨航班优化不属于当前 v1，必须以版本化调度规则和业务评审引入。

### 4.3 三层确认

| 层次 | 记录什么 | 前端如何展示 |
| --- | --- | --- |
| 系统确认 | Core 已创建 Assignment 并预留人员 | `assigned`，带系统同步状态 |
| 投影确认 | Outbox/Inbox 已提交 Edge Projection | 员工可查询到任务 |
| 员工业务确认 | 员工提交 `received` | 已收件，不显示“已同意” |

员工动作接口：

```text
POST /api/v1/tasks/{taskPublicID}/received
POST /api/v1/tasks/{taskPublicID}/start
POST /api/v1/tasks/{taskPublicID}/complete
POST /api/v1/tasks/{taskPublicID}/exceptions
GET  /api/v1/commands/{commandID}
```

Edge 对员工动作先持久化 Command 并返回 `202`。前端展示 `pending/syncing`，再查询 `confirmed/failed`；不能把 `202` 直接写成任务已开始或已完成。员工收到 `task_changed`、WebSocket 断线、提示丢失或乱序时，重新拉取完整的 `GET /api/v1/tasks` 快照。

收件超时默认 300 秒。Worker 锁定并复核未收件 Assignment，释放旧预留，按相同规则尝试其他候选；没有候选时保留可见的缺员状态。

## 5. 任务变更申请

员工、队长或授权外部指令只能创建变更申请，不能直接改 Task/Assignment。动作：`pause`、`reassign`、`reschedule`、`cancel`、`resume`；`reschedule` 必须带目标计划时间。

```text
GET   /api/v1/task-change-requests
POST  /api/v1/task-change-requests
PATCH /api/v1/task-change-requests/{publicID}/review
```

队长可以上报和查看；只有值班经理或管理员可以审批。审批时重新校验 Task 版本、状态、候选资格和时间冲突，并在一个 Core 事务内更新 Task、Assignment、Personnel、历史、Audit 和 Outbox。结果为 `pending/approved/rejected/failed`；应用失败保留为 `failed`，不能伪装为已完成。

任务取消是保留历史的生命周期操作，不是硬删除；管理端使用 `POST /api/v1/tasks/{taskPublicID}/cancel`。不存在通用的手工 `POST /api/v1/tasks`、任意 PATCH 或硬删除任务接口。

## 6. 角色和实时订阅

| 角色 | 可见/可执行范围 |
| --- | --- |
| `admin` | 全局管理、主数据维护、变更审批和诊断；不能伪造员工执行动作 |
| `manager` | 授权范围内运行查看、补派/确认和变更审批 |
| `leader` | 授权区域/班组内查看和上报；不能审批变更 |
| `supervisor` | 授权范围内只读态势查看和管理实时订阅 |
| `staff` | 本人 Projection、收件、开始、完成和异常上报；不能拒绝或改任务 |

管理端使用 `GET /api/v1/realtime/management`，仅 `admin/manager/leader/supervisor` 按 Core Scope 接收短时 SSE 提示，使用 `Last-Event-ID`/`after_id` 恢复。员工使用 Edge WebSocket 或小程序原生 Socket，只接收本人的变化提示。所有实时通道都是 hint，可靠事实来自数据库和 HTTP 查询。

## 7. 分页与查询

Core 管理集合和 Edge 历史/通知集合统一使用服务端 `page`、`page_size`、`total`、稳定排序和边界限制。筛选条件交给服务端，前端切换筛选时回到第 1 页并取消过期请求。

员工 `GET /api/v1/tasks` 保持本人完整 Projection 快照，服务端按 JWT Principal 过滤；这是断线恢复合同，不是管理端一次性查询全部数据。实时提示、浏览器缓存和小程序本地存储均不能替代该快照。

## 8. 当前实现和未完成验收

当前代码/契约已经覆盖：岗位/能力独立 CRUD、员工/组织/模板维护、外部航班源接收边界、源健康/fallback 数据模型、自动派发、收件超时重派、三层握手、任务变更申请和经理审批、管理角色 Scope、实时订阅以及服务端分页。

仍未完成生产验收：真实航班 Provider 与字段映射、真实微信/企业微信凭据和 HTTPS 域名、平台告警/订阅消息、多副本/网络分区/服务重启故障门禁、正式员工与管理员主数据预置、性能 p50/p95/p99 基线和最终品牌评审。不能用开发种子、Mock Provider 或单机 Socket 验收代替这些项目。

正式验收应按 [`business-process-v2.md`](business-process-v2.md) 覆盖：接口失效与 fallback、重复/冲突消息、Worker/Edge 断线、延误、取消、候选冲突、收件超时重派、异常审批、员工无拒绝入口、Scope 越权、分页空结果和实时订阅边界。

## 9. 代码和契约索引

- Core 业务流程：`internal/core/application/flighttask`、`internal/core/application/taskchange`、`internal/core/application/flightsync`
- Core 管理与运维查询：`internal/core/application/management`、`internal/core/application/operations`、`internal/core/application/managementrealtime`
- Edge 员工入口：`internal/edge/application/server.go`、`internal/edge/application/realtime`
- Worker：`cmd/worker/main.go`
- Core OpenAPI：`api/core/openapi.yaml`
- Edge OpenAPI：`api/edge/openapi.yaml`
- 业务流程：`docs/business-process-v2.md`
- Task API：`docs/task-crud-contract-v2.md`
- 前端接入：`frontend/docs/api-integration.md`
