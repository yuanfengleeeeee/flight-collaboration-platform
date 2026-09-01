# 项目长期记忆

> 用途：保存跨会话、跨任务仍然有效的协作约束。当前任务进度和临时错误仍记录在 `memory-bank/progress.md` 或根目录 `HANDOFF.md`。

## 当前基线

- Architecture v2.0 Foundation 已完成并冻结为 `ACCEPTED / FROZEN`。
- 当前处于 Phase 2 Business Implementation：BVS2-01 设计、BVS2-02 Core Migration、BVS2-03 `Flight → Task → Candidate`、BVS2-04 `Leader Confirm`、BVS2-05 Edge Projection/Employee Command 以及 BVS2-06 的 Task Cancel、Compose/恢复验证、JWT/mTLS 和 Task Query 读侧已完成对应代码或验证；当前转入前端 F0 架构冻结。
- Core 是航班、人员、任务、Assignment、状态历史、Audit 和业务状态机的唯一事实源；Edge 只保存最小 Projection、Command、Session 和 Inbox 数据。
- 前端采用前后端分离模式，所有前端运行时代码位于单仓库内独立的 `frontend/` 工作区；首期同时建设管理端和员工端。目录骨架已创建，前端源码和构建配置尚未实现。管理端 `admin-web` 只调用 Core API，采用桌面优先响应式 Web；员工端首期 `employee-miniapp` 只调用 Edge API，采用同时支持个人微信和企业微信入口的微信小程序；`employee-web` 保留为正式可用的员工网页版和备用入口，不改变 Core/Edge 边界。
- 旧 `cmd/server`、旧 `internal/model`、旧业务路由和旧 B3/B4/B5 现场保持 `legacy/paused`，不能作为新业务入口恢复。

## 不可变协作约束

- 新业务必须遵守 Core/Edge 独立数据库、版本化 SQL Migration、Outbox/Inbox、幂等、Retry、RBAC/Scope 和 Human/Machine Principal 边界。
- Handler 不直接调用 GORM；Service 不依赖 Gin；Domain 不依赖 Gin、GORM 或 Redis。
- 关键 Core 业务事实、状态历史、Audit 和 Outbox 必须在同一个 Core 事务内提交。
- 不使用 `tenant_id`、`airport_id` 或多机场切换模型；不擅自引入 Kafka、微服务拆分、真实外部推送或 AI 模型。

## 功能验证前置规则

- 任何功能验证、集成验证、数据库验证或 Compose 验证，先执行：

  ```powershell
  powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
  ```

- 推荐使用统一入口：

  ```powershell
  powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
  ```

- `ensure-docker.ps1` 会检测 Docker Engine；未就绪时启动 Docker Desktop 并等待 Engine 可用。它不会自动启动项目容器、执行 Migration、删除 Volume 或清理数据库。
- Docker CLI 或 Docker Desktop 不可用时，验证必须明确失败并记录原因，不得静默跳过 Docker 前置检查。
- 运行验证前仍须阅读 `memory-bank/architecture.md`、`memory-bank/design-document.md`、`memory-bank/implementation-plan.md` 和 `memory-bank/progress.md`，并保留工作区已有修改。

## 记忆维护规则

- 跨任务仍有效的架构和协作规则更新 `AGENTS.md` 与本文件。
- 里程碑完成情况、测试真实结果和临时阻塞更新 `memory-bank/progress.md`。
- 长会话结束或交接时更新 `HANDOFF.md`；不把未运行的测试写成已通过。

## GitHub 同步规则

- 当用户表达“我要结束当前这个对话了”或语义相近的结束会话意思时，除更新 `HANDOFF.md` 外，自动执行一次 GitHub 同步。
- 同步前检查当前分支、远程、工作区差异和未跟踪文件；只提交已经核对且属于项目的修改，不提交密钥、Token、生产配置、本地数据库/Volume、缓存、临时产物或归属不明的用户文件。
- 触发该规则时，用户已明确授权创建一次 commit 并 push 到当前分支的 `origin`；不得 force push、reset、删除或覆盖本地修改，也不自动创建 Pull Request。
- Push 后核对远程 SHA 和工作区状态。若远端领先、冲突或推送失败，保留现场并报告真实原因，不得把未成功的操作写成已同步。
## Current handoff baseline (2026-09-01)

- BVS2-06 Compose closure and Worker/Edge outage recovery have been physically verified on isolated project `bvs206-closure`; existing `flight-mysql` and `flight-redis` were not changed.
- Core/Edge management entrypoints now require Bearer JWT in normal configuration; development actor headers are explicit non-release compatibility adapters. TLS/mTLS is configurable for Core/Edge servers and Worker sync transport.
- Task CRUD currently means Core list/detail plus Arrival-created tasks and Confirm/Cancel lifecycle operations. Manual `POST /tasks`, arbitrary PATCH and hard delete are intentionally not implemented until the product contract is frozen.

## Current frontend freeze baseline (2026-09-01)

- F0 架构冻结已启动，冻结门槛和关闭条件统一记录在 `frontend/docs/architecture-freeze-gate.md`。
- 前端客户端边界固定为：`admin-web → Core API`、`employee-miniapp → Edge API`、`employee-web → Edge API`；员工小程序同时支持个人微信和企业微信，员工 Web 是正式支持的备用入口。
- Core Task 列表/详情读侧、Arrival 创建和 Confirm/Cancel 基础代码已有；手工 Task `POST/PATCH/硬删除` 不得由前端自行假设。
- 当前进入真实业务前的主要阻塞是双微信/员工 Web 身份映射、客户端 Command 幂等键与状态查询、OpenAPI 对齐、Task 生命周期矩阵、Mock/测试数据、前端工具链和环境域名。
