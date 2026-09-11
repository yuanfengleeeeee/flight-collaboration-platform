# ADR-002 Core Edge Hybrid Architecture

状态：Accepted

## Context

管理端位于机场/公司内网，移动端需要公网接入；航班、人员和业务状态包含敏感信息，不能把 Core 直接暴露到公网。

## Decision

采用内网 Core 模块化单体 + 云端 Edge 接入服务。Core/Edge API、数据库和 migration 独立。生产优先由 Core Worker 主动连接/拉取 Edge，Transport 预留 HTTPS/mTLS/Machine Identity。

## Consequences

移动端入口与核心网络隔离，Core 可以在 Edge 不可用时继续工作；系统需要处理 Projection 延迟、Command pending 和双向补偿。

## Alternatives

不采用单一公网 API 直连 Core，也不采用 Core/Edge 共库或直接数据库复制；它们会扩大攻击面并模糊数据所有权。
