# 架构记录

> 当前架构事实源。过时的 BVS2 阶段说明、旧版员工 Accept 流程和旧手工航班入口已删除；阶段进度与验证结果见 [`progress.md`](progress.md)。

## Architecture v2.0 冻结结论

- 单仓库、Core 模块化单体、Edge 接入服务、Core/Edge 独立 MySQL；系统只服务单机场。
- Core 是航班、人员、岗位、能力、人员状态、任务模板/实例/候选/分配、事件、规则、审计和状态机的唯一事实源。
- Edge 只保存员工端所需的 Projection、Command、Session、Inbox、通知和 delivery 数据；Edge 不拥有业务事实。
- Core 与 Edge 使用独立账号、schema 和版本化 SQL migration。服务启动不调用 GORM `AutoMigrate`，Edge 不直连 Core 数据库。
- 同步采用持久化 Outbox/Inbox、at-least-once、幂等、版本收敛、Retry/Failed；Redis 只用于缓存、限流、短锁、在线状态和 best-effort fan-out。
- 不引入 `tenant_id`、`airport_id`、多机场切换、Kafka/微服务拆分或未经设计确认的外部推送。

## 代码边界

| 边界 | 目录 | 责任 |
| --- | --- | --- |
| Core 进程 | `cmd/core-api` | 事实查询、管理写入、任务与状态机 API |
| Edge 进程 | `cmd/edge-api` | 员工认证、Projection、Command、通知与实时提示 |
| Worker | `cmd/worker` | 航班源 inbox 应用、Outbox 投递、Command 拉取、超时重派 |
| Migration | `cmd/migrate` | 显式执行 Core/Edge 独立 migration |
| Core 领域 | `internal/core/module` | flight、personnel、task、iam 等事实与值对象 |
| Core 用例 | `internal/core/application` | flighttask、flightsync、taskchange、management、managementrealtime |
| Edge 同步 | `internal/edge/sync` | Inbox、Projection、Command 的持久化收敛 |
| 集成层 | `internal/integration` | 外部航班 Provider、Core/Edge Transport、Worker |
| 平台层 | `internal/platform` | 配置、安全、MySQL、Redis、健康检查、日志、指标 |

Handler 不直接调用 GORM；Service 不依赖 Gin；Domain 不依赖 Gin/GORM/Redis。业务事实、状态历史、Audit 和 Outbox 必须在同一个 Core 事务中提交。

## 当前业务流程

```text
外部航班源
  -> flight_source_inbox（幂等接收）
  -> Worker 应用航班事实
  -> 航班到达
  -> Task pending_dispatch
  -> 自动预分配
  -> Core Assignment 确认
  -> Outbox -> Edge Inbox/Projection
  -> 员工 received（已收到）
  -> 员工 start -> complete
```

航班相关事实只能来自外部接口。管理端可以读取航班，也可以查看同步健康，但不能任意新增、修改或删除航班来伪造业务事件；开发种子中的航班仅是测试夹具。

### 航班同步

- 适配边界：`internal/integration/flight.Provider`。
- 批量接收：`POST /internal/integration/v1/flight-source/sync`。
- 单航班到达：`POST /internal/integration/v1/flights/{flightPublicID}/arrival`，受 `X-Flight-Source-Key` 保护。
- 健康诊断：`GET /internal/integration/v1/flight-source/health`。
- 源记录先写入 Core `flight_source_inbox`，按 `provider + record_type + external_record_id` 幂等；Worker 异步应用到航班事实。
- 来源状态为 `fresh`、`stale`、`fallback`、`failed`。接口失败时重试并保留错误诊断，继续读取前一晚已预同步的数据库事实；不把 fallback 伪装为实时成功。

## 任务、派发与握手

