# 员工小程序设计基线

> 当前状态：页面与适配器已实现；真实平台凭据、域名、开发者工具预览和发布验收待完成。适用客户端：`employee-miniapp`、`employee-wecom-miniapp`。

## 1. 设计原则

小程序是员工在航站楼现场的轻量工作入口，不是缩小版管理 Web。个人微信和企业微信使用独立原生登录适配，但共享员工业务页面、Edge Projection、Command 状态和错误语义。

- 首屏优先显示当前要处理的任务、航班号、位置和业务状态。
- 任务详情只提供“我已收到”“开始执行”“完成”和异常报告；没有接受/拒绝二选一。
- `received` 是通知送达回执，不是员工同意；保障冲突、延误、取消和突发事件通过异常/变更申请报告。
- 底部导航固定为任务、通知、历史、异常、账号；主要触控区域不小于 88rpx。
- 实时连接只作刷新提示；启动、刷新、重连和恢复始终通过 HTTP 完整快照校准。
- loading、空数据、网络错误、会话失效、Command pending/syncing/confirmed/failed 都要有可读反馈。

## 2. 页面地图

| 页面 | 个人微信 | 企业微信 | 作用 |
| --- | --- | --- | --- |
| `pages/login/index` | ✓ | ✓ | 快捷登录、工号密码首次绑定、恢复登录 |
| `pages/tasks/index` | ✓ | ✓ | 当前员工任务 Projection |
| `pages/task-detail/index` | ✓ | ✓ | 任务说明、收件/开始/完成、命令回执 |
| `pages/notifications/index` | ✓ | ✓ | 通知读取与已读 |
| `pages/history/index` | ✓ | ✓ | 完成/取消任务历史 |
| `pages/exceptions/index` | ✓ | ✓ | 现场异常和受控变更申请 |
| `pages/account/index` | ✓ | ✓ | 会话、绑定和退出 |

## 3. 登录交互

个人微信：快捷登录使用 `wx.login()`，首次绑定使用工号密码获取一次性 binding ticket，再用新的微信 code 完成绑定；工号密码保留为恢复入口。

企业微信：快捷登录使用 `wx.qy.login()`，首次绑定使用工号密码和企业微信 code；企业微信 Secret 永远不进入小程序包。

两个入口的外部身份都必须绑定到同一个 Core `Staff`；平台标识不直接作为业务身份。

## 4. 业务状态与同步

```text
pending_dispatch -> assigned -> 我已收到 -> 开始执行 -> 已完成
                         \-> 异常申请 -> manager/admin 审批
```

`task_changed` 只触发当前页面重新拉取快照，不是业务事实，也不是离线队列。可靠写入使用稳定 `command_id`；HTTP `202` 只显示“已提交/同步中”，最终状态等待 Core 事件回投到 Edge Projection。

页面显示业务状态和 Command 状态两条轨道：业务状态来自 Projection，Command 状态来自 Command status；本地缓存只做展示优化。

## 5. 当前验收边界

- 两个小程序在 `frontend/apps` 下独立维护，Provider 和客户端标识独立。
- 页面、任务状态、异常申请和恢复规则已形成共享基线。
- HTTP 请求使用 `application/json; charset=utf-8`；不把 AppSecret、Corp Secret、数据库密码或 Token 打包。
- 仍需使用目标平台开发工具完成页面预览、真实 Provider、HTTPS 通信域名、会话存储、网络分区、重连、真机和发布验收。
