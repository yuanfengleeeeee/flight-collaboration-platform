# ADR-003 Core Data Ownership and Edge Projection

状态：Accepted

## Context

移动端只需要执行任务和接收通知的最小数据。完整 Core 数据包含航班敏感信息、人员档案和事件明细。

## Decision

Core 是航班、人员、任务、事件、规则、审计和状态机的唯一事实源。Edge 只存 `task_projection`、`notification_projection`、Session、Command、Inbox 和 delivery 数据，按字段白名单投影。

## Consequences

Edge 查询快且数据暴露面小，但投影可能短暂滞后；移动端必须显示同步状态并支持 HTTP 重新读取 Projection，不能把 WebSocket 当事实源。

## Alternatives

不复制完整 Core 表，不让 Edge 自行演化业务状态，也不采用跨库 JOIN 或共享数据库。
