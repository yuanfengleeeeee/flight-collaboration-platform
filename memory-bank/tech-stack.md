# 技术栈基线

> 来源：根目录 `tech-stack.md`。当前只固化基础设施方向；具体版本和实现细节以验证结果为准。

## 基础选型

- 后端：Go + Gin
- 架构：模块化单体，先保持单仓和清晰模块边界，不提前拆微服务
- 数据库：MySQL，作为核心业务数据唯一事实来源
- ORM：GORM；仅用于数据访问和模型映射，不在服务启动时执行隐式迁移
- 迁移：版本化 SQL 文件，使用独立迁移命令执行；服务启动只检查数据库连接和迁移状态
- 缓存/轻量队列：Redis，可选依赖，不能让核心数据一致性依赖 Redis
- 认证：JWT；角色和资源权限需要形成独立中间件/服务边界
- 配置：Viper + 环境变量；敏感配置不得硬编码
- 日志：Zap，统一结构化日志
- 部署：Docker Compose 作为本地开发和基础验证环境
- 测试：Go 单元测试、HTTP 集成测试、数据库迁移验证
- AI：当前只保留 prediction 接口桩，不进入本期实现

## 基础阶段冻结决策（2026-08-07）

- 模块依赖方向固定为：HTTP handler → module service → repository/store；公共能力只能向下被依赖，业务模块不直接依赖其他业务模块的数据库实现。
- MySQL 保存业务事实、状态和审计所需数据；Redis 只用于缓存、幂等键或可替换的轻量队列，Redis 不可用时核心写入仍必须保持可解释。
- 数据库使用 `migrations/mysql/` 的递增版本 SQL；迁移命令单独运行，服务进程不再调用 `AutoMigrate`。
- 配置采用 YAML 基线 + `FLIGHT_` 前缀环境变量覆盖，敏感值优先从环境变量读取；本地 Compose 的 MySQL 宿主机端口由 `MYSQL_HOST_PORT` 覆盖。
- 认证采用 Bearer JWT；角色先保留 `admin`、`manager`、`leader`、`staff`，权限检查通过独立中间件/服务接口，业务资源数据范围在后续模块中接入。
- 错误响应统一包含稳定错误码、用户可读消息和 request ID；日志使用 Zap 结构化输出，禁止密码、JWT secret 和完整 token 进入日志。
- 测试分为纯单元测试、HTTP `httptest` 集成测试和显式开启的 MySQL/Redis 集成测试；默认测试不得清空用户已有数据库。

## 目标模块边界

```text
cmd/server          程序入口与生命周期
internal/config     配置加载与校验
internal/server     HTTP 服务、路由、依赖装配
internal/middleware 日志、恢复、CORS、认证、权限
internal/store      MySQL/Redis 连接与生命周期
internal/model      持久化模型
internal/common     响应、错误、审计和公共能力
internal/module     后续业务模块
deployments         本地依赖和部署定义
```

## 基础阶段不做的事情

- 不新增业务规则和复杂状态流转
- 不扩展航班、任务、事件 API
- 不接入 AI 模型
- 不接入真实推送渠道
- 不以“能启动”替代迁移、权限、测试和依赖验证
