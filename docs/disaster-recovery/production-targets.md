# 生产 HA / 灾备目标

本文记录 Architecture v2 的生产目标，不代表当前个人开发环境已经达到这些指标。

## 目标

- Core API ≥2 实例，Edge API ≥2 实例，前置 Load Balancer。
- Core/Edge 各自 MySQL Primary + Replica。
- Full Backup + Binlog + Point-in-Time Recovery。
- Core API 单实例故障 RTO ≤1 分钟。
- Core 数据库节点故障 RTO ≤10 分钟。
- 核心数据 RPO ≤5 分钟。
- 严重区域故障 RTO ≤60 分钟。

## 恢复流程（生产实施时）

1. 确认故障范围并保护最近可用的备份和 binlog。
2. 恢复对应 Core 或 Edge 数据库到隔离恢复节点。
3. 使用 binlog 执行 PITR 到确认的恢复时间点。
4. 先运行对应 schema migration status，再启动 API/Worker。
5. 校验 Outbox/Inbox/Projection 的 pending、retry、failed 记录，执行幂等补偿。
6. 校验 Core 业务事实、Edge 最小投影和审计链路，再切换流量。

当前不建设自动 Failover、Kubernetes、灾备中心或真实 PITR 作业；本阶段只冻结边界和恢复流程。
