# Architecture v2.0 基线

> 状态：`ACCEPTED / FROZEN`
> 更新时间：2026-09-07
> 适用范围：Core、Edge、Worker、迁移、同步协议、前端边界和本地部署

## 1. 总体形态

```text
管理端 Web -> Core API -> Core MySQL
                      -> Core Outbox

外部航班源 -> Core flight_source_inbox -> Worker -> Core 事实
                                      -> Task/Assignment
                                      -> Outbox -> Edge Inbox/Projection

员工 Web / 个人微信小程序 / 企业微信小程序 -> Edge API -> Edge MySQL
                                               -> Command -> Worker -> Core
```

系统只服务单机场。Core 是模块化单体，不拆成 flight/task/personnel/event/rule 微服务；Core 与 Edge 使用独立数据库、账号、schema 和 migration。

禁止 `tenant_id`、`airport_id`、多机场切换、Edge 直连 Core DB、Core 直写 Edge DB 和服务启动 `AutoMigrate`。

## 2. 数据所有权

Core 唯一拥有：

- 航班、航班源、时刻和到达/延误/取消事实；
- 员工、岗位、能力、状态、区域、班组和身份管理；
- 任务模板、任务实例、候选快照、Assignment、状态历史、变更申请；
- 事件、规则、审计和业务状态机。

Edge 只保存员工端最小 Projection、Command、Session、Inbox、通知和 delivery。Edge 不拥有航班、员工或任务最终事实。

业务事实、状态历史、Audit 和 Outbox 必须在一个 Core 事务中提交。可靠同步使用 at-least-once、幂等、版本收敛、Retry/Failed；Redis 只承担缓存、限流、短锁、在线状态和 best-effort fan-out。

## 3. 程序与目录

| 程序/目录 | 责任 |
| --- | --- |
| `cmd/core-api` | Core 事实查询、管理查询、管理写入、状态机用例 |
| `cmd/edge-api` | 员工认证、快照、Command、通知、实时提示 |
| `cmd/worker` | 航班源应用、Outbox 投递、Command 拉取、超时重派 |
| `cmd/migrate` | 显式执行 Core/Edge 独立 SQL migration |
| `internal/core/module` | flight、personnel、task、iam 等领域事实 |
| `internal/core/application` | flighttask、flightsync、taskchange、management、managementrealtime |
| `internal/edge/sync` | Edge Inbox、Projection、Command 持久化和版本收敛 |
| `internal/integration` | 外部航班 Provider、Core/Edge Transport、Worker 适配 |
| `internal/platform` | 配置、安全、MySQL、Redis、健康、日志、指标 |

Handler 不直接调用 GORM；Service 不依赖 Gin；Domain 不依赖 Gin、GORM 或 Redis。

## 4. 当前业务状态机

```text
外部航班源
  -> source inbox
  -> 航班事实应用
  -> 航班到达
  -> Task pending_dispatch
  -> 自动预分配
  -> Assignment confirmed / Task assigned
  -> Edge Projection
  -> received（已收到通知）
  -> start（开始执行）
  -> complete（完成）
```

员工没有拒绝任务的业务动作。`received` 只证明员工已经收到通知，不代表同意；正在保障其他航班或遇到突发情况时，员工通过异常/任务变更申请报告。

### 自动调度 v1

硬过滤：员工启用、属于任务区域和活动主班组、精确岗位匹配、精确能力匹配、`idle`、同一计划时间没有活动 Assignment。

排序：`last_state_changed_at ASC`，再按候选公共 ID 升序。任务保存候选快照；自动派发和超时重派使用同一快照与规则。候选冲突只淘汰当前候选并继续尝试；没有替补时保留任务和短缺信息，不静默取消。

## 5. 三层握手

1. 系统确认：Core 事务锁定任务和候选，预留员工，创建 Assignment，写入 History/Audit/Outbox。
2. 投影确认：Worker 投递 Outbox，Edge Inbox/Projection 持久化、去重、重试并按版本收敛。
3. 员工业务确认：员工提交 `received`、`start`、`complete`；业务最终状态以 Core Event 回投后的 Edge Projection 为准。

