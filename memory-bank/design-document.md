# Architecture v2.0 设计基线

> 架构状态：`ACCEPTED / FROZEN`（2026-08-13）
> 当前业务边界：2026-09-07 版本

本文件保留稳定的架构决策；当前业务流程、状态机和验收口径以 [`docs/business-process-v2.md`](../docs/business-process-v2.md) 为准。

## 产品与范围

产品是航空客运地面代理协同平台，只服务单机场。首期覆盖国内/国际值机、自助值机、进出港、中转、配载平衡、特殊旅客、不正常航班、行李地面派送、航班地面保障、客运销售代理和航空信息咨询；维修、客舱及其他非旅客地面代理业务不在范围内。

组织范围固定为 `OperationArea -> Team -> Personnel`，与 Flight、Task、Event、Rule 并列。禁止 Tenant/Airport 多租户模型、`tenant_id`、`airport_id`、多机场切换。

## Core/Edge 架构

- Core 是模块化单体，拥有航班、人员、岗位、能力、人员状态、任务模板/实例/候选/分配、事件、规则、审计和业务状态机。
- Edge 是员工接入服务，只保存最小 Projection、Command、Session、Inbox、通知和 delivery。
- Core/Edge 使用独立 MySQL、账号、schema 和 migration；Edge 不直连 Core DB，Core 不直写 Edge DB。
- Core 业务数据、状态历史、Audit 和 Outbox 在一个事务内提交；Worker 通过可靠 Outbox/Inbox 链路同步到 Edge。
- 同步语义为 at-least-once、幂等、版本收敛、Retry/Failed。Redis 只做缓存、限流、短锁、在线状态和 best-effort fan-out。

## 依赖方向

```text
HTTP handler -> Application service -> Domain/Port -> Repository adapter
Core module  -> Core Application Port / shared
Edge         -> Projection/Command store / shared
integration  -> stable business Port
```

Handler 不调用 GORM；Service 不依赖 Gin；Domain 不依赖 Gin、GORM 或 Redis；跨模块用例由 Core Application 编排。

## 当前业务主干

```text
外部航班源
  -> flight_source_inbox（幂等接收）
  -> Worker 应用航班事实
  -> 航班到达
  -> pending_dispatch
  -> 自动预分配
  -> Core Assignment
  -> Edge Projection
  -> 员工 received -> start -> complete
```

航班事实只能来自外部 Provider。管理端只读航班；开发种子中的航班仅为测试夹具。源状态为 `fresh/stale/fallback/failed`，Provider 失败时保留重试和诊断并读取最近一次预同步事实。

自动调度 v1 过滤启用员工、任务区域/活动主班组、精确岗位、精确能力、`idle` 和同一计划时间冲突；按 `last_state_changed_at`、候选公共 ID 稳定排序。候选快照用于自动初派和收件超时重派。

三层握手为：Core 系统 Assignment 确认、Outbox/Inbox Projection 投影确认、员工 `received` 收件回执。`received` 不是同意，员工没有拒绝动作；员工冲突通过异常/任务变更申请报告。

## 角色与任务变更

- `admin`：系统、身份、字典、审计和全局管理。
- `manager`：授权范围内运行管理、自动派发补派、任务变更审批。
- `leader`：团队/区域 Scope 内查看、报告和跟进，不能审批变更。
- `supervisor`：授权范围内只读观察和管理实时订阅。
- `staff`：本人任务的收件、开始、完成和异常报告。

异常/任务变更支持 `pause`、`reassign`、`reschedule`、`cancel`、`resume`。员工、leader 或外部来源只能提出报告；只有 manager/admin 审批并应用。审批事务重新校验版本、状态、人员资格和时间冲突，并写 History/Audit/Outbox。

## 身份与前端

管理端 `admin-web` 只调用 Core；`employee-web`、个人微信小程序、企业微信小程序只调用 Edge。个人微信和企业微信身份都映射到同一个 Core Staff，平台标识不能直接作为业务身份。

员工工号由服务端校验为固定长度纯数字字符串；人员通常绑定一个岗位和一个能力，岗位/能力编码创建后不可修改，字典删除为安全停用。

真实微信/企业微信 Provider、管理端 SSO、HTTPS/回调域名、管理员预置和平台通知是部署验收事项；`mock:<subject>` 只用于开发。

## 实时与读取

管理端使用按 Scope 过滤的短时 SSE；员工端 WebSocket/原生 socket 只作 `task_changed` 提示。断线、重启或丢失提示时，管理端重新读取分页 API，员工端重新读取完整本人快照。

Core 管理集合、Edge 历史和通知使用 `page/page_size/total` 与稳定排序；员工任务完整快照是恢复机制，不是管理端无界全量查询。

## 验收与非目标

代码/契约已覆盖自动派发、三层握手、收件超时重派、变更审批、航班源 inbox/fallback、字典 CRUD、管理订阅和集合分页。正式验收仍需真实 Provider、平台凭据/域名、告警渠道、多副本故障、性能基线以及接口失效、重复消息、断线、延误、取消、无候选和审批失败场景。

不引入微服务拆分、Kafka/RabbitMQ、Kubernetes、Service Mesh、分布式事务、真实 AI/硬件、多租户或多机场模型。

## 验证规则

任何功能、数据库或 Compose 验证前先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

统一入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

未实际运行的检查不得标记为通过；不得默认执行清库、`TRUNCATE`、migration down 或 `docker compose down -v`。
