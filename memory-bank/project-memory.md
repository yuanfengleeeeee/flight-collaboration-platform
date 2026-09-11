# 项目长期记忆

> 本文件只保存跨会话仍有效的架构、业务边界和协作规则。阶段进度、临时错误和验证结果写入 [`progress.md`](progress.md) 或根目录 [`HANDOFF.md`](../HANDOFF.md)。

## 当前基线

- Architecture v2.0 Foundation 已 `ACCEPTED / FROZEN`；业务实现采用 Core 模块化单体 + Edge 接入服务，不拆 flight/task/personnel/event/rule 微服务。
- 系统只服务单机场，禁止 `tenant_id`、`airport_id`、多机场切换和机场 Scope。
- Core 是航班、人员、岗位、能力、人员状态、任务模板/实例/候选/分配、事件、规则、审计和状态机的唯一事实源；Edge 只保存员工端最小 Projection、Command、Session、Inbox、通知和 delivery 数据。
- Core/Edge 使用完全独立的 MySQL、账号、schema 和 SQL migration；服务启动不得调用 GORM `AutoMigrate`；Edge 不得直连 Core 数据库。
- 当前业务主干为：外部航班同步 → `pending_dispatch` → 自动预分配 → Core/Edge/员工三层握手 → 员工执行。员工 `received` 是收件回执，不是同意；员工没有拒绝命令。
- 员工冲突、延误、取消和突发事件通过异常/任务变更申请提出；只有 `manager` 或 `admin` 审批并应用 `pause/reassign/reschedule/cancel/resume`，`leader` 报告并跟进，`supervisor` 只读观察。
- 航班事实只能来自外部 Provider。源记录先写入 Core `flight_source_inbox`，再由 Worker 异步应用；`fresh/stale/fallback/failed` 表示来源健康。开发种子航班仅用于夹具，不能作为生产手工写入方案。
- 当前自动派发 v1 按启用状态、区域/活动主班组、精确岗位、精确能力、`idle`、时间冲突过滤，按 `last_state_changed_at` 和候选公共 ID 稳定排序；超时重派默认 300 秒。

## 客户端与身份边界

- `frontend/apps/admin-web` 只调用 Core API；`employee-web`、`employee-miniapp`、`employee-wecom-miniapp` 只调用 Edge API。
- 个人微信、企业微信和员工 Web 都映射到同一个 Core `Staff`；小程序只使用服务端签发的 Edge Session，不把 openid/unionid/企业微信用户标识当业务身份。
- 员工首次使用工号 + 密码认证；工号由服务端校验为固定长度数字字符串并唯一。人员通常绑定一个岗位和一个能力，岗位/能力编码创建后不可修改，字典删除采用安全停用。
- Core 管理角色为 `admin`、`manager`、`leader`、`supervisor`；员工角色为 `staff`。权限由后端 RBAC + Scope 校验，JWT 不承载完整权限列表。
- 真实微信/企业微信 Provider、管理端 SSO、正式凭据、域名和管理员预置是部署/验收事项；`mock:<subject>` 仅用于开发验收。

## 可靠同步与实时性

- Core 事实、状态历史、Audit 和 Outbox 必须在同一个事务内提交。
- Core Outbox → Worker → Edge Inbox/Projection 是可靠链路，必须支持 at-least-once、幂等、版本控制、Retry/Failed 和断线恢复。
- 员工 Command 使用稳定 `command_id`；HTTP `202` 只表示已接收，最终状态必须等待 Core 事件回投并从 Projection 读取。员工可通过公开 Command status 接口恢复重试。
- 管理端使用按 Scope 的短时 SSE 提示流，员工端使用 WebSocket/原生 socket `task_changed` 提示；实时提示可以丢失、重复、乱序，客户端收到后重新读取分页/完整快照校准。
- Redis 只负责缓存、限流、短锁、在线状态和 best-effort fan-out，不承担可靠消息存储。

## 性能与数据读取

- Core 管理集合、Edge 历史和通知必须使用服务端分页、边界校验、稳定排序和 `page/page_size/total`；禁止前端一次性拉取无界数据再过滤。
- 员工任务快照按员工隔离保留完整集合，用于启动、刷新、断线和离线恢复；这不等同于管理端全量查询。
- 后端性能优化前先测量 API p95/p99、数据库查询、Worker 延迟、Outbox/Inbox 堆积和 Projection lag，不得为了吞吐削弱可靠同步。
- 前端动效只服务反馈、状态或空间关系；默认使用短时 `transform`/`opacity`，支持 reduced motion，禁止高成本持续特效和全量列表 stagger。

## 不可变协作约束

- Handler 不直接调用 GORM；Service 不依赖 Gin；Domain 不依赖 Gin、GORM 或 Redis。
- 所有写操作必须经过后端权限、状态机、版本和事务校验；前端、脚本或 AI 不得直接修改 Core 事实。
- 不擅自引入 Kafka、微服务拆分、真实外部推送、AI 模型或新的事实数据库。
- 旧 `cmd/server`、旧 `internal/model`、旧业务路由和旧 B3/B4/B5 现场保持 `legacy/paused`，不能作为 v2 入口恢复。
- 源码、JSON/HTTP、WebSocket、小程序请求和 MySQL 文本统一使用 UTF-8/`utf8mb4`；日志不得记录密码、完整 JWT、密钥或其他敏感凭据。

## 验证前置规则

任何功能、集成、数据库或 Compose 验证前必须执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

推荐统一入口：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify.ps1 -Mode all
```

Docker CLI/Desktop 不可用时必须让验证明确失败并记录原因，不得静默跳过。未实际运行的测试或 Compose 场景不得写成“已通过”。

## 交接与 Git 规则

- 当前进度写入 `memory-bank/progress.md`；前端、后端和基础设施滚动快照分别写入 `memory-bank/handoffs/frontend.md`、`backend.md`、`infrastructure.md`，根索引写入 `HANDOFF.md`。
- 修改文档或代码前检查 `git status`、`git diff` 和未跟踪文件，保留其他任务的现场；修改 Go 后运行 `gofmt`，并检查 `git diff --check`。
- 未经用户明确结束会话，不提交、push、切换/删除分支，不执行破坏性 migration、清库、`TRUNCATE`、`down -v` 或修改用户已有服务。

## 权威文档索引

- 当前架构：[`architecture.md`](architecture.md)
- 当前业务流程：[`docs/business-process-v2.md`](../docs/business-process-v2.md)
- 当前代码导览：[`docs/code-tour-v2.md`](../docs/code-tour-v2.md)
- 任务合同：[`docs/task-crud-contract-v2.md`](../docs/task-crud-contract-v2.md)
- 前后端交接：[`docs/frontend-backend-handoff.md`](../docs/frontend-backend-handoff.md)
- 前端冻结门槛：[`frontend/docs/architecture-freeze-gate.md`](../frontend/docs/architecture-freeze-gate.md)
