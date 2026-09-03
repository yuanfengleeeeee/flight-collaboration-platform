# 前后端交接文档

> 更新时间：2026-09-01
> 项目：航空保障智能协同平台
> 当前基线：Architecture v2.0 `ACCEPTED / FROZEN`；BVS2-03/04/05 已完成，BVS2-06 已完成 Task Cancel、Compose/恢复验证、JWT/mTLS 接入和 Task Query 读侧闭环；前端 F0 架构冻结进行中
> 文档状态：基于当前工作区代码、路由、迁移和测试结果的交接草案。本文保留历史章节；当前前端可用边界和未决合同以文末校正、`memory-bank/` 最新记录和实际代码为准。

> **当前状态校正（2026-09-01）**：以上早期描述已由本文件文末的“当前前端 F0 冻结校正”补充。按当前代码和最新 `memory-bank/` 记录，BVS2-04 `Leader Confirm`、BVS2-05 `Edge Projection → Employee Command`、BVS2-06 的 Task Cancel、Compose/恢复验证、JWT/mTLS 和 Task Query 读侧已完成对应代码或验证；以实际注册路由、测试结果和最新 `memory-bank/architecture.md` 为准。

## 1. 文档说明

### 1.1 目的与适用范围

本文面向第一次接触仓库的前端、低代码和后端开发人员，说明当前真正可以调用的服务、接口、数据边界和同步语义，并为下一阶段任务业务提供明确标记为“建议/尚未实现”的接口契约。

本文覆盖 Architecture v2 的 core-api、edge-api、worker、migrate、Core/Edge 独立 MySQL、同步协议和旧原型现场。仓库没有名为 legacy/paused/ 的实际目录；“legacy/paused”是对旧目录和旧代码状态的标记。

### 1.2 当前版本与文档优先级

- 工程架构版本：Architecture v2.0，状态 ACCEPTED / FROZEN。
- 业务阶段：Phase 2；BVS2-01 业务设计 FROZEN，BVS2-02 Core 迁移 COMPLETED，BVS2-03 Flight → Task → Candidate COMPLETED，下一项为 BVS2-04 Leader Confirm。
- API 描述文件的 info.version 当前为 1.0.0；仓库没有单独的产品 Release 版本号。
- Go module 声明版本为 Go 1.25.1，实际运行环境以本机 go version 为准。

若本文、OpenAPI、设计文档和代码不一致，按以下顺序处理：实际代码和注册路由；已应用/待应用的 Core/Edge SQL migration；实际测试和联调结果；已接受的架构/业务设计；本文和其他说明文档。本文中的“建议/尚未实现”“TODO”“待确认”不能作为现有接口使用。

## 2. 系统总体架构

### 2.1 组件关系

~~~text
前端/低代码管理端
        ↓
Core API
        ↓
Core MySQL
        ↓
Outbox
        ↓
Worker
        ↓
Edge API / Edge MySQL
        ↓
员工端或移动端
~~~

Edge → Core 方向为：员工端调用 Edge API，Edge 将 Command 持久化到 Edge MySQL，Core 侧 Worker 主动拉取，再交给 Core Application 做权限、状态和事务校验。

### 2.2 组件职责和数据库边界

| 组件 | 实际职责 | 当前边界 |
|---|---|---|
| Core API | 内网管理端和 Core 业务事实入口；当前真实业务入口是航班到达处理 | 只连接 Core DB；业务路由使用 Bearer JWT/RBAC，管理 SSO 通过 Core 管理会话解析，默认配置仍关闭 |
| Edge API | 公网/移动端入口；读取 Edge Projection，持久化移动端 Command，提供内部同步 HTTP 接口 | 只连接 Edge DB；当前业务路由仍是 Foundation/Probe 级别 |
| Worker | Core Outbox 投递到 Edge；主动拉取 Edge Pending Command；处理重试、失败和确认 | 只连接 Core DB，通过 Transport 访问 Edge，不连接 Edge DB |
| Core MySQL | 航班、人员、岗位、能力、任务、分配、事件、规则、审计和状态机的唯一事实库 | 由 migrations/core/mysql 管理 |
| Edge MySQL | 移动端最小 Projection、Command、Session、Inbox、delivery 等数据 | 由 migrations/edge/mysql 管理 |

Core 和 Edge 使用独立实例、账号、schema 和 migration，禁止共库、跨库 JOIN、Edge 直连 Core DB、Core API 直接写 Edge DB。这样可避免移动端暴露完整 Core 事实，并让同步失败进入重试而不是回滚 Core 业务。

### 2.3 Outbox、Inbox、Projection、Command

- **Outbox**：Core 业务事务中同时写入的待投递事件。业务提交后由 Worker 异步发送。
- **Inbox**：接收方对已见消息的持久化记录。Edge 的 sync_inbox 按 event_id 去重，Core 的 core_inbox 按 command_id 去重。
- **Projection**：面向查询的最小数据副本，如 Edge 的 task_projection。它不是业务事实源，丢失后应可从 Core 事件重建。
- **Command**：移动端请求 Core 执行某个动作的持久化意图。Edge 先保存，Core 处理后才能改变最终业务状态。

BVS2-03/04 已在 Core 事务中生成 Task/Candidate/Assignment 以及 `task.assigned.v1`；BVS2-05 已接入员工可执行的 Edge Projection 和 Accept/Complete Command 链路，最终业务状态仍以 Core Event 为准。

### 2.4 Redis

Redis 只允许承担 cache、rate limit、短期锁、短期去重、在线状态和临时状态。可靠业务事实、Outbox、Inbox 和 Command 不能只存 Redis。Redis 不可用时，Core 事务和可靠同步仍应工作；当前 v2 readiness 将 Redis 作为 optional 依赖。

## 3. 项目目录与模块职责

