# ADR-007 High Availability and Disaster Recovery Targets

状态：Accepted

## Context

机场保障系统需要支持 API 扩容、数据库恢复和网络分区，但个人开发环境不应提前建设复杂集群。

## Decision

Core/Edge API 按无状态设计，生产目标为各自至少两个实例和独立 MySQL Primary/Replica；备份目标为 Full Backup + Binlog + PITR。记录 RTO/RPO 目标：API 单实例 ≤1 分钟、数据库节点 ≤10 分钟、核心数据 RPO ≤5 分钟、严重区域故障 ≤60 分钟。当前只实现代码、配置和恢复流程边界。

## Consequences

未来可在负载均衡、Replica 和备份系统之上扩展，不需要改业务状态模型；当前文档中的目标不等同于开发环境已达到。

## Alternatives

不在本轮构建 MySQL Cluster、自动 Failover、Kubernetes 或完整灾备中心。
