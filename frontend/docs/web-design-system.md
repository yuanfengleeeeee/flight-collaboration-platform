# Web 端设计基线

> 当前状态：Web 设计基线已形成，等待最终视觉与正式环境验收。适用客户端：`admin-web`、`employee-web`。

## 1. 设计主张

产品是航空客运地面运行台，不是泛企业 BI。页面优先回答：哪一班航班、哪项保障任务、谁负责、当前状态和下一步可操作动作。

- 管理端：高密度、可筛选、可追溯，服务 admin/manager/leader/supervisor。
- 员工端：移动优先、低干扰、下一步明确，服务员工执行已分配工作。
- 航班数据只读外部事实；任务由到达事件自动生成、自动预分配，不由前端手工创建。

## 2. 视觉与状态

航班号、计划时间、国内/国际、航站楼、区域和任务名称组成首要识别条带。示例：

```text
07:40   MU 2156   国际 / T2   值机服务             [自动派发中]
        ─────────────── amber 状态轨 ───────────────
```

状态颜色必须与文字/图标同时出现：

- amber：自动派发中、已分配、需要关注；
- blue：执行中、信息处理中；
- teal：已完成、同步正常；
- red：取消、异常、阻断。

颜色不能单独承担状态含义；状态轨不使用持续闪烁、粒子、视频、渐变或大面积毛玻璃。

推荐基础 Token：`--ops-ink-950` `#081722`、`--ops-paper-050` `#f9fbfc`、`--ops-signal-amber` `#f0b85a`、`--ops-signal-teal` `#16847a`、`--ops-signal-blue` `#2d6e9f`、`--ops-signal-red` `#c94b45`。中文字体优先 `Noto Sans SC`/`PingFang SC`，航班号和时间使用等宽字体。

## 3. 页面地图

### 管理端 `admin-web`

| 路由 | 页面任务 | 首期角色 |
| --- | --- | --- |
| `/login`、`/sso/callback` | 管理账号与企业微信浏览器登录回调 | manager、leader、supervisor、admin |
| `/overview` | 航班、任务、异常、同步和待处理摘要 | 按权限显示 |
| `/flights` | 外部航班事实、来源新鲜度和运行态势 | manager、leader、supervisor、admin |
| `/tasks`、`/tasks/:id` | 任务中心、候选、Assignment、状态轨、变更申请 | 按后端 Scope |
| `/assignments` | 自动派发结果、收件、短缺和重派记录 | 按后端 Scope |
| `/templates`、`/rules` | 任务模板、岗位能力要求和规则版本 | manager、admin |
| `/organization`、`/personnel` | 区域、班组、员工、账号和状态 | manager、admin |
| `/positions`、`/capabilities`、`/status` | 岗位/能力字典、人员状态和可用性 | manager、admin |
| `/events`、`/audit`、`/reports` | 运行事件、审计和聚合报表 | 按权限显示 |
| `/users-roles`、`/scopes`、`/diagnostics` | 管理身份、Scope 和诊断 | admin；部分只读页按授权开放 |

航班页不提供任意新增、修改或删除；没有公开 API 的页面显示明确空态，不伪造成功数据。

### 员工端 `employee-web`

| 路由 | 页面任务 | 关键动作 |
| --- | --- | --- |
| `/login` | 工号密码登录和平台绑定入口 | 登录、进入绑定 |
| `/bind` | 将平台身份绑定到项目员工账号 | 绑定、查看状态 |
| `/tasks` | 本人任务、航班条带和状态筛选 | 我已收到、进入详情 |
| `/tasks/:id` | 任务说明、地点、状态轨和同步状态 | 我已收到、开始执行、完成、提交异常 |
| `/notifications` | 通知和异常提醒 | 已读、进入关联任务 |
| `/exceptions` | 保障冲突、延误、取消和突发事件 | 报告受控变更申请 |
| `/history` | 本人任务状态时间线 | 分页查看 |
| `/account` | 工号、岗位、能力、绑定和会话 | 刷新、退出、解绑按策略执行 |

## 4. 角色与数据范围

| 角色 | 可见范围 | 管理端重点 |
| --- | --- | --- |
| `manager` | 授权范围，默认全局运行视图 | 任务运行、审批、组织和人员 |
| `leader` | 自己团队/区域 Scope | 所辖任务、异常报告和跟进；不能审批 |
| `supervisor` | 授权范围 | 只读运行态势和实时订阅 |
| `staff` | 本人任务 | 员工端收件、执行、完成和异常报告 |
| `admin` | 全局系统权限 | 用户、角色、Scope、字典、审计和诊断 |

页面显示当前范围，例如“全局任务”“华东区/值机一组”“本人任务”。范围为空时显示空态，不降级读取其他范围。

## 5. 任务状态表达

```text
pending_dispatch -> assigned -> 我已收到 -> 开始执行 -> 已完成
                         \-> 异常申请 -> manager/admin 审批
```

- `pending_dispatch`：系统正在自动派发；管理端可见短缺，员工端不显示执行按钮。
- `assigned`：员工看到工作地点、岗位和计划时间，操作为“我已收到”。
- `received`：收件回执，不是同意，员工没有拒绝按钮。
- `in_progress`：员工点击“开始执行”。
- `completed`：员工完成操作并等待服务端状态回投。
- `cancelled`：授权变更后的终态；员工冲突、延误、取消和突发事件都走申请。

业务状态、Command 状态和航班源健康分开显示：`pending/syncing/confirmed/failed` 不等于任务业务状态；HTTP `202` 只显示“已提交/同步中”。

## 6. 实时与分页

管理端使用按 Scope 过滤的 SSE；员工 WebSocket 只发送 `task_changed` 提示。提示可以丢失、重复或乱序，客户端收到后重新读取服务端数据。

管理集合、历史、通知使用 `page/page_size/total` 和稳定排序；员工任务使用本人完整快照作为启动、刷新、断线和离线恢复路径。浏览器缓存不是可靠消息队列。

## 7. 可访问性、动效与验收

- 表单有可见标签，键盘可以访问导航、筛选、表格、弹层和主按钮。
- 状态颜色同时提供文字和图标，保留 `:focus-visible`，支持 `prefers-reduced-motion`。
- 使用短时 `transform`/`opacity` 反馈；关键操作立即发请求，不等待动画。
- 不使用持续 WebGL/Canvas、全量列表 stagger、大面积 blur、动态渐变或大型动效资源。
- F0 仍需完成真实身份、域名/TLS、浏览器与原生设备、故障恢复、多副本、性能和最终视觉验收。
