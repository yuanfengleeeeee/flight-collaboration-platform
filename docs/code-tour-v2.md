# Architecture v2 代码导览

> 本文件是当前代码入口的导览，不再保留旧版 BVS2 阶段草案。业务流程以 [`docs/business-process-v2.md`](business-process-v2.md) 为准；实际验证结果以 [`memory-bank/progress.md`](../memory-bank/progress.md) 为准。

## 1. 当前业务主干

```text
外部航班源
  -> Core flight_source_inbox（幂等接收）
  -> Worker 应用航班事实 / fresh-stale-fallback-failed
  -> 航班到达
  -> Task pending_dispatch
  -> 自动候选筛选与预分配
  -> Core Assignment 确认
  -> Core Outbox -> Worker -> Edge Inbox/Projection
  -> 员工 received（收件回执）
  -> 员工 start -> complete
```

员工没有“拒绝任务”命令。员工确认的是“已收到通知”，不是是否接受工作；已在保障其他航班时，通过异常/变更申请报告冲突，由经理或管理员审批处理。

## 2. 服务与进程入口

| 入口 | 职责 |
| --- | --- |
| `cmd/core-api` | Core 事实、管理查询和管理写入；只连接 Core MySQL |
| `cmd/edge-api` | 员工会话、员工任务投影、Command 和实时提示；只连接 Edge MySQL |
| `cmd/worker` | 航班源 inbox 应用、Core Outbox 投递、Edge Command 拉取、超时重派 |
| `cmd/migrate` | 按 `-target core|edge` 显式执行独立 SQL migration |

旧 `cmd/server`、旧 `internal/module` 和旧 B3/B4/B5 目录保持 `legacy/paused`，不能作为 v2 生产入口恢复。

## 3. Core 代码边界

- `internal/core/module/flight/`：航班事实、来源、时刻、到达/延误/取消等生命周期事实。
- `internal/core/module/personnel/`：员工、岗位、能力、人员状态、班组与区域归属。
- `internal/core/module/task/`：模板、任务实例、候选快照、Assignment、状态历史和领域状态机。
- `internal/core/application/flighttask/`：航班到达触发、任务查询和任务生命周期操作。外部到达入口由集成认证保护，不接受浏览器手工写入。
- `internal/core/application/flightsync/`：批量航班源接收、来源健康查询和同步诊断。
- `internal/core/application/taskchange/`：任务变更申请创建、经理/管理员审批和原子应用。
- `internal/core/application/management/`：组织、人员、模板、字典和管理查询；航班事实不通过管理端任意新增。
- `internal/core/application/managementrealtime/`：按角色和 Scope 过滤的管理端 SSE 提示流。
- `internal/core/adapter/mysql/`：Core Repository 和事务边界；Handler 不直接调用 GORM。

### Core 写入原则

业务事实、状态历史、Audit 和 Outbox 在同一个 Core 事务内提交。任务变更必须带版本并重新检查状态、候选资格和人员冲突；失败申请保留为 `failed`，不丢失原因和审计链。

## 4. 航班源接入路径

外部航班源通过 `internal/integration/flight.Provider` 适配，不把供应商字段泄漏到领域层。当前标准入口为：

- `POST /internal/integration/v1/flight-source/sync`：批量接收标准化 Schedule/Event，快速写入 `flight_source_inbox` 后返回接收统计。
- `POST /internal/integration/v1/flights/{flightPublicID}/arrival`：受 `X-Flight-Source-Key` 保护的内部到达入口。
- `GET /internal/integration/v1/flight-source/health`：查看来源新鲜度、最近成功/失败、重试和 fallback 状态。

接入与应用分离：收到源记录不等于航班事实已经应用，也不等于任务已经分配。失败记录带重试/退避信息；源不可用时继续读取前一晚已预同步的数据库事实，不伪造“实时成功”。

## 5. 自动派发与三层握手

