# Local Architecture v2 部署

`deployments/local/docker-compose.yml` 使用两个 MySQL 容器模拟 Core/Edge 物理隔离：Core 映射宿主机 3310，Edge 映射宿主机 3311，避免占用宿主机已有 MySQL 3306。

Redis 使用 `redis` profile，可选；Core/Edge 可靠同步不能依赖 Redis。

Windows per-user Docker Desktop 安装时，已有 PowerShell 会话可能没有刷新 `docker` 的 PATH；可重新打开终端，或直接调用 `%LOCALAPPDATA%\Programs\DockerDesktop\resources\bin\docker.exe`。先执行 `config` 再执行 `up`，不要停止其他项目的容器，也不要执行 `docker compose down -v`。

```powershell
docker compose -f deployments/local/docker-compose.yml config
docker compose -f deployments/local/docker-compose.yml up -d
go run ./cmd/migrate -target core -config configs/config.v2.yaml -command up
go run ./cmd/migrate -target edge -config configs/config.v2.yaml -command up
```

不要执行 `docker compose down -v`。如果需要测试破坏性 migration，必须使用隔离测试数据库，并显式确认数据可丢失。