| 目录/文件 | 用途 |
|---|---|
| [cmd/core-api/main.go](../cmd/core-api/main.go) | 加载 v2 配置，连接 Core DB/可选 Redis，装配 BVS2-03 到达服务和 Core Server。 |
| [cmd/edge-api/main.go](../cmd/edge-api/main.go) | 只连接 Edge DB/可选 Redis，装配 Projection/Command Store 和 Edge Server。 |
| [cmd/worker/main.go](../cmd/worker/main.go) | 连接 Core Store，创建 Edge HTTP Transport，周期执行 Outbox 投递和 Command 拉取。 |
| [cmd/migrate/main.go](../cmd/migrate/main.go) | 按 target core 或 edge 独立执行 SQL migration；启动时不隐式迁移。 |
| [internal/core/application](../internal/core/application) | Core Application 编排层；包含 Foundation Command 处理和 BVS2-03 入口。 |
| [internal/core/application/flighttask](../internal/core/application/flighttask) | Flight arrival → Task → Candidate 用例、HTTP 翻译和稳定错误码。 |
| [internal/core/adapter/mysql](../internal/core/adapter/mysql) | Core-only GORM Repository、事务映射和 Core 业务表读写。 |
| [internal/core/module](../internal/core/module) | Flight、Personnel、Task、IAM 等领域模型、状态和值对象。 |
| [internal/core/sync](../internal/core/sync) | Core Outbox、Core Inbox、事务接口和重试/失败状态。 |
| [internal/edge/application](../internal/edge/application) | Edge 路由、健康检查、Projection 查询、Command 落库和同步 HTTP 接口。 |
| [internal/edge/sync](../internal/edge/sync) | Edge Inbox、Task Projection、Mobile Command Store 及 SQL/Memory 实现。 |
| [internal/integration/sync](../internal/integration/sync) | Core↔Edge Transport Port、HTTP/Memory Transport、Worker 和 RetryPolicy。 |
| [internal/platform](../internal/platform) | 配置、MySQL、Redis、健康检查、日志、request/trace、JWT 和安全基础设施。 |
| [internal/shared](../internal/shared) | 版本化 Event/Command Envelope、公共 ID 和共享基础类型。 |
| [migrations/core/mysql](../migrations/core/mysql) | Core Foundation 000001 和 BVS2-02 业务事实 migration 000002。 |
| [migrations/edge/mysql](../migrations/edge/mysql) | Edge Foundation 000001，包含 Projection、Command、Inbox 等最小表。 |
| [frontend/README.md](../frontend/README.md) | 前端工作区边界和目录骨架说明；当前只有占位文件，没有可运行前端。 |
| [frontend/apps/admin-web](../frontend/apps/admin-web) | 管理端应用目录；只调用 Core API，当前尚未实现页面和构建配置。 |
| [frontend/apps/employee-miniapp](../frontend/apps/employee-miniapp) | 首期员工微信小程序目录；个人微信和企业微信共用，F1 创建源码和构建配置，只调用 Edge API。 |
| [frontend/apps/employee-web](../frontend/apps/employee-web) | 正式员工网页版和备用入口目录；只调用 Edge API，当前尚未实现页面和构建配置。 |
| [frontend/packages](../frontend/packages) | contracts、api-client、auth、ui、task-domain、mock 共享包目录；当前尚未实现源码。 |
| [docs/performance-and-reliability-baseline.md](performance-and-reliability-baseline.md) | 性能优先级、前端动效限制、性能门禁和后端优化待办；当前指标为待验证目标。 |
| [cmd/server](../cmd/server) | 旧单体入口，legacy/paused，不是 v2 生产入口。 |
| [internal/server](../internal/server)、[internal/model](../internal/model)、[internal/store](../internal/store)、[internal/module](../internal/module) | 旧原型路由、模型、存储和 B3 现场，legacy/paused。 |
| [migrations/mysql](../migrations/mysql) | 旧单库 migration，legacy/paused；不得作为 v2 migration 或服务启动依据。 |

## 4. 当前已经实现的 API

### 4.1 服务地址和通用约定

默认本地配置来自 [configs/config.v2.yaml](../configs/config.v2.yaml)：

- Core API：http://127.0.0.1:8081
- Edge API：http://127.0.0.1:8082
- Worker：无 HTTP 业务端口
- Core MySQL：宿主机 127.0.0.1:3310
- Edge MySQL：宿主机 127.0.0.1:3311

Core/Edge v2 都接收可选 X-Request-ID、X-Trace-ID，缺失或超过 128 字符时生成新值，并在响应头返回；请求日志包含这两个值。当前 v2 路由没有统一登录中间件。Authorization: Bearer 只有旧 cmd/server 的认证路由实际使用。

### 4.2 v2 实际路由总表

| 服务 | HTTP 方法 | 路径 | 用途 | 当前状态 | 前端是否可以使用 |
|---|---|---|---|---|---|
| Core | GET | /health/live | 进程存活检查 | 已实现 | 可以用于健康探测 |
| Core | GET | /health/ready | 检查 Core MySQL；Redis optional | 已实现 | 可以用于健康探测 |
| Core | GET | /api/v1/foundation | 返回架构和 business_mode | Foundation 诊断 | 仅开发/联调 |
| Core | POST | /api/v1/flights/{flightPublicID}/arrival | 记录到达并生成 Task/Candidate | BVS2-03 已实现 | 后端/内部联调可用 |
| Edge | GET | /health/live | 进程存活检查 | 已实现 | 可以用于健康探测 |
| Edge | GET | /health/ready | 检查 Edge MySQL；Redis optional | 已实现 | 可以用于健康探测 |
| Edge | GET | /api/v1/foundation | 返回 projection-only 模式 | Foundation 诊断 | 仅开发/联调 |
| Edge | GET | /api/v1/tasks | 读取 Edge task_projection | Foundation 查询 | 可用于 Projection/Probe |
| Edge | POST | /api/v1/commands | 保存版本化 Command，返回 pending | Foundation/业务 Command Store | 已实现；业务路由见下两行 |
| Edge | POST | /api/v1/tasks/{taskPublicID}/accept | 保存员工 Accept Command，返回 pending | BVS2-05 | 已实现；Core 异步校验 |
| Edge | POST | /api/v1/tasks/{taskPublicID}/complete | 保存员工 Complete Command，返回 pending | BVS2-05 | 已实现；Core 异步校验 |
| Edge | POST | /internal/sync/v1/events | Worker 投递 Event 到 Edge Inbox | 内部同步 | 前端不可调用 |
| Edge | GET | /internal/sync/v1/commands/pending | Worker 拉取 Pending Command | 内部同步 | 前端不可调用 |
| Edge | POST | /internal/sync/v1/commands/{commandID}/ack | Worker 确认/重试/失败 Command | 内部同步 | 前端不可调用 |

### 4.3 Core：记录航班到达

实现位置：[flighttask handler](../internal/core/application/flighttask/handler.go)、[flighttask service](../internal/core/application/flighttask/service.go)。

~~~http
POST http://127.0.0.1:8081/api/v1/flights/{flightPublicID}/arrival
Content-Type: application/json
X-Request-ID: optional-request-id
X-Trace-ID: optional-trace-id
X-Actor-Public-ID: optional-machine-id
~~~

请求体：

~~~json
{
  "source_event_id": "source-event-001",
  "occurred_at": "2026-08-31T08:00:00Z",
  "actual_arrival_at": "2026-08-31T08:02:00Z",
  "source": "manual"
}
~~~

occurred_at 和 actual_arrival_at 必填；source_event_id 可选，缺失时由 Core 生成稳定业务幂等键；source 缺失默认为 manual。Path 在代码中只校验非空，不实际校验 UUID。当前不需要登录，也没有权限校验；Handler 将 ActorType 固定为 machine，X-Actor-Public-ID 只作为审计 Actor 值来源。

首次生成任务成功响应为 HTTP 201：

