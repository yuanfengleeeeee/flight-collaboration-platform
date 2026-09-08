# 开发容器与测试数据约定

本项目保留 Core/Edge 的数据库边界，但统一了开发容器生命周期：

| 场景 | Compose 项目 | 容器策略 | 数据策略 |
| --- | --- | --- | --- |
| 日常开发 | `flight-dev` | 固定 `app` 应用容器 + 固定 Core/Edge MySQL、Redis | 固定 named Volume，重复启动复用并追加随机演示数据 |
| 普通测试 | `flight-test` | 固定测试容器，不与开发容器共用 | 独立 Core/Edge named Volume，不污染开发数据 |
| 危险/版本测试 | `flight-danger-*` | 每次新项目、新容器 | 新建独立 named Volume，项目默认保留供检查 |

日常开发和普通测试使用：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/dev-up.ps1
powershell -ExecutionPolicy Bypass -File scripts/test-up.ps1
```

危险测试或 migration 版本测试使用：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/dangerous-test-up.ps1
```

三个脚本都会先执行 `scripts/ensure-docker.ps1`，再启动容器、显式执行 Core/Edge migration，并调用现有 `cmd/seed` 逻辑生成随机种子、人员、航班、任务、身份和 Projection 数据。可以通过 `-PersonnelCount`、`-FlightCount` 和 `-Seed` 控制规模；不指定 `-Seed` 时每次运行生成新的随机数据前缀。

## 单应用容器

`deployments/local/docker-compose.dev.yml` 的 `app` 容器包含：

- Core API、Edge API、Worker；
- Gateway；
- Admin Web 和 Employee Web 静态构建产物。

Core MySQL、Edge MySQL 和 Edge Redis 保持独立容器，因为它们是有状态基础设施；MySQL 和 Redis 数据均挂载 named Volume。生产样式、多副本和故障门禁仍使用 `deployments/local/docker-compose.yml` 的拆分服务，不把开发容器布局当成生产拓扑。

脚本不会自动执行 `docker compose down -v`、Volume 删除、migration down、TRUNCATE 或 DROP。需要清理某个测试项目时，必须先确认目标项目和 Volume。