- 航班到达按照有效模板生成任务，任务初始状态为 `pending_dispatch`；不开放通用手工 `POST /tasks`、任意 PATCH 或硬删除。
- `AutomaticTaskDispatcher` 使用冻结的 v1 规则：员工启用、同区域/活动主班组、精确岗位、精确能力、`idle`，并且同一计划时间没有活动 Assignment。
- 排序为 `last_state_changed_at ASC`，再按候选公共 ID 升序。候选快照随任务保存，自动派发和超时重派使用同一快照与规则。
- 预留冲突只使当前候选失效并继续尝试；没有候选时任务保持可见并标记短缺，不静默取消。
- 员工收件超时默认 300 秒。Worker 锁定并复核后释放旧预留，再按相同规则尝试替补。

三层握手：

1. 系统确认：Core 原子预留人员、创建 Assignment、更新任务并写入历史/Audit/Outbox。
2. 投影确认：Worker 投递 Outbox，Edge Inbox/Projection 持久化、去重、重试并按版本收敛。
3. 员工业务确认：`received` 仅表示通知已收到；`start` 表示开始执行；`complete` 表示完成。

员工不得拒绝系统安排。若员工正在保障其他航班或遇到异常，提交异常/任务变更申请，不产生拒绝状态。

## 任务变更申请与权限

员工、队长或授权外部来源只能报告，不得直接修改任务。可申请 `pause`、`reassign`、`reschedule`、`cancel`、`resume`；重新排班必须提供目标时间。

- `POST /api/v1/task-change-requests`：创建申请。
- `GET /api/v1/task-change-requests`：按权限和 Scope 分页查询。
- `PATCH /api/v1/task-change-requests/{publicID}/review`：经理/管理员审批并应用。

`manager` 与 `admin` 可以审批和应用；`leader` 可以在 Scope 内查看、报告和跟进；`supervisor` 是分管领导只读管理角色；`staff` 只能处理本人任务的收件、开始、完成和异常报告。审批事务会重新检查任务版本、状态、人员资格和时间冲突，成功后同步历史、Audit 和 Outbox，失败申请保留为 `failed`。

## 实时、分页与恢复

- 管理端 `GET /api/v1/realtime/management` 是按 Scope 过滤的短时 SSE 提示流，覆盖 admin/manager/leader/supervisor；staff 不订阅。
- 管理端使用 `Last-Event-ID/after_id` 恢复 outbox 游标，断线后必须重新读取分页事实。
- 员工 WebSocket 与个人微信/企业微信原生 socket 只发送 best-effort `task_changed` 提示，不保存可靠事实。
- Core 管理集合、Edge 历史和通知使用 `page/page_size/total` 与稳定排序；员工任务保留按员工隔离的完整快照，用于启动、刷新、断线和离线恢复。

## 数据库与迁移

- Core migration：`migrations/core/mysql`。
- Edge migration：`migrations/edge/mysql`。
- 当前业务增量包含 Core `000010`（派发/收件）、`000011`（任务变更）、`000012`（航班源健康）、`000013`（supervisor），以及 Edge `000007`（收件投影）。
- migration 文件存在不等于数据库已应用；状态必须通过 `cmd/migrate -target core|edge -command status` 的实际输出确认。

## 验证规则

任何功能、集成、数据库或 Compose 验证前先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

统一入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

Docker 不可用时必须明确记录失败原因；未运行的测试、迁移或 Compose 场景不能标记为通过。禁止默认执行 `down -v`、清库、`TRUNCATE`、migration down 或修改用户已有服务。

## 关联文档

- 业务流程：[`docs/business-process-v2.md`](../docs/business-process-v2.md)
- 代码导览：[`docs/code-tour-v2.md`](../docs/code-tour-v2.md)
- 前后端交接：[`docs/frontend-backend-handoff.md`](../docs/frontend-backend-handoff.md)
- 任务合同：[`docs/task-crud-contract-v2.md`](../docs/task-crud-contract-v2.md)
- 前端冻结门槛：[`frontend/docs/architecture-freeze-gate.md`](../frontend/docs/architecture-freeze-gate.md)
- 设计决策：[`memory-bank/design-document.md`](design-document.md)
- 当前进度：[`memory-bank/progress.md`](progress.md)