- 任务由航班到达生成并进入 `pending_dispatch`，不提供通用手工 `POST /tasks`。
- `AutomaticTaskDispatcher` 使用候选快照和当前冻结的 v1 规则：启用、同区域/主班组、精确岗位、精确能力、`idle`、同一计划时间无活动 Assignment；按 `last_state_changed_at` 升序、候选公共 ID 升序。
- 候选冲突只淘汰当前候选并继续尝试；全部失败时保留待派任务和短缺信息，不静默取消。
- 收件超时默认 300 秒。Worker 锁定并复核后释放旧预留，按同一规则重新分配；没有替补时仍保留任务供管理处理。

三层握手分别是：

1. 系统确认：Core 在事务中确认候选、预留员工并创建 Assignment。
2. 投影确认：Core Outbox、Edge Inbox/Projection 持久化、重试、去重并按版本收敛。
3. 员工业务确认：员工提交 `received` 表示已收到，`start` 表示开始执行，`complete` 表示完成。

## 6. 任务变更申请

员工、队长或授权外部来源只能报告异常并创建申请，不能直接改任务。支持 `pause`、`reassign`、`reschedule`、`cancel`、`resume`；`reschedule` 必须提供目标时间。

- 创建：`POST /api/v1/task-change-requests`
- 列表：`GET /api/v1/task-change-requests`
- 审批：`PATCH /api/v1/task-change-requests/{publicID}/review`

只有 `manager` 或 `admin` 可以审批和应用；`leader` 可以报告、查看其 Scope 内任务并跟进；`supervisor` 只读观察；`staff` 只能报告本人任务异常。

## 7. Edge 员工链路

- `internal/edge/application/server.go`：员工认证、任务快照、`received/start/complete` Command、异常报告和命令状态查询。
- `internal/edge/sync/`：Edge Inbox、Projection、Command Store；负责版本收敛、幂等和失败重试。
- `internal/edge/application/realtime/`：员工 WebSocket/原生 socket 的 best-effort `task_changed` 提示；断线后必须重新读取完整快照。
- `internal/edge/application/notification/`：提交后通知抽象；通知不是可靠事实存储。

员工客户端只能访问 Edge，不能访问 Core 或 `/internal/sync/v1/*`。HTTP `202` 只代表 Command 已接收，最终业务状态必须等待 Core 事件回投并从 Edge Projection 读取。

## 8. 管理端实时与读取

`GET /api/v1/realtime/management` 提供短时、按 Scope 过滤的 SSE 提示流，覆盖 `admin`、`manager`、`leader`、`supervisor`；`staff` 不订阅管理流。客户端使用 `Last-Event-ID/after_id` 恢复提示游标，并重新读取分页 API 校准事实。

管理端集合接口使用 `page/page_size/total` 和稳定排序；Edge 历史、通知同样分页。员工任务是按员工隔离的完整快照，用于启动、刷新、断线和离线恢复，不等同于管理端的无界全量查询。

## 9. 前端入口

| 客户端 | 数据边界 | 当前重点 |
| --- | --- | --- |
| `frontend/apps/admin-web` | Core API | 任务、派发/Assignment、变更审批、人员、岗位/能力、模板和运行监控 |
| `frontend/apps/employee-web` | Edge API | 收件、开始、完成、异常申请、通知、历史和账号 |
| `frontend/apps/employee-miniapp` | Edge API | 个人微信登录、任务和原生实时提示 |
| `frontend/apps/employee-wecom-miniapp` | Edge API | 企业微信登录、任务和原生实时提示 |

共享包只承载 DTO、状态映射、API Client、会话和平台适配；权限、事务、最终状态和可靠同步仍在后端。

## 10. 当前实现与验收边界

当前代码/契约已覆盖自动派发、候选规则、三层握手、超时重派、变更审批、航班源 inbox/fallback、管理订阅、字典 CRUD 和集合分页。正式验收仍需按实际环境完成真实航班 Provider、微信/企业微信凭据与域名、告警投递、多副本故障、性能基线和接口失效/重复消息/断线/延误/取消场景。

Core migration `000010` 至 `000013`、Edge migration `000007` 是对应增量结构；是否已应用必须以当前迁移状态命令的真实输出为准，不能由文件存在推断为已应用。
