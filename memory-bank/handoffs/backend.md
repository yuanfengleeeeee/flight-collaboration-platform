# 后端任务交接快照

> 更新时间：2026-09-08
> 状态：当前代码/契约已覆盖主要业务流程；生产配置与专项验收待完成

详细业务流程见 [`docs/business-process-v2.md`](../../docs/business-process-v2.md)，前后端边界见 [`docs/frontend-backend-handoff.md`](../../docs/frontend-backend-handoff.md)。本快照只记录当前接手所需事实，不保留旧阶段草案。

## 当前架构

- Core 是 Go 模块化单体，入口为 `cmd/core-api`；Edge 入口为 `cmd/edge-api`；可靠同步和员工 Command 由 `cmd/worker` 处理；迁移由 `cmd/migrate` 处理。
- Core 是航班、人员、岗位、能力、任务模板/实例、Candidate、Assignment、事件、规则、审计和状态机的唯一事实源。
- Edge 只保存员工 Projection、Command、Session、Inbox 和 delivery。Core/Edge 使用独立 MySQL/schema/migration；禁止共库、跨库 JOIN 和 Edge 直连 Core。
- 可靠同步使用 Outbox/Inbox、幂等、版本收敛、Retry/Failed；Redis、WebSocket 和小程序 Socket 只作缓存/提示，不承担可靠事实。

## 当前业务流程

```text
外部航班 Provider
  → flight_source_inbox（快速、幂等预同步）
  → Worker 应用航班事实
  → 到达事件生成 pending_dispatch Task/Candidate 快照
  → 自动调度器完成 Assignment 预留
  → Outbox → Edge Inbox/Projection
  → 员工 received 收件
  → 员工 start 执行
  → 员工 complete 完成
```

员工不能拒绝已分配任务。`received` 是通知收件回执，不是业务同意；员工在其他航班保障或遇到异常时提交变更申请。旧 `accept` 仅为 `start` 的兼容入口。

## 已实现代码边界

### 航班源

- `internal/integration/flight.Provider` 是外部航班适配器边界。
- `internal/integration/flight.HTTPJSONProvider` 已实现配置化 HTTP/JSON 拉取：支持 envelope/字段路径、状态映射、时间格式、bearer/API-key/basic 认证和可选 mTLS；凭据通过环境变量解析。
- Worker 已按 `sync.flight_source` 配置启动定时 Poller，按 UTC 窗口拉取 schedule/event，复用 Inbox → Worker 应用链路。
- `POST /internal/integration/v1/flight-source/sync` 快速接收规范化 Schedule/Event 并写入 `flight_source_inbox`。
- `POST /internal/integration/v1/flights/{flightPublicID}/arrival` 是受航班源密钥保护的到达入口。
- `GET /internal/integration/v1/flight-source/health` 返回 `fresh`、`stale`、`fallback`、`failed` 状态及诊断信息。
- Worker 负责租约领取、异步应用、指数退避和失败记录；数据库中的前一晚/最近一次预同步事实可在源不可用时继续读取。
- `flight_source_reconciliation` 持久化每个拉取窗口的 matched/pending/mismatch 对账结果，并可通过健康接口读取摘要；通用 webhook 告警覆盖拉取失败、对账失败和 mismatch。
- 真实 AODB/航司接口的最终 URL、字段映射、凭据和告警网关仍待合同资料与正式环境配置，开发种子不等于真实接入。

### 自动派发

硬条件：人员启用、区域/班组匹配、当前主班组成员、精确岗位、精确能力、`idle`、计划时间无活动 Assignment 冲突。排序：`last_state_changed_at ASC`，再按人员 public ID 升序。

自动派发复用 Core Assignment 确认事务；候选并发失效时继续尝试剩余候选，全部失败则保留缺员事实。收件超时默认 300 秒，释放未收件 Assignment 后按同一规则重派。

### 任务变更

- `GET/POST /api/v1/task-change-requests` 和 `PATCH /api/v1/task-change-requests/{publicID}/review` 已提供申请/审核边界。
- 动作：`pause`、`reassign`、`reschedule`、`cancel`、`resume`；`reschedule` 必须有目标计划时间。
- 员工和队长可上报；值班经理或管理员审批和应用；分管领导只读。
- 审批重新校验 Task 版本、当前状态、人员资格和时间冲突，并在 Core 一个事务内更新 Task、Assignment、Personnel、History、Audit、Outbox。应用失败保留 `failed`。

### 角色

| 角色 | 后端职责 |
| --- | --- |
| `admin` | 全局主数据、受控变更审批、诊断 |
| `manager` | 授权范围内运行查看、补派/确认、变更审批 |
| `leader` | 授权区域/班组查看和上报，不能审批变更 |
| `supervisor` | 授权范围内只读查看和实时订阅 |
| `staff` | 本人任务收件、开始、完成和异常上报 |

管理 SSE `GET /api/v1/realtime/management` 向 `admin/manager/leader/supervisor` 按 Scope 提供短时提示；员工 Socket 只提供本人任务变化提示。断线后使用 HTTP 快照/分页查询恢复。

## 主数据和查询

- 岗位、能力为独立、分页 CRUD；`code` 创建后不可修改，删除为安全停用；人员和模板只能绑定一个岗位和一个主能力。
- 员工工号由服务端校验为 4–12 位数字并保持唯一；管理端可以新增、编辑、启停用和重置密码。
- Core 管理集合和 Edge 历史/通知使用 `page/page_size/total`；员工任务接口保留本人完整 Projection 快照作为恢复合同。

## 当前未完成

- 真实 AODB/航司合同落地后的具体字段映射、生产凭据、正式定时配置、告警/平台通知和生产 HTTPS 域名。
- 正式员工/管理员主数据预置、真实个人微信/企业微信联调和管理端 SSO 上线配置。
- 全部当前 migration 的正式部署验收，以及多副本、网络分区、重启恢复专项门禁。
- API/Gateway/Worker 运行时的完整隔离验收，以及对账/告警链路在运行时的 Probe 验收；迁移专用隔离项目已完成 Core `000010–000014`、Edge `000007` 应用和 schema 复核。
- 代表性数据规模下的 API、数据库、Worker、Outbox/Inbox、Projection lag p50/p95/p99 基线。

## 验证纪律

任何功能、集成、数据库或 Compose 验证前先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

推荐入口：`powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all`。文档修订阶段不应用 migration、不清理数据库、不停止已有服务；迁移专项和 Docker 资源清理使用独立项目及精确资源名单执行。
