# 前端真实 E2E

这些用例只通过 HTTP/浏览器访问 Core、Edge 和员工 Web，不连接 MySQL、Redis、Outbox、Inbox 或 Worker 内部接口。

## 基础员工 Projection

```powershell
$env:E2E_EMPLOYEE_NO = "<employee-no>"
$env:E2E_EMPLOYEE_PASSWORD = "<password>"
$env:E2E_BASE_URL = "http://127.0.0.1:4175"
pnpm.cmd exec playwright test e2e/employee-web.spec.ts
```

## F3 生命周期与角色 Scope

需要由隔离环境预置一条 `assigned` 任务，并提供其员工、团队和区域 Scope：

```powershell
$env:E2E_LIFECYCLE_TASK_ID = "<assigned-task-public-id>"
$env:E2E_LIFECYCLE_EMPLOYEE_NO = "<employee-no>"
$env:E2E_LIFECYCLE_EMPLOYEE_PASSWORD = "<password>"
$env:E2E_SCOPE_TASK_ID = $env:E2E_LIFECYCLE_TASK_ID
$env:E2E_SCOPE_TEAM_ID = "<task-team-id>"
$env:E2E_SCOPE_AREA_ID = "<task-area-id>"
$env:E2E_CORE_API_URL = "http://127.0.0.1:8081"
$env:E2E_BASE_URL = "http://127.0.0.1:4175"
pnpm.cmd exec playwright test e2e/employee-task-lifecycle.spec.ts e2e/core-task-scope.spec.ts
```

生命周期用例会真实执行 `received → start → Command status confirmed → complete → 刷新恢复终态`；Scope 用例会验证主任全局可见、正确团队/区域队长可见、错误团队/区域队长返回空列表且详情为 404。所有 JSON 请求和响应使用 UTF-8。

## Edge 错误契约

设置员工账号后可单独运行：

```powershell
$env:E2E_EMPLOYEE_NO = "<employee-no>"
$env:E2E_EMPLOYEE_PASSWORD = "<password>"
$env:E2E_EDGE_API_URL = "http://127.0.0.1:8082"
pnpm.cmd exec playwright test e2e/edge-error-contract.spec.ts
```

该用例验证未登录 401、跨员工命令 403、未知 Command 404、同一 `command_id` 不同业务内容 409；非 JSON 上游 503 的稳定映射由 `@flight/api-client` 单元测试覆盖。
