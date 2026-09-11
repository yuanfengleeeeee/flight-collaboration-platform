# ADR-001 Modular Monolith Core

状态：Accepted

## Context

平台包含航班、任务、人员、事件、规则等紧密协作的领域，但当前团队需要先冻结边界和一致性，而不是承担微服务治理成本。

## Decision

Core 采用单进程 Go 模块化单体。领域模块保留清晰依赖边界，跨模块用例由 Application 层编排；模块不拆成独立微服务。

## Consequences

事务、调试和本地部署简单；模块边界必须通过包依赖和接口保持。未来若拆分，只能以稳定 Integration Port 为边界，不以当前架构形式为目的拆分。

## Alternatives

不采用一开始拆分的微服务、Kubernetes 或 Service Mesh；这些方案在当前阶段增加运维和一致性复杂度，不能提升 Foundation 验证价值。
