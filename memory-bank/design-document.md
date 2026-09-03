# Architecture v2.0 设计基线

> 状态：Accepted / Frozen（2026-08-13）
> 本文是已验收的 Architecture Foundation 可执行设计基线。当前已在该基线上完成 BVS2-03 `Flight → Task → Candidate` 首个 Core 业务切片；Leader Confirm、Edge Projection 和员工 Command 仍属于后续切片。

## 产品与边界

产品名称为航空保障智能协同平台，服务单机场。核心目标是让正确的信息在正确时间传递给正确的人。

组织范围只有 `OperationArea -> Team -> Personnel`，并与 `Flight`、`Task`、`Event`、`Rule` 并列。禁止 Tenant/Airport 多租户模型、`tenant_id`、`airport_id`、airport scope 和机场切换。

## 架构决策

仓库是单仓库。内网 Core 是模块化单体，云端 Edge 是移动端接入服务；两者使用完全独立的 MySQL、账号、schema 和 migration。Core 是唯一业务事实源，Edge 只保存最小 Projection 与 Command 数据。

Core 与 Edge 通过版本化 Event/Command Envelope、Transactional Outbox、Inbox、幂等、Retry 和 Failed 状态同步。投递语义是 at-least-once，Core Worker 优先主动拉取 Edge Pending Commands；生产 Transport 预留 HTTPS/mTLS。

## 本轮范围

- Core API、Edge API、Worker、Migration 入口和独立生命周期。
- 双数据库连接、独立 SQL migration、健康检查和本地 Compose。
- Outbox、Inbox、Command Store、Event/Command Envelope、幂等、重试和失败记录。
- Human/Machine Principal、四角色 RBAC、结构化 Data Scope、Authorizer Port 和 Audit 基础设施。
- request_id、trace_id、structured logging、Core/Edge/Sync OpenAPI 骨架。
- 仅测试/开发使用的双向 Architecture Probe 与故障测试。

## 本轮非目标

暂停 B3/B4/B5，不能实现真实航班、任务、人员、事件业务切片。也不实现 Kafka/RabbitMQ、Kubernetes、Service Mesh、分布式事务、复杂 CQRS/Event Sourcing、真实 AI、RFID/UWB、真实平台推送、SSO、完整多实例生产部署。T0-5 只实现本地 Gateway、双 Edge 和 best-effort fan-out 骨架，不等同于生产 HA 验收。

## 设计验收条件

1. Core/Edge 可独立启动并分别提供 live/ready。
2. Core/Edge 数据库和 migration 独立，服务启动不调用 AutoMigrate。
3. Core Transaction 可同库写业务 Probe 数据、Audit 和 Outbox。
4. Edge Inbox 对重复 Event 幂等，Core Inbox 对重复 Command 幂等。
5. Edge 故障、Worker 重启、Redis 故障不破坏 Core 事实和 Outbox。
6. 双向 Probe 与对应集成/故障测试可重复运行。
7. 文档、API 契约和代码入口不宣称当前开发环境已经达到 HA/RPO/RTO 目标。

## 依赖方向

```text
HTTP handler -> Application service -> Domain/Port -> Repository adapter
Core module  -> Core Application Port / shared
Edge        -> Projection/Command store / shared
integration -> stable business Port
```

Handler 不调用 GORM；Service 不依赖 Gin；Domain 不依赖基础设施；模块不直接访问其他模块的表。

## T0 员工任务实时交付设计（2026-09-02）

本节是 Architecture Foundation 冻结后的产品化实施方向；T0-4 已完成员工 WebSocket 提示和客户端接入，T0-5 已完成 Gateway、双 Edge、共享 ticket、租约和 Redis fan-out 代码，但不表示微信订阅消息或生产多实例故障验收已经完成，也不重新打开 Core/Edge 数据所有权边界。完整决策见 [`docs/adr/ADR-009-employee-task-realtime-delivery.md`](../docs/adr/ADR-009-employee-task-realtime-delivery.md) 和 [`docs/adr/ADR-010-gateway-edge-horizontal-scaling.md`](../docs/adr/ADR-010-gateway-edge-horizontal-scaling.md)。

- Edge 不按任务类型拆分；它是统一的员工接入和最小 Projection 服务，扩容时部署相同副本。
- 员工任务的最终读取仍采用 Edge `GET /api/v1/tasks`。前台实时体验采用 WebSocket `task_changed` 提示，后台可接入经用户授权的平台通知；两者都不能替代拉取和恢复。
- Core 事务仍写业务事实、Audit 和 Outbox；Worker 投递到 Edge Inbox，Edge Projection 事务提交后才触发尽力而为的通知。
- 推送允许丢失、重复和乱序；客户端收到提示后重新拉取并按版本校准。断线、推送丢失或 Redis 不可用时，HTTP 拉取必须继续可用。
- T0 顺序为：契约冻结 → 性能/同步基线 → 拉取恢复契约 → Notification Port → 前台 WebSocket → Gateway 与多 Edge/Worker → 后台平台通知 → 故障和容量门禁。
- T0 不默认引入 Kafka、RabbitMQ、NATS、Service Mesh 或按任务拆分微服务；持久 Broker 必须以测量结果和独立 ADR 为前提。
