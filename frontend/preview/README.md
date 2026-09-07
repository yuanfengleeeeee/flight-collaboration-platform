# F0 Mock 页面预览

这是一个零依赖的静态探索预览，用于在真实业务接入前暴露管理端、员工端和登录流程的页面结构、视觉方向与关键状态问题。它不是最终产品导航，也不代表 F0 已冻结；它不连接 Core API、Edge API 或任何数据库。

## 启动

如果本机安装了 Python，可以在项目根目录运行：

```powershell
python -m http.server 4173 --directory frontend/preview
```

然后打开 <http://127.0.0.1:4173>。

也可以直接打开 `index.html`，但使用本地 HTTP 服务更接近浏览器实际运行方式。

## 页面入口与可查看状态

- `index.html`：页面地图，不属于正式产品页面。
- `login-admin.html`：独立管理端登录页；提交后进入 `admin.html`。
- `login-employee.html`：独立员工登录页；提交后进入 `employee.html`。
- `admin.html`：管理端完整工作区，支持主任/队长 Mock 视图切换、任务 Scope、任务详情和模块空态。
- `employee.html`：员工端任务、通知、异常、历史和账号独立入口。
- 管理端：任务筛选、任务详情、候选人确认、任务取消、角色 Scope 和完整导航骨架。
- 员工端：任务列表、任务详情、我已收到/开始执行/完成的 pending → 已同步状态、异常申请、账号与微信入口绑定。

预览中的操作反馈明确标注为 `Mock`，不代表真实 API 已接入或业务状态已经落库。
