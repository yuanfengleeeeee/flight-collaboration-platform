# 前端工程实施计划

> 当前状态：核心页面、共享客户端和三个员工入口已完成代码接入；F0 仍在进行真实平台配置、隔离环境验收、故障场景和性能门禁。业务合同以 [`docs/business-process-v2.md`](../docs/business-process-v2.md) 为准。

## 实施原则

- 管理端只调用 Core，员工端只调用 Edge；前端不连接数据库、不调用 `/internal/sync/v1/*`。
- 页面只渲染服务端 Projection/Core 查询结果；本地点击、缓存、socket 提示和 HTTP `202` 都不能伪造最终业务状态。
- 可靠写入使用 Edge 持久化 Command 和稳定 `command_id`；失败可查询、可重试，断线后重新读取快照。
- 航班只读外部事实；前端不提供任意新增/修改/删除航班。
- 员工任务操作是“我已收到”→“开始执行”→“完成”；没有接受/拒绝二选一。冲突和突发事件通过异常/变更申请提交。
- 所有管理集合、历史和通知服务端分页；员工任务使用按员工隔离的完整快照恢复。

## 当前客户端范围

| 客户端 | API | 当前代码范围 |
| --- | --- | --- |
| `admin-web` | Core | 任务、Assignment、任务变更审批、人员、岗位/能力、模板、运行/诊断读取 |
| `employee-web` | Edge | 登录/绑定、任务、收件/开始/完成、异常、通知、历史、账号、WebSocket 提示 |
| `employee-miniapp` | Edge | 个人微信登录、任务、异常、通知、历史、账号、原生 socket 提示 |
| `employee-wecom-miniapp` | Edge | 企业微信登录、任务、异常、通知、历史、账号、原生 socket 提示 |

两个小程序独立发布、独立使用平台登录适配器，但最终映射到同一个 Core `Staff`；业务页面和 Edge 任务合同一致。

## 已完成的仓库内工作

- 共享 `contracts`、`api-client`、`auth`、`task-domain` 和 UI 基础包已建立。
- Core/Edge API Client 已覆盖认证、任务快照、收件/开始/完成、异常申请、Command status、管理查询和分页。
- 管理端已按角色和 Scope 展示任务与审批边界；只有 manager/admin 显示任务变更审批。
- 员工端已按 `pending_dispatch/assigned/received/in_progress/completed/cancelled` 映射业务状态，并分离 Command 的 `pending/syncing/confirmed/failed`。
- 页面已覆盖 loading、empty、error、forbidden、not found、stale、pending、syncing 和 failure 等状态；实时消息只触发刷新。
- 个人微信、企业微信原生壳保留独立登录和 socket 适配；真实 AppID、域名和密钥不进入源码或小程序包。

## 当前剩余工作

### F0：合同与页面验收

- 对齐 Core/Edge OpenAPI、错误码、分页元数据和状态展示文案。
- 用可重复 Mock/Contract/E2E 夹具覆盖空数据、权限、重复 Command、超时、失败重试、断线恢复和 Projection 延迟。
- 管理端验证待派短缺、Assignment、变更申请审批、版本冲突和审批失败；员工端验证收件超时提示和异常申请。

### F1：真实身份与平台配置

- 配置工号密码正式策略、管理员预置、员工导入和管理端 SSO。
- 接入真实个人微信/企业微信 Provider，完成首次绑定、换绑、身份冲突和离职失效验证。
- 配置 HTTPS、通信域名、回调域名、TLS、平台通知和生产密钥注入。

### F2：联调与可靠性门禁

- 在隔离环境验证航班源失效/fallback、重复/乱序消息、Worker/Edge 重启、网络分区、实时提示丢失和多副本恢复。
- 验证延误、取消、员工保障冲突、任务变更审批和收件超时重派；确认员工不会出现拒绝状态。
- 形成 API/DB/Worker/Projection 的 p50/p95/p99、列表规模、内存、重连和小程序性能报告。

## 设计与动效边界

- 关键操作立即发起请求，使用短时 `transform`/`opacity` 反馈，支持 `prefers-reduced-motion`。
- 不使用持续 WebGL/Canvas、全量列表 stagger、大面积 blur、动态渐变或大型动效资源。
- 请求支持取消、去重和有限退避；本地缓存只用于展示优化，不能当作可靠离线队列。

## 验证入口

涉及后端、数据库、Compose 或双端联调前先执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
```

前端工具链可使用：

```powershell
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

真实平台和 Docker/Compose 未实际运行前，不得在页面文档中标记为已验收。
