# ADR-012：运行历史、异常和报表的业务边界

状态：Accepted
日期：2026-09-03

## 决策

- Core 是任务、Assignment、人员状态和异常事实的唯一来源。
- 管理端任务历史通过 `GET /api/v1/tasks/{taskPublicID}/history` 读取 Core 的三类状态历史，并按发生时间组成只读时间线。
- 员工端历史通过 Edge Projection 的 `GET /api/v1/history` 读取，只展示该员工已完成或已取消的任务；它不是 Core 审计替代品。
- 员工通知通过 Edge 的持久化 Projection 读取。Core 任务状态事件进入 Edge 后生成通知 Projection，通知已读状态只属于该员工，不回写 Core 事实。
- 员工异常通过 Edge `POST /api/v1/tasks/{taskPublicID}/exceptions` 进入可靠 Command；Core Inbox 在同一事务内校验 Assignment、人员归属、同步版本、权限并写入 `task_exception` 与审计。
- 管理端报表通过 Core `GET /api/v1/reports/overview` 读取按 Scope 聚合的当前事实；不在浏览器端根据分页列表自行推导全量统计。
- 当前规则配置以 Core 任务模板版本为可操作规则。模板的 `trigger_type`、目标区域/班组、岗位/能力约束和启用状态共同构成规则快照；历史任务保留模板版本。

## 状态与幂等

- 异常状态为 `open → acknowledged → resolved|rejected`；异常上报使用客户端生成的 `command_id`，重复 Command 必须没有重复异常记录或重复副作用。
- 历史和报表接口只读并强制使用服务端 Principal Scope；客户端不能提交 SQL 条件或扩大可见范围。
- Edge 端通知读取和已读操作必须按当前员工隔离；已读重复提交是幂等成功。
- Projection 或实时通知缺失时，员工仍可通过 Edge 的可靠查询恢复状态；实时通道只作为低延迟提示。

## 验收条件

1. Core 任务历史包含任务、Assignment、人员状态变更，时间线有稳定排序且越权任务返回不可见/拒绝。
2. 员工只能读取本人 Edge 历史和通知，已读操作不会影响其他员工。
3. 异常上报在 Core 中可查询，迟到同步版本、错误 Assignment 和非本人上报被拒绝；重复 Command 不增加记录。
4. 管理端可按异常状态/等级筛选，并能看到按当前 Scope 聚合的任务、航班、Assignment、人员、异常和同步指标。
5. 所有新接口都有 OpenAPI、客户端类型和至少一个自动化测试或隔离环境 HTTP 验收场景。