收件超时默认 300 秒。Worker 锁定并复核后释放旧预留，再按同一调度规则尝试替补；没有替补时不把超时当员工拒绝。

## 6. 航班源与 fallback

外部航班通过 `internal/integration/flight.Provider` 适配，供应商字段不得进入领域层。标准入口：

- `POST /internal/integration/v1/flight-source/sync`：批量写入 `flight_source_inbox` 后快速返回接收统计。
- `POST /internal/integration/v1/flights/{flightPublicID}/arrival`：受 `X-Flight-Source-Key` 保护的内部到达入口。
- `GET /internal/integration/v1/flight-source/health`：来源健康与同步诊断。

幂等键为 `provider + record_type + external_record_id`。接收和应用分离，Worker 异步处理；源健康状态为 `fresh`、`stale`、`fallback`、`failed`。Provider 失败时继续读取前一晚/最近一次预同步的数据库事实，保留 retry、退避和失败诊断，不把 fallback 伪装成实时成功。

管理端的 `GET /api/v1/flights` 只读 Core 事实，不提供任意航班新增、修改或删除；开发种子航班仅用于测试夹具。

## 7. 任务变更与角色

员工、leader 或授权外部来源只能报告，不得直接改任务。任务变更申请支持 `pause`、`reassign`、`reschedule`、`cancel`、`resume`：

- `POST /api/v1/task-change-requests`
- `GET /api/v1/task-change-requests`
- `PATCH /api/v1/task-change-requests/{publicID}/review`

只有 `manager`/`admin` 可以审批并应用；`leader` 可以查看其 Scope、报告和跟进；`supervisor` 只读观察；`staff` 只能操作本人任务的收件、开始、完成和异常报告。审批会重新校验任务版本、状态、人员资格和时间冲突，成功后原子写入变更、History、Audit 和 Outbox，失败申请保留。

## 8. 实时与读取恢复

- 管理端 `GET /api/v1/realtime/management` 是按 Scope 过滤的短时 SSE，覆盖 admin/manager/leader/supervisor；staff 不订阅。
- 管理端使用 `Last-Event-ID/after_id` 恢复提示游标，并重新读取分页 API。
- 员工 WebSocket 和两个小程序原生 socket 只发送 `task_changed` 等 best-effort 提示；断线、重启或提示丢失时重新读取完整员工快照。
- Core 管理集合、Edge 历史和通知使用 `page/page_size/total` 与稳定排序；员工任务快照按员工隔离，不是管理端无界全量查询。

## 9. 前端与身份

`admin-web` 只调用 Core；`employee-web`、`employee-miniapp`、`employee-wecom-miniapp` 只调用 Edge。个人微信和企业微信都绑定到同一 Core `Staff`，小程序不把平台 openid/unionid/UserId 直接当业务身份。

真实微信/企业微信 Provider、管理端 SSO、AppID/CorpID、HTTPS 域名、回调白名单、管理员预置和平台通知是部署验收事项；`mock:<subject>` 仅用于本地开发。

## 10. 数据库与迁移

- Core：`migrations/core/mysql`。
- Edge：`migrations/edge/mysql`。
- 当前业务增量：Core `000010_task_dispatch_receipt`、`000011_task_change_request`、`000012_flight_source_health`、`000013_supervisor_role`；Edge `000007_task_receipt_projection`。
- migration 文件存在不等于已应用；使用 `go run ./cmd/migrate -target core|edge -command status` 以实际输出判断。

## 11. 验收边界

代码/契约已覆盖自动派发、三层握手、收件超时重派、变更审批、航班源 inbox/fallback、管理订阅、岗位/能力字典 CRUD 和集合分页。仍需在隔离环境完成真实 Provider、平台凭据/域名、告警渠道、多副本故障、性能基线，以及接口失效、重复消息、断线、延误、取消、无候选和审批失败测试。

任何功能、数据库或 Compose 验证前先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

统一入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

未实际运行的验证不得标记为通过；不得默认执行清库、`TRUNCATE`、migration down 或 `docker compose down -v`。
