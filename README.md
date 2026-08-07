# 航空保障智能协同平台

## 基础阶段本地运行

项目当前已完成基础架构，航班、任务、人员等主业务逻辑尚未开始实现。

在 Windows PowerShell 中，如果本机已有 MySQL 占用 3306，可使用 3307 映射：

```powershell
$env:MYSQL_HOST_PORT = "3307"
$env:FLIGHT_MYSQL_PORT = "3307"
docker compose -f deployments/docker-compose.yml up -d mysql redis
go run ./cmd/migrate -command up
go run ./cmd/server
```

常用验证命令：

```powershell
go test ./...
go build ./...
go run ./cmd/migrate -command status
```

健康检查：`/health/live` 检查进程存活，`/health/ready` 检查 MySQL 就绪状态，`/health` 是兼容别名。认证骨架提供 `/api/v1/auth/login` 和受保护的 `/api/v1/auth/me`。

默认不执行 `docker compose down -v`，因为它会删除本地数据库卷；迁移 `down` 也只应在确认数据可丢失的隔离库上执行。
