# ADR-004 Core Edge Sync with Outbox Inbox

状态：Accepted

## Context

Core/Edge 网络可能中断，业务事务不能因 Edge 暂时不可用而失败；消息也可能重复或在 Worker 退出时未完成。

## Decision

采用同库事务内写业务数据、Audit 和 Transactional Outbox；接收方使用 Inbox 唯一约束和幂等消费。Edge Command 先持久化，Core Worker 主动拉取。投递语义为 at-least-once，记录 retry、failed 和错误上下文。

## Consequences

核心写入可靠且可恢复；系统不承诺网络 exactly-once，必须依赖 `event_id`/`command_id` 唯一性和消费端幂等。

## Alternatives

当前不引入 Kafka、RabbitMQ、分布式事务框架或 Redis 可靠队列；这些组件不是 Foundation 的必要前提。
