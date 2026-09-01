# ADR-006 Database ID and Migration Strategy

状态：Accepted

## Context

内部数据库关系查询需要紧凑主键，跨系统 API 和日志需要不可猜测、可追踪的稳定标识。Core 与 Edge 的 schema 演进也必须独立。

## Decision

内部主键使用 `BIGINT UNSIGNED AUTO_INCREMENT`，对外使用 UUIDv7 `public_id`。Core 与 Edge 使用独立 MySQL 和 `migrations/{core,edge}/mysql` 版本化 SQL；服务启动禁止 AutoMigrate，迁移由 `migrate` 命令显式执行。

## Consequences

API 不暴露内部自增 ID；迁移可审计、可回滚测试并且不依赖运行进程。需要在事件、Command、审计和 Projection 中明确 public ID 映射。

## Alternatives

不使用业务编号作为主键，不使用共享 migration 目录，不用启动时 GORM AutoMigrate 代替生产 schema 管理。
