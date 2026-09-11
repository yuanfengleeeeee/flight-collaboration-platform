# ADR-010 Gateway 与 Edge/Worker 横向扩展

状态：T0-5 代码实现完成；双副本、Redis 故障和实例停止场景仍需 Docker 环境验收。

## 决策

- Edge 继续是一个统一的员工接入与 Projection 服务，不按任务类型拆成不同 Edge。
- Gateway 负责把浏览器、员工小程序和 Worker 的请求分发到任意健康的 Edge 副本。WebSocket 必须转发 HTTP/1.1 Upgrade，并保留 Host 以满足同源校验。
- 所有 Edge 副本共享 Edge MySQL。Projection、Command、Session、Inbox 和实时连接 ticket 不保存在单个副本内存中；短期 ticket 使用 Edge SQL 表并只保存 SHA-256 hash。
- Core Worker 使用数据库租约领取 Outbox 和 Edge Command。确认操作必须携带 lease owner，过期租约可被其他 Worker 重新领取，旧 owner 的确认返回 lease lost。
- Redis Pub/Sub 只做跨 Edge 的 task_changed best-effort fan-out。每个副本先投递本地连接，再发布带 origin 的内部 envelope；Redis 不可用时，Projection 和 HTTP 全量快照仍然可用。
- 员工客户端仍以 `GET /api/v1/tasks` 全量快照和 `projection_revision` 为恢复依据。WebSocket 与 Redis 都只是提示通道，不能确认业务成功。

## 本地拓扑

```text
Browser / MiniApp / Worker
             |
          Gateway
          /      \
     Edge API 1  Edge API 2
          \      /
        shared Edge MySQL
              |
         optional Redis fan-out
```

本地 Compose 通过 `gateway` 暴露宿主机 8082，两个 Edge 服务不再占用宿主机固定端口；Worker 的同步地址改为 `http://gateway`。Edge Redis 在本地拓扑中启用，用于验证跨副本提示分发；可靠同步仍然只依赖 MySQL、Outbox、Inbox 和 Command。

## 租约语义

领取条件是 pending/retry 且已到重试时间，或 processing 且 lease 已过期。领取后写入 `lease_owner` 与 `lease_expires_at`；Worker 处理超时后，另一个 Worker 可以重新领取。处理结果必须以 owner 条件更新并清除租约，避免旧 Worker 覆盖新 Worker 的结果。

为保持旧 Store 测试替身兼容，未携带 owner 的旧接口保留；生产 Worker 优先使用带租约接口。Redis、WebSocket 或 Gateway 的短暂故障不会回滚 Core 业务事务。

## 验收边界

已完成代码级闭环：Gateway 配置、双 Edge Compose 服务、持久化租约、共享 realtime ticket、Redis fan-out、Worker owner 传递及租约单元测试。当前环境的 `scripts/ensure-docker.ps1` 报告找不到 `docker.exe`，因此尚未宣称 Compose 配置、双副本网络、迁移和故障注入验收通过。
