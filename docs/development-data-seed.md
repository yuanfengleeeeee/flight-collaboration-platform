# 开发测试数据

`cmd/seed` 用于给本地开发数据库写入可重复的航空客运地面代理演示数据。它只新增或更新指定前缀的演示记录，不删除数据、不回滚迁移，也不作为生产初始化工具。种子中的航班是开发夹具，生产航班仍只能从外部接口同步。

## 默认数据

- 5 个区域：国内航站楼、国际航站楼、中转服务区、航班地面保障区、客运销售与咨询区。
- 11 个服务组：国内/国际值机、进出港、中转、特殊旅客、不正常航班、配载平衡、行李地面服务、航班地面保障、客运销售代理、航空信息咨询。
- 96 名项目员工、96 个工号凭证、个人微信/企业微信本地绑定。
- 72 个航班和 72 个任务实例，覆盖待自动派发、已分配、执行中、已完成、已取消。
- 已分配任务投影、员工通知投影、候选快照、Assignment、状态历史、审计和 Outbox 演示数据。
- 4 个开发管理身份：管理员、主任、值机服务队长、航班地面保障队长。

## 执行

先确保 Docker Engine 已运行，并在隔离数据库完成 Core/Edge migration，再执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-docker.ps1
$env:FLIGHT_CORE_DB_PASSWORD = "change-me"
$env:FLIGHT_EDGE_DB_PASSWORD = "change-me"
go run ./cmd/seed -config configs/config.v2.yaml -target all
```

默认测试员工账号（工号由 seed 根据前缀生成，以下仅示意格式）：

```text
工号格式：10000001
密码：Flight123!
个人微信模拟身份：mock:demo-wx-0001
企业微信模拟身份：mock:demo-wecom-0001
```

工号必须符合当前人员规则的纯数字格式。开发种子使用 `DEMO` 前缀和固定随机种子 `20260903`，可以重复执行以恢复演示数据；需要另一套数据时使用不同 `-prefix`/`-seed`。不要在真实数据库执行该命令。

## 隔离说明

验证时使用独立 Compose project 和 Core/Edge MySQL Volume；不得删除、重置或覆盖原有 `local-*`、`flight-*` 数据库和服务。任何数据库验证前先执行 `scripts/ensure-docker.ps1`。
