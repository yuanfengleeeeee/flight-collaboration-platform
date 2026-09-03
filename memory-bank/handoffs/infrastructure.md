# 基础设施任务交接

> 更新时间：2026-09-01
>
> 状态：Foundation 和 BVS2-06 隔离闭环已有验证；生产化与性能基线待推进

## 当前基线

- 本地使用 Docker Compose，包含 Core API、Edge API、Worker、Core MySQL、Edge MySQL；Redis 为可选 profile。
- Core/Edge MySQL 完全独立，分别使用 `migrations/core/mysql` 和 `migrations/edge/mysql`；不共库、不跨库 JOIN、不共享业务账号。
- Worker 连接 Core DB，通过 Edge HTTP Transport 投递和拉取，不直接连接 Edge DB。
- Redis 只能承担 cache、rate limit、短锁、在线状态和临时去重，不能承载可靠消息或业务事实。

## 已验证和限制

- Foundation 及 BVS2-06 已有隔离 Compose、双库 Migration、健康检查、SQL/HTTP/Worker Probe、重复消息、失败重试和 Worker/Edge 中断恢复记录。
- 正常 `docker compose up --build` 的基础镜像元数据请求曾受 Docker Hub `EOF` 影响；使用当前代码的交叉编译二进制和本地缓存运行时镜像完成过隔离验证。
- 不执行 `docker compose down -v`、DROP、TRUNCATE、破坏性 Migration down，不删除或重置既有本地 MySQL/Redis 数据。

## 基础设施性能待办

- 记录 Core/Edge API、MySQL、Worker、Outbox/Inbox 和 Projection 的延迟、堆积、重试和资源指标。
- 评估 Compose/生产环境连接池、HTTP keep-alive、payload、超时、并发、健康检查和资源上限。
- 建立可重复的大数据量、Worker 重启、Edge 不可用、数据库慢查询和恢复时间测试。
- 修复或稳定基础镜像获取、CI 缓存和可重复构建流程；不能用临时 workaround 代替正式构建验收。
- 继续保持 Core/Edge 独立数据库和可靠同步边界，不因为性能引入未经测量的 Redis 队列、Kafka 或微服务拆分。

## 下一步

1. 任何 Compose、数据库或集成验证前先运行 `scripts/ensure-docker.ps1`。
2. 记录标准镜像构建和隔离验证的差异，明确可复现的 CI 环境。
3. 配合后端建立同步延迟、资源使用和故障恢复的性能门禁。
## Current session handoff (2026-09-03)

- `scripts/ensure-docker.ps1 -PassThru` succeeded with Docker Desktop started by the script; the resolved CLI is `C:\Users\yuanfengleeeeee\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`.
- No Compose startup, migration application, volume cleanup, migration down, DROP, TRUNCATE, or existing-service shutdown was performed in this handoff turn.
- The next session should validate Core migration `000005_admin_sso` and management HTTP flows only in an isolated database, then continue the dual-Edge/failure and performance gates.
