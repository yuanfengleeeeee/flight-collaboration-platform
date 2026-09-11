# ADR-008 External Integration Port Adapter Strategy

状态：Accepted

## Context

企业微信、短信、外部航班数据、RFID/UWB 和 AI 供应商可能变化，业务模块不应直接依赖厂商 SDK。

## Decision

第三方边界统一放在 `internal/integration`。Core Domain/Application 只依赖稳定 Port，例如 `NotificationSender`、`PushSender`、`DeviceObservationPort`、`Predictor` 和 `FlightDataSource`。本轮使用 Noop/InApp/Disabled Adapter，不接入真实生产系统。

## Consequences

未来替换供应商不需要修改 Task、Flight 或 Notification Domain；当前实现保持小而可验证，不创建没有明确需求的接口。

## Alternatives

不把 WeCom/RFID/AI SDK 直接 import 到业务模块，也不提前建设模型仓库、消息中间件或硬件业务流水线。