~~~json
{
  "data": {
    "result_code": "created",
    "duplicate": false,
    "flight_public_id": "018f0000-0000-7000-8000-000000000001",
    "flight_status": "arrived",
    "task_public_id": "018f0000-0000-7000-8000-000000000002",
    "task_status": "awaiting_confirmation",
    "generation_key": "018f0000-0000-7000-8000-000000000001:flight_arrived",
    "candidate_count": 1,
    "candidates": [
      {
        "public_id": "candidate-public-id",
        "personnel_public_id": "personnel-public-id",
        "rank": 1,
        "position_code": "ramp",
        "capabilities": ["ramp", "radio"],
        "work_state": "idle"
      }
    ]
  },
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

当前 result_code：

| code | 含义 | HTTP |
|---|---|---|
| created | Flight 变为 arrived，生成 awaiting_confirmation Task 和 Candidate 快照 | 201 |
| candidate_shortage | Task 已生成，但没有合格 Candidate | 201 |
| no_active_template | Flight 到达事实已保存，但没有启用模板，不创建 Task | 201 |
| already_processed | 同一 generation key 已有 Task，返回已有结果 | 当前 duplicate=false 时仍可能 201 |
| flight_status_conflict | departed/cancelled 不能逆向到达 | 409 |

错误响应：

~~~json
{
  "code": "source_event_id_conflict",
  "message": "source event was already used with different content",
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

稳定错误码为：400 的 invalid_input/template_invalid；404 的 flight_not_found；409 的 source_event_id_conflict/flight_status_conflict；未知错误为 500 internal_error。同一 source ID 且规范化内容相同返回 200、duplicate=true 和第一次结果；同一 ID 内容不同返回 409，不修改业务事实。

Flight、状态历史、Task、Task 状态历史、Candidate、业务幂等记录、Audit 和 task.generated.v1 Outbox 在同一个 Core 事务中完成。请求不等待 Worker/Edge，所以 HTTP 成功不代表 Edge 已同步。

### 4.4 Edge：读取员工 Task Projection

~~~http
GET http://127.0.0.1:8082/api/v1/tasks
X-Employee-Public-ID: optional-employee-public-id
~~~

Header 存在时按该值过滤；缺失时当前实现返回全部 Projection。它不是身份认证，当前没有登录、权限或 self scope 校验。

成功响应：

~~~json
{
  "items": [
    {
      "public_id": "task-public-id",
      "employee_public_id": "employee-public-id",
      "flight_display_no": "CA1234",
      "task_name": "到达保障",
      "area_name": "到达区",
      "planned_at": "2026-08-31T08:15:00Z",
      "status": "assigned",
      "message": "请前往到达区执行保障任务",
      "sync_version": 1,
      "updated_at": "2026-08-31T08:03:00Z"
    }
  ],
  "source": "edge_projection",
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

没有数据时 items 为空数组。Store 不可用返回 503 projection_unavailable；读取失败返回 500 projection_read_failed。错误 JSON body 当前未带 request/trace ID，但响应头仍有。

### 4.5 Edge：保存 Foundation Command

~~~http
POST http://127.0.0.1:8082/api/v1/commands
Content-Type: application/json
~~~

请求体是完整 CommandEnvelope，必须有版本化 command_type、正数 schema_version、actor_public_id、aggregate_id、时间、trace_id 和合法 JSON Payload。Foundation Probe 示例：

~~~json
{
  "command_id": "018f0000-0000-7000-8000-000000000010",
  "command_type": "probe.complete.v1",
  "schema_version": 1,
  "actor_public_id": "018f0000-0000-7000-8000-000000000011",
  "aggregate_id": "018f0000-0000-7000-8000-000000000012",
  "occurred_at": "2026-08-31T08:05:00Z",
  "trace_id": "trace-id",
  "payload": {"projection": {"public_id": "task-public-id", "employee_public_id": "employee-public-id", "flight_display_no": "PROBE-001", "task_name": "Probe", "area_name": "Probe Area", "planned_at": "2026-08-31T08:10:00Z", "status": "pending", "message": "test only", "sync_version": 1}}
}
~~~

成功返回 HTTP 202：

~~~json
{
  "command_id": "018f0000-0000-7000-8000-000000000010",
  "status": "pending",
  "duplicate": false,
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

重复 command_id 当前返回 202 和 duplicate=true；格式错误或 Store 不可用分别为 400 invalid_command 或 503 command_store_unavailable。当前路由直接信任 body 中的 actor_public_id，且只有 probe.complete.v1 有 Foundation Handler；员工 Accept/Complete 不能当作已实现业务接口。

### 4.6 健康检查、Foundation 和内部同步

健康响应实际包含 status、details、request_id、trace_id。health/live 只代表进程存活，200；health/ready 的 MySQL 不可用时 503，Redis 不可用仍可 200。api/v1/foundation 仅为诊断。

内部同步接口由 Worker 调用，不应暴露给浏览器、低代码平台或员工端。当前代码没有机器认证中间件，部署必须依赖网络边界，并在生产补齐 Machine Identity。

| 方法/路径 | Body/Query | 成功 | 实际行为 |
|---|---|---|---|
| POST /internal/sync/v1/events | EventEnvelope | 202 event_id/status=applied/duplicate | 按 event_id 写 Inbox；当前只有 Probe 事件更新 Projection，其他事件可能 applied 但不投影 |
| GET /internal/sync/v1/commands/pending?limit=20 | limit 可选，1–100；非法值回退 20 | 200 items: CommandRecord[] | 领取 pending/retry/processing Command，返回 attempts、next_attempt_at、last_error |
| POST /internal/sync/v1/commands/{commandID}/ack | status、reason、next_attempt_at 可选 | 204 | applied/sent→sent；retry→retry；failed/rejected→failed；其他状态 400 |

### 4.7 Legacy/paused 路由

以下路由实际存在于 [internal/server/router.go](../internal/server/router.go)，但只由 [cmd/server/main.go](../cmd/server/main.go) 启动，不是 v2 Core/Edge 入口：

| 方法/路径 | 旧用途 | 状态 |
|---|---|---|
| GET /health/live、/health/ready、/health | 旧服务健康检查 | legacy/paused |
| POST /api/v1/auth/login | 旧用户名密码登录，返回旧 JWT wrapper | legacy/paused；无可用测试账号 |
| GET /api/v1/auth/me | 旧 Bearer JWT 当前用户 | legacy/paused |
| POST /api/v1/events/flight-arrived | 旧 B3 航班到达/任务生成 | legacy/paused，不等同 BVS2-03 |
| POST /api/v1/device/status | RFID/UWB 状态预留，只接收不处理 | legacy/paused |
| POST /api/prediction/analyze | AI 预测预留，固定返回 not_enabled | legacy/paused |

### 4.8 OpenAPI 与实际路由差异

1. [api/core/openapi.yaml](../api/core/openapi.yaml) 漏记 Core GET /api/v1/foundation。
2. Core OpenAPI 将 /internal/sync/v1/commands/pending 写在 Core API 下；实际它注册在 Edge API。
3. Core OpenAPI 未描述健康响应的 details、request_id、trace_id；arrival 的 200 描述也与实际“只有 duplicate=true 才是 200”不完全一致。
4. [api/edge/openapi.yaml](../api/edge/openapi.yaml) 使用 /tasks、/commands，实际为 /api/v1/tasks、/api/v1/commands，并漏记 foundation 和内部同步路由。
5. [api/sync/openapi.yaml](../api/sync/openapi.yaml) 使用相对路径；实际 HTTP 路径包含 /internal/sync/v1 前缀。
6. Edge OpenAPI 没有描述当前实际请求/响应字段和错误 body。

## 5. 计划中的业务接口

本章所有接口均为建议/尚未实现，不属于当前可调用 API。

本节不是当前可调用 API。现有冻结设计把核心动作称为 Leader/Manager Confirm，而不是 Publish；Task 当前数据库状态只有 awaiting_confirmation、assigned、in_progress、completed、cancelled，没有 draft、publishing、published。

| 方法/路径 | 建议用途 | 建议权限 | 建议事件/同步 | 状态 |
|---|---|---|---|---|
| POST /api/v1/tasks | 手工创建或从模板创建；规则待确认 | task:create（TODO） | 待确认 | 仍未实现；创建继续由 Flight Arrival 事务触发 |
| GET /api/v1/tasks | Core 管理端按状态、航班、Area/Team 查询 | task:read + Scope | 直接读 Core | BVS2-06 已实现；与 Edge 同路径但服务不同 |
| GET /api/v1/tasks/{id} | 任务详情、Candidate、Assignment、历史 | task:read + Scope | Core 最新事实 | BVS2-06 已实现；历史字段仍待产品确认 |
| POST /api/v1/tasks/{id}/publish | 若等同 Confirm，则选择 Candidate、创建 Assignment | task:assign + Area/Team Scope | task.assigned.v1，异步同步 Edge | 建议；Publish vs Confirm 待确认 |
| POST /api/v1/tasks/{id}/cancel | 取消 Task，释放 Assignment/Personnel | task:cancel + Scope | task.cancelled.v1，异步同步 Edge | 已实现；Core 事务提交后异步同步 Edge |

### 5.1 创建、列表和详情

POST /api/v1/tasks 的手工创建、是否产生 draft、是否绑定 Flight/Template、是否计算 Candidate、幂等键位置和事件均为 TODO。建议请求形状：

~~~json
{
  "flight_public_id": "flight-public-id",
  "template_public_id": "template-public-id",
  "name": "到达保障",
  "planned_at": "2026-08-31T08:15:00Z",
  "message": "请前往到达区",
  "area_public_id": "area-public-id",
  "team_public_id": "team-public-id",
  "source": "manual",
  "idempotency_key": "client-create-001"
}
~~~

建议响应形状（draft 当前不在数据库枚举中）：

~~~json
{
  "data": {"task_public_id": "task-public-id", "status": "draft", "status_version": 0, "sync_state": "not_applicable"},
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

建议的 Core 列表请求：

~~~http
GET /api/v1/tasks?status=awaiting_confirmation&flight_public_id=flight-public-id&page=1&page_size=20
Authorization: Bearer <待接入的管理端令牌>
~~~

建议响应：

~~~json
{
  "data": {
    "items": [{"task_public_id": "task-public-id", "flight_public_id": "flight-public-id", "task_name": "到达保障", "status": "awaiting_confirmation", "status_version": 0, "planned_at": "2026-08-31T08:15:00Z", "candidate_count": 1, "assignment_public_id": null, "sync_state": "not_applicable"}],
    "page": 1,
    "page_size": 20,
    "total": 1
  },
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

列表/详情的分页、过滤、是否返回完整 Candidate/Assignment/历史和同步状态均需产品/后端确认。失败场景包括 401、403、404、非法过滤、数据库不可用和版本冲突。

### 5.2 发布（若等同 Leader Confirm）

建议请求：

~~~http
POST /api/v1/tasks/task-public-id/publish
Content-Type: application/json
Idempotency-Key: optional-confirmation-id
Authorization: Bearer <待接入的管理端令牌>
~~~

~~~json
{
  "confirmation_id": "confirm-001",
  "candidate_public_id": "candidate-public-id",
  "expected_task_version": 0
}
~~~

建议响应：

~~~json
{
  "data": {
    "task_public_id": "task-public-id",
    "status": "assigned",
    "status_version": 1,
    "assignment_public_id": "assignment-public-id",
    "personnel_public_id": "personnel-public-id",
    "sync_state": "sync_pending"
  },
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

成功前提是 Human Principal、task:assign、Area/Team Scope、Task 为 awaiting_confirmation、Candidate 重新校验仍合格、Personnel 可从 idle 预留为 reserved。一个 Core 事务应创建 Assignment、更新 Candidate、Task/Personnel 状态历史、Audit 和 task.assigned.v1 Outbox。

失败场景：forbidden、404、candidate_no_longer_eligible、task_already_assigned、stale_task_version、confirmation_id_conflict 和事务失败。Core 成功但 Edge 未应用时显示“已分配，等待同步”；所有字段、状态码和 Publish 语义均为待确认。

### 5.3 取消

建议请求：

~~~http
POST /api/v1/tasks/task-public-id/cancel
Content-Type: application/json
Authorization: Bearer <待接入的管理端令牌>
~~~

~~~json
{
  "cancellation_id": "cancel-001",
  "expected_task_version": 1,
  "reason": "航班保障需求取消"
}
~~~

建议响应：

~~~json
{
  "data": {"task_public_id": "task-public-id", "status": "cancelled", "status_version": 2, "cancellation_id": "cancel-001", "sync_state": "sync_pending"},
  "request_id": "request-id",
  "trace_id": "trace-id"
}
~~~

冻结规则是：Manager/Admin 可在自身 Scope 取消非终态 Task；Leader 只能取消自身 Scope 内且员工尚未 Accept 的 Task；Staff 无权限。awaiting_confirmation 取消使 proposed Candidate 失效；assigned/in_progress 取消释放 Assignment 和 Personnel；完成/已取消任务不能回退。成功写状态历史、Audit、task.cancelled.v1 Outbox；失败包括 forbidden、404、状态非法、stale_task_version、cancellation_id_conflict 和事务失败。

## 6. 前端页面与接口对应关系

| 页面 | 页面用途 | 调用接口 | 主要状态 | 失败处理 |
|---|---|---|---|---|
| 任务列表（管理端） | 查询 Core 任务、按状态/Scope 筛选 | 建议 Core GET；当前无此路由 | loading、empty、error、awaiting_confirmation、assigned、in_progress、completed、cancelled | 401/403 提示登录/权限；5xx 可重试 |
| 任务详情 | 查看 Task、Candidate、Assignment、历史 | 建议 Core GET /tasks/{id}；当前无此路由 | loading、not_found、stale、version conflict | 404 返回列表；冲突重新拉取 |
| 新建任务 | 手工创建或本地草稿 | 建议 Core POST /tasks；当前无此路由 | draft、saving、saved、error | 不伪造任务 ID；draft 待确认 |
| 编辑草稿 | 编辑尚未提交的草稿 | 当前没有 API | draft、saving、stale | 只能本地编辑，直到后端确认 draft |
| 发布任务 | 确认 Candidate/创建 Assignment | 建议 Core POST /publish；当前无此路由 | publishing、assigned、sync_pending、sync_failed | 同 confirmation_id 重试；冲突刷新详情 |
| 取消任务 | 取消并释放占用 | 建议 Core POST /cancel；当前无此路由 | cancelling、cancelled、sync_pending、sync_failed | 状态/版本冲突不盲重试 |
| 同步失败重试 | 查看/重试同步 | 当前无公共查询/重试 API | sync_pending、sync_failed、retrying、confirmed | 需要后端提供受控 API，不能查表 |
| 员工任务查看 | 读取本人最小任务 | 当前可用 Edge GET /api/v1/tasks | loading、empty、error、assigned、in_progress、completed、cancelled | 不带员工 Header 当前会返回全部 Projection，是已知问题 |

前端必须显式处理 loading、empty、error、permission denied、draft、publishing、published、sync_pending、sync_failed、retrying、stale data 和版本冲突。其中 draft/publishing/published 不是当前 Core 状态；收到 202 也不能直接把 Projection 改成最终业务状态。

## 7. 数据模型与数据来源

| 数据 | 负责人 | 当前状态 |
|---|---|---|
| 航班、状态和状态历史 | Core | BVS2-03 已用于 scheduled→arrived；无完整航班查询 API |
| 员工、岗位、能力、人员状态 | Core | 表和领域模型已建立；BVS2-03 用于候选筛选，管理 API 未实现 |
| 任务模板 | Core | migration 和最高启用版本读取已实现；管理 API 未实现 |
| 任务实例 | Core | BVS2-03 生成 awaiting_confirmation；CRUD/确认/取消未实现 |
| 任务分配 | Core | 表已建立；Leader Confirm 代码未实现 |
| 任务状态 | Core | 状态机在设计和 SQL CHECK 中存在；只有初始状态写入已实现 |
| 事件 | Core | `task.assigned.v1`、`task.accepted.v1`、`task.completed.v1` 已实现并进入 Outbox；cancelled 仅保留 Edge 投影兼容入口 |
| 审计日志 | Core | BVS2-03 与业务事务同提交 |
| Edge Projection | Edge | 表、读接口、Core 快照事件投影和 `sync_version` 冲突保护已实现 |
| Mobile Command | Edge | Envelope 落库、员工 Accept/Complete 入队和技术状态已实现 |
| Edge Inbox / Core Inbox | Edge / Core | 分别按 event_id / command_id 接收和去重 |
| Outbox | Core | outbox_event 由业务事务写入，Worker 投递 |

Core BVS2-02 主要表：operation_area、team、personnel、team_member、flight、flight_status_history、task_template、task_instance、task_status_history、task_candidate、task_assignment、task_assignment_status_history、personnel_status_history、business_idempotency_record，另有 Foundation 的 audit_log、outbox_event、core_inbox。

Edge Foundation 表：sync_inbox、task_projection、notification_projection、mobile_command、mobile_session、delivery_log。前端/低代码平台不能直接修改数据库；所有 Core 写操作必须通过 API，由后端完成权限、状态机、事务、Audit 和幂等；Projection 只能通过事件收敛。

## 8. 事务、事件和同步流程

### 8.1 发布/确认任务的目标流程

当前 Publish/Confirm 尚未实现；若产品确认二者等价，流程为：

~~~text
前端点击发布
→ Core API 接收请求
→ 权限校验
→ 任务状态校验
→ Core 数据库事务
→ 更新任务状态
→ 写入 audit_log
→ 写入 outbox_event
→ 事务提交
→ Worker 投递事件
→ Edge 写入 sync_inbox
→ 更新 task_projection
→ 前端查询到最新状态
~~~

完整 Confirm 还要重新校验 Candidate、预留 Personnel、创建 Assignment、更新其他 Candidate 和写状态历史。Edge 投递失败不回滚 Core。

### 8.2 同步语义

不能把网络请求放进数据库事务：否则 Edge 超时会长时间持锁，并把本可成功的 Core 业务拖成回滚。Outbox 将事实提交与网络投递拆开；Inbox 防止超时、重启和重复消息产生第二次副作用。网络语义是 at-least-once delivery + idempotent consumption，不承诺 exactly-once。

同步可能延迟，因为请求只等待 Core 事务，Worker 还要轮询，Edge 还可能重启或暂不可用。“Core 发布成功”和“Edge 已同步”不是同一件事。当前 BVS2-03 的真实流转是 Core arrival → Flight arrived → Task/Candidate → Audit/幂等/task.generated.v1 Outbox → HTTP 返回；此阶段不创建员工 Projection。
## 9. 并发和幂等处理

### 9.1 已有保护与仍需完善的场景

| 场景 | 当前代码行为 | 已有保护 | 当前风险/前端处理 |
| --- | --- | --- | --- |
| 同一个 HTTP 请求重复提交 | Core 到达接口先查 business_idempotency_record，事务内锁定航班后再次查；相同业务键可返回已处理结果 | source_event_id 或派生业务键；数据库唯一约束；事务内二次检查 | 同一 source_event_id 对不同内容的完整 payload 冲突校验仍待完善；前端应保留并重用幂等业务标识，不要因超时盲目换 ID |
| 两个请求同时处理同一航班到达 | 航班行锁和事务内二次幂等检查使其中一个完成，另一个返回已处理或状态冲突 | FindFlightForUpdate、唯一业务幂等记录 | 这是当前已实现的到达切片保护；前端把 already_processed 视为可展示的已完成结果 |
| 两个用户同时发布同一个任务 | 当前发布接口尚未实现；设计要求确认时做状态、候选和版本校验 | BVS2 设计已定义确认并发验收项，但没有可运行实现 | 不能假设发布已经具备保护；前端遇到 409/版本冲突时刷新详情并提示“任务已被其他人处理” |
| 两个 Worker 同时处理同一个事件 | Core SQL Store 使用 FOR UPDATE SKIP LOCKED 抢占待投递记录；Edge Inbox 用 event_id 去重 | 数据库锁、Outbox 状态、Inbox 唯一事件 ID | 当前 processing 记录没有独立租约过期时间，且 next_attempt_at 不会在领取时推进；进程崩溃后的重新领取策略需要加强 |
| Edge 重复收到同一个事件 | 先写 sync_inbox，相同 event_id 返回 duplicate；投影按 sync_version 忽略更旧版本 | Inbox 唯一键、版本比较 | 相同事件 ID 但不同 payload 未比较；相同 sync_version 会更新，事件内容冲突需后端补充 |
| 旧版本事件晚于新版本到达 | UpsertTaskProjection 忽略 incoming 小于 existing 的版本 | 投影版本条件 | 版本字段的生产和所有事件类型的版本语义还需统一；前端应显示 stale/刷新提示，不要本地覆盖服务端版本 |
| 重复提交同一个员工 Command | Edge 以 command_id 去重并返回 duplicate；Worker 对已处理命令不重复执行 | Command 唯一 ID、命令状态 | 相同 command ID 的不同 payload 冲突未明确；Edge 当前命令入口未接入登录校验，不能作为生产员工端接口 |
| Worker 处理失败 | 记录 retry，达到最大次数后 failed；失败时通过 ACK 回写 Edge | 尝试次数、退避时间、失败状态 | 当前 Worker 对业务错误未做完整的永久/可重试分类；前端应区分 pending、retrying、failed，不要把投递失败当作业务失败 |

### 9.2 前端幂等建议

- 用户点击创建、发布、取消或重试时，应在 loading/retrying 状态，避免并行发送同一操作。
- 网络超时后先查询任务详情或操作状态；如果产品确认采用幂等键，应重试原请求并保留原幂等键，不能生成新的业务操作 ID。
- 收到 409、already_processed、stale_assignment 或版本冲突时，刷新 Core/Edge 的最新数据，并说明“数据已被其他操作更新”。
- 收到 Edge 202 pending 时，显示“已提交，等待同步”，轮询查询或等待推送；不要立刻把员工任务标记为已完成。
- 收到 sync_failed 或内部投递失败时显示“同步失败，可重试”，重试前先重新查询，避免重复创建命令。
- 客户端缓存只能用于展示优化，不能替代服务端的状态和版本校验。

### 9.3 后端下一步需要补充

1. 为任务取消/CRUD、正式认证入口和完整 Compose 闭环完成真实 Service、事务、状态机、权限和测试；员工 Command 的核心事务已实现。
2. 为 Outbox/Command 增加明确的 lease/lease_until 或等价抢占策略，并定义 Worker 崩溃恢复规则。
3. 将同 ID 不同 payload 的冲突语义补齐到其他 Foundation/历史路径，并保留 BVS2-05 Core/Edge 实现。
4. 将 BVS2-05 Projection 的版本来源、单调递增和同版本冲突规则扩展到所有业务事件，并补齐重建/乱序验证。
5. 细化可重试基础设施错误与不可重试业务错误，稳定错误码并补齐观测指标。

## 10. 前后端并行开发方式

建议按以下顺序协作：

~~~text
确定业务流程
→ 确定 API 契约、状态机、错误码和版本规则
→ 后端实现真实业务与迁移
→ 前端/低代码使用 Mock 并行开发
→ 接口联调
→ 异常、权限、重复请求和离线场景测试
→ 端到端验证
~~~

### 10.1 后端需要先确定的内容

- 任务创建、编辑、发布/Leader Confirm、取消的最终业务语义及状态转换。
- POST /api/v1/tasks 等建议接口的字段、权限角色、错误码、幂等键和响应 envelope。
- 任务、候选人、分配、事件及 Projection 的 ID、版本、时间字段语义。
- Core 到 Edge 的事件类型、schema version、Projection 可见字段和同步失败处理。
- 员工 Accept/Complete Command 的认证方式、服务端 actor 来源、成功/重复/过期响应。
- 哪些接口需要 JWT、哪些是内部 Worker 接口；不能把当前未鉴权的 Foundation 路由直接当生产权限模型。

### 10.2 前端可以先做的内容

- 任务列表、详情、新建和草稿编辑页面的布局与本地状态机。
- 基于建议契约的 Mock 数据、loading/empty/error/permission denied 展示。
- 发布、取消、重试按钮的禁用、确认弹窗和状态反馈。
- 员工端 Projection 的离线/待同步/同步失败展示，但字段要以最终契约为准。

### 10.3 契约变更规则

- API 变更应同时更新后端 handler/service、OpenAPI、Mock、本文档和联调记录。
- 字段不能由前端自行发明；未知字段、状态值、权限规则和版本字段必须标记“待确认”并由后端/产品确认。
- 低代码平台只调用 Core API、Edge API 或正式约定的内部同步接口；不能连接 Core/Edge MySQL，也不能写 Redis。
- 未实现的建议接口不能被前端当作可用接口发布；可通过 feature flag 或 Mock 保持页面开发进度。

## 11. 本地开发与联调

### 11.1 配置基线

当前可参考的配置文件是 configs/config.v2.yaml。它定义了：

| 项目 | 当前默认值 |
| --- | --- |
| Core API | :8081 |
| Edge API | :8082 |
| Core MySQL | 127.0.0.1:3310，数据库 flight_core |
| Edge MySQL | 127.0.0.1:3311，数据库 flight_edge |
| Edge Base URL | http://127.0.0.1:8082 |
| Worker 轮询间隔 | 500 ms |
| Worker 批量大小 | 20 |
| Worker 最大尝试次数 | 5 |
| Worker 请求超时 | 3 s |
| Redis | 配置中 disabled |
| JWT | 本地占位值，不能用于生产 |

配置使用 YAML，并可用 FLIGHT_ 前缀环境变量覆盖；实际环境变量键名遵循 Viper 的点号转下划线规则。真实密码、密钥和生产配置不能提交。

### 11.2 启动和迁移

本地依赖定义在 deployments/local/docker-compose.yml：

~~~powershell
docker compose -f deployments/local/docker-compose.yml config
docker compose -f deployments/local/docker-compose.yml up -d
docker compose -f deployments/local/docker-compose.yml ps
~~~

服务可分别运行：

~~~powershell
go run ./cmd/core-api -config configs/config.v2.yaml
go run ./cmd/edge-api -config configs/config.v2.yaml
go run ./cmd/worker -config configs/config.v2.yaml
~~~

迁移必须显式选择数据库：

~~~powershell
go run ./cmd/migrate -target core -command status -config configs/config.v2.yaml
go run ./cmd/migrate -target core -command up -config configs/config.v2.yaml
go run ./cmd/migrate -target edge -command status -config configs/config.v2.yaml
go run ./cmd/migrate -target edge -command up -config configs/config.v2.yaml
~~~

不要对当前机器上已有的 MySQL 使用破坏性回滚或清空命令。迁移程序支持 down，但项目规则要求先明确确认并使用 -allow-destructive。

### 11.3 健康检查和调用地址

- Core：GET http://127.0.0.1:8081/health/live、GET http://127.0.0.1:8081/health/ready
- Edge：GET http://127.0.0.1:8082/health/live、GET http://127.0.0.1:8082/health/ready
- Core 业务调用：http://127.0.0.1:8081
- Edge 员工端调用：http://127.0.0.1:8082
- Worker 通过配置中的 sync.edge_base_url 调用 Edge 内部同步接口；该地址不是浏览器端业务入口。

服务会返回或生成 X-Request-ID、X-Trace-ID，前端建议在日志和错误上报中保留它们。Ready 会检查所需 MySQL；Redis 在当前配置中是可选项，异常不会单独使 Ready 失败。

### 11.4 测试账号和测试数据

当前代码没有发现可直接使用的测试账号、登录种子数据或 Core/Edge 业务种子数据；迁移主要创建表结构。v2 Foundation API 目前没有接入 JWT 鉴权，不能据此推断生产认证已经完成。旧 cmd/server 的登录接口属于 legacy/paused，不应作为 v2 联调入口。

因此，联调前需要后端或测试人员明确：

- 测试账号及角色；
- 如何创建航班、模板、人员、岗位、能力和任务数据；
- Core/Edge 的本地数据库初始化方式；
- 是否提供可重复执行的 fixture/seed。

### 11.5 测试和构建命令

项目默认检查命令：

~~~powershell
go test ./...
go build ./...
go build ./cmd/core-api
go build ./cmd/edge-api
go build ./cmd/worker
git diff --check
git status
~~~

本文档完成时将只报告本次实际运行的结果；历史交接文档中的结果仅作为背景，不能替代本次检查。

## 12. 前后端交接清单

~~~text
[ ] API 路径已确认
[ ] 请求字段已确认
[ ] 响应字段已确认
[ ] 错误码已确认
[ ] 权限要求已确认
[ ] 状态机已确认
[ ] 异步同步状态已确认
[ ] Mock 数据已准备
[ ] 前端页面已完成
[ ] 后端接口已完成
[ ] 联调已完成
[ ] 重复请求已测试
[ ] 权限失败已测试
[ ] 同步失败已测试
[ ] 版本冲突已测试
~~~

BVS2-03/04/05 已可用于到达、Leader Confirm、Edge Projection 和员工 Command 的有限联调；任务 CRUD、Task Cancel、正式认证和完整 Compose 闭环仍需在 BVS2-06 逐项勾选。

## 13. 当前问题和待确认事项

### 13.1 当前已确认的问题

1. **业务范围仍未覆盖完整 BVS2-06。** BVS2-03 Flight → Task → Candidate、BVS2-04 Leader Confirm 和 BVS2-05 Edge Projection/Employee Command 已实现；任务 CRUD、Task Cancel 和完整容器闭环验收仍未完成。
2. **v2 API 当前未接入认证授权。** internal/platform/security 与 Core IAM 已有基础组件和角色概念，但当前 Foundation 路由没有看到 JWT middleware/Authorizer 的实际接入；不能把未鉴权接口交付给生产用户。
3. **认证仍需收敛。** BVS2-05 员工路由使用 `X-Employee-Public-ID` 作为当前内部联调 Actor 来源，Core 业务校验仍在 Worker 内执行；正式 JWT/mTLS 接线尚未作为生产入口交付。
4. **建议接口尚未实现。** 文档第 5 章的任务接口是建议契约，不能当作当前可调用 API。
5. **Edge 的员工筛选尚不完整。** GET /api/v1/tasks 没有员工 ID 时会返回 Edge 中全部任务投影；员工鉴权、可见范围和正式业务筛选待确认。
6. **没有发现可直接使用的测试账号和业务种子数据。** 本地联调需要补充 fixture/seed 或明确数据准备步骤。
7. **基础设施租约需要加强。** Core Outbox、Edge Command 领取使用行锁，但 processing 记录没有明确 lease 到期字段，领取时也没有推进 next_attempt_at；Worker 崩溃后的恢复、并发领取和超时语义需补充测试。
8. **重复 ID 冲突检测仍需全链路补齐。** BVS2-05 Projection 已比较同版本快照，业务 Command 的 Core Inbox 已比较同 ID Envelope；其他 Foundation/历史路径仍需统一错误结果。
9. **完整 Compose 服务闭环仍未验收。** Docker CLI 已由 `scripts/ensure-docker.ps1` 自动定位并确认 Engine 就绪；本轮已在隔离临时 Core/Edge MySQL 完成 migration 和对应 SQL 验证，但尚未重新启动独立 Compose 项目完成 Core→Worker→Edge 全服务闭环。
10. **未知事件可能被标记已应用。** Edge 对未知事件类型不生成任务投影但仍可完成 Inbox 应用流程；task.generated.v1 到 Edge 的最终投影映射仍未实现。
11. **Worker 失败分类较粗。** 当前主要按尝试次数重试，业务永久错误与基础设施临时错误的分类、告警和人工恢复流程待完善。

### 13.2 需要产品/后端确认的事项

- “发布任务”是否就是 BVS2-04 的 Leader Confirm；如果不是，需要另行定义状态、权限和事件。
- 任务创建者、Leader、Manager、员工的权限矩阵以及单机场景下的岗位/区域边界。
- 任务计划时间、航班到达时间、任务状态和人员状态的最终字段与时区规则。
- 任务取消的触发条件、是否需要原因、是否可撤销，以及对候选/分配的影响。
- Edge 是否需要保存员工会话/设备信息才能支持正式员工端。
- Edge Projection 是否需要公开 sync_version、更新时间和同步错误详情。
- API 是否统一采用当前 data/request_id/trace_id、items 等响应 envelope，还是要更新 OpenAPI 和错误码模型。
- 外部 Core API、Edge API 是否需要 JWT；内部同步接口是否需要独立服务认证和网络边界。
- 测试账号、种子数据、契约测试和端到端环境由谁维护。

### 13.3 下一步分工建议

**后端：**

- 按冻结设计补齐任务 CRUD、取消和员工 Command 的全链路 API/契约验收；员工 Command 核心状态机和事务已完成。
- 接入 JWT/RBAC/Actor 来源，区分浏览器业务接口和 Worker 内部同步接口。
- 修正 OpenAPI 与实际路由差异，补齐错误码、request/trace ID 和版本条件更新说明。
- 完善 Outbox/Inbox/Command 的 lease、同 ID payload 冲突和 Projection 同版本规则。
- 提供可重复测试数据、测试账号和覆盖并发/重试/同步失败的测试。

**前端/低代码：**

- 先按本文建议契约制作 Mock，完成列表、详情、草稿、发布确认、取消、同步失败重试的状态展示。
- 接入实际 API 前确认环境地址、鉴权方式、错误码和状态枚举；不要连接数据库。
- 对每个写操作实现按钮幂等、超时查询、409/版本冲突刷新、202 pending 和 sync_failed 展示。
- 保存 request ID/trace ID 便于联调，准备重复点击、权限失败、网络断开、旧版本数据和空数据场景。
- 业务契约冻结后再移除 Mock，并参与端到端验收。

## 14. 2026-09-01 前端工程设计交接校正

前端工作区目录骨架已经创建，采用一个 `frontend/` 工作区、`admin-web`、首期 `employee-miniapp` 和 `employee-web` 三个客户端，共享 contracts、API client、认证接口、UI 和 Mock 包。当前只存在 README 和占位文件，前端源码、依赖清单、构建配置和页面仍未实现。

```text
frontend/
├── apps/
│   ├── admin-web/src/       # 管理端，只调用 Core API
│   ├── employee-miniapp/src/ # 首期员工小程序，个人微信/企业微信共用，只调用 Edge API（F1 创建）
│   └── employee-web/src/    # 正式员工网页版/备用入口，只调用 Edge API
├── packages/
│   ├── contracts/src/       # DTO、枚举、schema、错误码
│   ├── api-client/src/      # Core/Edge HTTP Client
│   ├── auth/src/            # 认证适配接口
│   ├── ui/src/              # 共享 UI 基础组件
│   ├── task-domain/src/     # 前端展示状态映射
│   └── mock/src/            # fixture 和 Mock handler
├── e2e/
└── docs/
```

根目录已有的 `web-admin` 和 `miniapp` 仍为空目录，未作为新的前端工程入口使用。

- `admin-web` 只调用 Core API；当前可依赖的业务入口包括 `GET /api/v1/tasks`、`GET /api/v1/tasks/{taskPublicID}`、`POST /api/v1/tasks/{taskPublicID}/confirm`、BVS2-03 Arrival 和已实现的 `POST /api/v1/tasks/{taskPublicID}/cancel`。手工创建、任意 PATCH 和硬删除仍未形成后端契约。
- `employee-web` 只调用 Edge API；当前代码提供 `GET /api/v1/tasks`、`POST /api/v1/tasks/{taskPublicID}/accept` 和 `/complete`。Accept/Complete 返回 202 时只代表 Command 已进入 Edge Store，不能直接把业务状态改为 `in_progress` 或 `completed`。
- Edge Projection 的业务状态由 Core 事件回传收敛，员工端 Command 的 `pending/syncing/confirmed/failed` 只作为显示层状态；浏览器不调用 `/internal/sync/v1/*`。
- BVS2-07 已提供员工工号密码、Provider exchange、绑定完成、Refresh 轮换、当前会话和 Logout；开发 Provider 只接受显式 `mock:<subject>`，真实微信/企业微信 Provider、客户端 Command 幂等键和 Command 状态查询仍待确认。`X-Employee-Public-ID` 与 `X-Actor-*` 仍只允许显式非 release 开发适配。
- 首期同时建设管理端和员工端：管理端采用桌面优先响应式 Web；员工端首期采用同时支持个人微信和企业微信入口的 `apps/employee-miniapp`，同时保留正式可用的 `employee-web` 网页版。两类微信身份必须映射到同一个 Staff，两个员工客户端复用共享契约和任务展示模型。
- 前端首期按 Mock-first 设计，未实现接口不得在生产构建中开放；页面路由、线框、视觉 Token 和状态模型见 `../frontend/docs/page-design.md`，主设计、技术栈和分步计划见 `../memory-bank/frontend-design-document.md`、`../memory-bank/frontend-tech-stack.md`、`../memory-bank/frontend-implementation-plan.md`。

## 16. 2026-09-01 F1 页面 Mock 预览

`frontend/preview/` 已提供零依赖静态探索 Mock 页面，用于在正式 React/Vite 工具链和真实 API 接入前暴露第一版页面结构问题。由于管理端/员工端/登录页不应合并在同一正式页面、管理端需要体现角色权限和更多工作区，本预览不再视为 F0 冻结依据。

- 入口：[`frontend/preview/index.html`](../frontend/preview/index.html)
- 启动：`python -m http.server 4173 --directory frontend/preview`
- 边界：预览不请求 Core/Edge、不连接数据库，不代表真实登录或业务状态已经落库；正式客户端仍必须遵守 `admin-web → Core API`、员工小程序/员工 Web `→ Edge API`。

## 17. 2026-09-02 F0 信息架构确认

用户已确认首期前端信息架构：主任映射 `manager`，队长映射 `leader`，员工映射 `staff`，系统管理员映射 `admin`；队长按照团队/区域自动获得任务，主任可以查看所有团队、区域和任务。管理端首期完整保留运行总览、航班运行、任务中心、人员管理、规则、事件/通知、审计、报表和系统管理导航，即使某些模块暂时没有真实内容，也显示明确空态。员工端保留任务、通知、异常、历史和账号入口。

当前 `frontend/preview/` 已拆分页面地图、管理端登录、员工登录、管理端工作区和员工端工作区。任务列表 Mock 会区分主任全量和队长团队/区域 Scope；该展示不替代 Core 后端权限校验。视觉美化列为后续独立修改项，最终页面路由、Scope 交叉规则和真实模块字段仍需完成 F0 最终评审。

## 18. 2026-09-02 F2 首个真实前端闭环

`frontend/` 已从目录骨架进入正式源码阶段：`admin-web` 通过 Core API 读取 Task List/Detail 并提交 Confirm/Cancel；`employee-web` 通过 Edge API 完成工号密码登录、可刷新会话、本人 Projection、Accept/Complete 和 Command Status 查询；`employee-miniapp` 已提供原生 `wx.request` 与存储适配器，共享同一 Edge 契约。

本轮没有伪造管理端密码登录接口。Core 已提供默认关闭的管理 SSO authorization-code 路由、`admin_identity`/`admin_session` 会话存储、企业微信 Provider 和管理会话解析；当前部署不依赖 OIDC，管理端支持配置企业微信 SSO callback，也保留粘贴已签发 Core JWT 或显式非 release 开发 Actor Header。真实企业微信配置、员工/管理员主数据、回调域名和线上联调仍待完成。Node/npm/pnpm 已配置；员工 Web 的正式依赖安装、lint/typecheck/build、真实 API 浏览器联调、刷新恢复和基础 WebSocket E2E 已运行并通过，完整异常场景 E2E 和真实浏览器/小程序平台矩阵仍待补齐。详细运行说明见 [`frontend/docs/api-integration.md`](../frontend/docs/api-integration.md)。
> BVS2-06 更新（2026-09-01）：Core POST /api/v1/tasks/{taskPublicID}/cancel 已实现并通过独立 Docker Core MySQL 验证；Task 列表/详情读侧、Compose/恢复验证和正式 JWT/mTLS 入口也已有对应代码或验证。当前 Task CRUD 仍遵循 Arrival 创建、GET 读侧、Confirm/Cancel 生命周期语义，手工 POST/PATCH/硬删除以及员工 Command 客户端幂等/状态查询仍待合同冻结。

## 2026-09-01 当前前端 F0 冻结校正

本文件前部和历史章节保留了早期 BVS2-06 交接状态，不能单独作为当前实现判断。按当前代码和 `memory-bank/progress.md`：

- Core 已有 Task 列表/详情读接口、Arrival 创建语义和 Task Cancel；手工 `POST /tasks`、任意 PATCH、硬删除仍未定义。
- Core/Edge 业务入口在正常配置下使用 Bearer JWT；`X-Actor-*`、`X-Employee-Public-ID` 只允许显式非 release 开发适配器。
- `employee-miniapp` 和 `employee-web` 都是正式前端边界内的员工客户端，只调用 Edge；个人微信和企业微信必须映射到同一个 Core `Staff`。
- 用户已确认员工身份体验：首次进入小程序用工号 + 密码认证，之后个人微信和企业微信均可快捷进入同一个员工账号；任一入口看到同一份 Edge Projection，长会话必须可刷新、可撤销。BVS2-07 已提供本地 Mock Provider、密码登录、双身份绑定、Session 刷新/轮换和撤销接口，并已在隔离双库完成当前源码在线联调；真实微信/企业微信 Provider 仍待接入，详见 [`frontend/docs/identity-contract-proposal.md`](../frontend/docs/identity-contract-proposal.md)。
- 员工快捷 Accept/Complete 当前返回 `202 pending`，客户端 Command 幂等键、公共 Command 状态查询和刷新恢复语义仍是 F0 阻塞项。
- 前端架构冻结门槛和进入真实业务的验收条件见 [`frontend/docs/architecture-freeze-gate.md`](../frontend/docs/architecture-freeze-gate.md)。

## 15. 性能与可靠性限制

- 消息不丢失、重复消息无错误副作用、Core/Edge 状态正确收敛和故障可恢复性优先于视觉效果。
- 管理端、员工小程序和员工 Web 的关键操作不能等待动画；默认只使用短时 `transform`/`opacity` 反馈，并支持 reduced motion。
- 默认禁止高成本持续特效、全屏视频/WebGL/Canvas 粒子、复杂 3D、大面积 blur/backdrop-filter、动态渐变、全量列表 stagger 和大型动效资源。
- 前端待优化代码分包、长列表、请求取消/去重、内存生命周期和小程序增量更新；后端待优化 API/数据库/Worker/同步链路指标、索引、分页、租约、有界并发、批量投递和性能剖析。
- 以上指标和待办不是当前已完成的压测结果；详细要求见 [`docs/performance-and-reliability-baseline.md`](performance-and-reliability-baseline.md)。
## 2026-09-01 员工 Command 契约已补齐

后端已关闭 F0 中员工 Command 幂等和公共状态查询两个阻塞项。Accept/Complete 请求体现在必须包含客户端生成的稳定 `command_id`；相同 `command_id` 且业务内容一致时，Edge 返回 `202`、原命令状态和 `duplicate=true`，内容不一致返回 `409 command_id_conflict`。客户端刷新恢复使用 `GET /api/v1/commands/{commandID}`，响应 `data.status` 只使用 `pending`、`syncing`、`confirmed`、`failed`，并包含 `attempts`、可选 `next_attempt_at`、`created_at`、`updated_at`；失败只返回 `error_code=command_failed`，不返回内部错误文本。该查询必须携带员工 Bearer JWT，不能调用 `/internal/sync/v1/*`。

## 2026-09-02 员工任务拉取恢复契约

`GET /api/v1/tasks` 是按员工 JWT Principal 过滤的完整 Edge Projection 快照。响应包含 `sync_mode=full_snapshot`、`snapshot_at`、员工级 `projection_revision`、`projection_lag_seconds`、`projection_lag_state`、`next_cursor` 和 `reset_required`。当前没有增量 API，因此 `next_cursor=null`、`reset_required=false`；不得使用单 Task `sync_version` 拼接员工全局游标。

客户端在启动、刷新、重连、收到变化提示和离线恢复时重新拉取快照并替换任务集合；HTTP 200 的空 `items` 是合法空结果，取消/完成以 Projection 终态为准。Projection lag 是观测信息，不是业务确认。本地缓存不作为可靠事实，Command 状态则以稳定 `command_id` 和 `GET /api/v1/commands/{commandID}` 独立恢复。Edge 部署前需按顺序应用 `migrations/edge/mysql/000004_employee_projection_cursor`。

## 2026-09-02 T0-3 通知抽象和提交后边界

后端已提供 Edge 内部 Notification/Fan-out Port：`internal/edge/application/notification` 暴露 `TaskChanged`、`Publisher` 和 `Sink`，当前实现为员工隔离的单实例内存 fan-out。T0-4 已在此 Port 之上增加 WebSocket 公共端点、连接鉴权、心跳、重连和快照恢复协议。

后续前端 WebSocket 收到 `task_changed` 时只能将其视为刷新提示：载荷包含 `notification_id`、`task_public_id`、`sync_version`、`reason` 和 `issued_at`，不包含员工 ID；客户端应去重后重新调用 `GET /api/v1/tasks`，以完整快照和 `projection_revision` 替换任务集合。通知可能丢失、重复、乱序或因副本未命中而不可见，不能直接改变业务状态，也不能替代 Command status 查询。

## 2026-09-02 T0-4 员工 WebSocket 提示

Edge 对员工 Web 暴露：

- `POST /api/v1/realtime/ticket`：使用员工 Bearer JWT 获取 30 秒一次性 ticket；ticket 只用于后续 WebSocket 子协议协商，不能放进 URL。
- `GET /api/v1/ws`：提供 `flight.realtime.v1`，浏览器同时发送 `flight.realtime.ticket.{ticket}` 作为候选子协议；服务端只回显应用协议。握手要求同源 Origin 和已解析的员工 Human Principal。

`employee-web` 已在 `packages/api-client` 提供 `RealtimeClient`，负责 ticket 获取、心跳回复、notification ID 有界去重和指数退避重连；连接成功、重连成功或收到 `task_changed` 时，页面重新拉取完整 `GET /api/v1/tasks`。WebSocket 关闭、ticket 过期或通知丢失不会阻塞任务读取和 Command status 恢复。Gateway、双 Edge、共享 SQL ticket 和 Redis best-effort fan-out 代码已接入，多副本 fan-out、服务重启和网络分区仍待 Docker 验收。
