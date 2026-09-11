# ADR-005 Identity RBAC and Data Scope

状态：Accepted

## Context

平台既服务人员，也需要未来接入 RFID/UWB 网关和同步 Worker。人员身份与机器身份的生命周期、权限和风险不同。

## Decision

区分 Human Principal 与 Machine Principal。Human 使用 `admin`、`manager`、`leader`、`staff` 四个角色，权限采用 `resource:action`，数据范围使用结构化的 global/area/team/assigned/self，不提供任意 SQL Scope Resolver。JWT 仅承载身份/会话声明。

## Consequences

业务授权可测试且不绑定 JWT 或具体 IAM 供应商；资源级 Scope 需要由 Application/Repository 以参数化查询实施。

## Alternatives

不把完整权限列表塞进 JWT，不把设备伪装为 staff，也不提前引入复杂策略语言或多租户授权模型。
