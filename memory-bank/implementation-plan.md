# Architecture v2.0 实施计划

> Architecture Foundation 已 `ACCEPTED / FROZEN`。本文只保留当前实施计划；历史阶段和过时的 Leader Confirm → Employee Accept 草案已删除。实际完成证据与命令结果见 [`progress.md`](progress.md)。

## 1. 已完成的基础层

- [x] 仓库审计、架构文档和 Core/Edge 边界冻结。
- [x] `cmd/core-api`、`cmd/edge-api`、`cmd/worker`、`cmd/migrate` 入口及 `internal/core`、`internal/edge`、`internal/integration`、`internal/platform`、`internal/shared` 目录建立。
- [x] Core/Edge 独立 MySQL 配置、版本化 SQL migration、健康检查、配置、日志、request ID、JWT、RBAC/Scope 基础完成。
- [x] Core Outbox、Edge Inbox/Projection、Edge Command、Retry/Failed、幂等和 Worker Transport 基础完成。
- [x] 基础 Probe、重复消息、Worker 重启、Edge 不可用恢复和 Docker 验证入口完成；真实运行结果不在本文件重复宣称，以 `progress.md` 为准。

## 2. 当前业务实现状态

| 能力 | 状态 | 当前合同 |
| --- | --- | --- |
| 航班源同步 | `IMPLEMENTED / CONTRACT_PENDING` | 已有配置化 HTTP/JSON Provider、凭据环境变量、定时拉取、字段/状态映射、Inbox 应用、对账报告和 webhook 告警；仍需按真实 AODB/航司合同填入最终配置并联调 |
| 航班事实 | `IMPLEMENTED / EXTERNAL_ONLY` | 管理端只读；开发种子航班仅为夹具，不提供生产手工航班 CRUD |
| 到达生成任务 | `IMPLEMENTED` | 到达按模板生成 `pending_dispatch`，使用生成幂等键 |
| 自动预分配 | `IMPLEMENTED / RULES_FROZEN_V1` | 按启用、区域/主班组、岗位、能力、idle、时间冲突过滤；稳定排序并记录候选快照 |
| 三层握手 | `IMPLEMENTED` | 系统确认、Outbox/Inbox 投影确认、员工 `received` 收件回执 |
| 员工执行 | `IMPLEMENTED` | `received` → `start` → `complete`；员工没有拒绝任务命令 |
| 收件超时重派 | `IMPLEMENTED / ACCEPTANCE_PENDING` | 默认 300 秒，锁定复核、释放旧预留、按同一规则尝试替补 |
| 任务变更 | `IMPLEMENTED / ACCEPTANCE_PENDING` | 员工/leader 报告，manager/admin 审批 pause/reassign/reschedule/cancel/resume |
| 岗位/能力字典 | `IMPLEMENTED` | 独立分页 CRUD；编码不可变，删除为安全停用；人员通常绑定一个岗位和一个能力 |
| 管理实时订阅 | `IMPLEMENTED / ACCEPTANCE_PENDING` | manager/leader/supervisor/admin 按 Scope 收到 SSE 提示，断线后分页读取恢复 |
| 管理与 Edge 分页 | `IMPLEMENTED` | 管理集合、Edge 历史和通知使用 page/page_size/total；员工任务为按员工完整快照 |

详细业务流程见 [`docs/business-process-v2.md`](../docs/business-process-v2.md)，代码入口见 [`docs/code-tour-v2.md`](../docs/code-tour-v2.md)。

## 3. 当前代码边界

### Core

- `internal/core/application/flighttask`：航班到达、任务查询、任务生命周期。
- `internal/core/application/flightsync`：批量航班源接收、健康诊断和 inbox 应用配合。
- `internal/core/application/taskchange`：任务变更申请、审批和事务应用。
- `internal/core/application/management`：人员、组织、岗位/能力字典、模板和管理查询。
- `internal/core/application/managementrealtime`：Scope 过滤的管理 SSE。

### Edge 与 Worker

- `internal/edge/application/server.go`：员工认证、任务快照、received/start/complete、异常和 Command status。
- `internal/edge/sync`：Projection/Command/Inbox 持久化、版本收敛和幂等。
- `internal/edge/application/realtime`：浏览器 WebSocket 和原生 socket 提示。
- `internal/integration/flight`：外部航班 Provider 适配边界。
- `internal/integration/sync`：Core/Edge Transport、Worker、Retry 和 lease。

## 4. 待完成实施顺序

### 开发环境容器化基线

本地开发/测试容器生命周期已统一：`flight-dev` 固定复用日常开发容器和 Core/Edge named Volume，`flight-test` 固定复用独立测试 Volume，危险/版本测试通过 `flight-danger-*` 创建全新项目和 Volume。`app` 容器内打包前后端运行进程、Gateway 及两个 Web 前端；MySQL/Redis 仍为带 Volume 的独立基础服务。脚本会显式执行 migration 并调用随机种子扩充开发/测试数据。

### P1：外部航班接口资料落地

配置化 HTTP/JSON Provider、认证环境变量、定时拉取、持久化对账报告和通用 webhook 告警已落地。收到 AODB/航空公司接口资料后，还需替换占位字段映射、注入正式凭据、配置告警渠道并完成 Provider 联调。Provider 失败时必须继续使用已预同步事实，并能观察 retry/fallback/failed。

### P2：隔离环境验收

在不触碰用户已有数据库和服务的前提下，应用 Core/Edge 待部署 migration，验证：

- 航班接口失效、重试、告警状态和预存数据 fallback；
- 重复/乱序消息、Worker/Edge 断线、Projection 延迟和恢复；
- 自动派发缺员、候选冲突、收件超时、替补和无替补；
- 航班延误、取消、员工异常报告、经理审批、审批失败和版本冲突。

### P3：平台身份与发布

完成真实个人微信/企业微信 Provider、HTTPS 域名、管理端 SSO、管理员预置、员工导入、平台通知 Provider 和四客户端隔离环境联调。Mock Provider 不作为生产验收证据。

### P4：容量与故障门禁

完成多副本、网络分区、服务重启、Redis 不可用和实时提示丢失测试；形成 API/DB/Worker/Outbox/Inbox/Projection 的 p50/p95/p99 与堆积报告。实时提示不能替代可靠同步和快照恢复。

## 5. 明确不做

- 不恢复通用手工 `POST /tasks`、任意 PATCH 或硬删除。
- 不允许员工通过“接受/拒绝”决定是否执行；`received` 只确认收件。
- 不允许前端直接修改 Core 事实、伪造员工身份或调用内部同步接口。
- 不把 Redis、WebSocket、平台通知、浏览器缓存或开发种子当作可靠业务事实。
- 不在规则和调度算法未经业务评审前擅自引入班次、负载、休息或跨航班优化。

## 6. 验证入口

任何功能、数据库或 Compose 验证前先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

统一验证：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

修改 Go 后运行 `gofmt`；所有修改后检查 `git diff --check` 和 `git status`。未实际运行的检查不得标为通过，破坏性迁移、清库和 `docker compose down -v` 不得默认执行。
