# 技术栈选型

> 版本：V2.0
> 日期：2026-07-29
> 依据：`design-document.md` V1.1
> 决策：采用 Go 技术栈搭建原型(用户决策)
> 备选论证：详见 `tech-selection-report.md`(Java 方案,作为未来企业扩展参考)

---

## 0. 选型总览(Go 原型方案)

| 层 | 选型 | 理由 |
|----|------|------|
| 后端语言 | Go 1.22+ | 用户决策,原型快速搭建 |
| Web 框架 | Gin | 生态最广,文档全,API 简洁 |
| 架构形态 | 模块化单体 | 原型阶段不微服务化 |
| 数据库 | MySQL 8.0 | 事务稳定,运维人才广 |
| ORM | GORM | Go 主流 ORM,用于数据访问和模型映射 |
| 缓存/队列 | Redis 7 | 状态缓存 + 轻量队列 |
| 实时通信 | gorilla/websocket | 态势看板实时推送 |
| 认证 | golang-jwt + Casbin | JWT + RBAC |
| Web 管理端 | Vue 3 + TypeScript + Element Plus | 中后台最低成本 |
| 移动端 | 微信小程序(原生) | 免安装、对接企业微信 |
| 图表 | ECharts | 免费、强大 |
| 配置 | viper + 环境变量 | Go 标准 |
| 日志 | zap | 高性能结构化日志 |
| 部署 | Docker Compose + Nginx | 轻量部署 |
| CI/CD | GitHub Actions(self-hosted runner) | 已有仓库 |
| 监控 | prometheus/client_golang + Grafana | 免费、标准 |
| AI 预测 | 本期 Go 桩 → 未来 Python 微服务 | 接口预留 |
| 硬件接口 | 设备状态上报 API(MQTT/HTTP) | RFID/UWB 预留 |

---

## 1. 后端:Go + Gin

**选择**:Go 1.22+ + Gin + GORM

**对齐要求**:
- 成本低:单二进制部署,资源占用极低
- 好维护:Go 语法简单,显式清晰
- 加功能成本低:模块化结构,新增模块快
- 稳定:Go 运行时成熟,内存安全
- 安全:JWT + Casbin RBAC + 参数校验
- 响应快:协程并发,内存级性能

**关键技术点**:
- 架构形态:模块化单体,模块边界清晰(flight/task/event/personnel/rule/analytics/push/device/prediction/auth/common)
- ORM:GORM(MySQL,数据访问) + 版本化 SQL 迁移(独立命令执行,服务启动不隐式迁移)
- 状态机:自研轻量状态机(人员/任务/事件状态流转)
- 规则引擎:自研轻量规则执行器(预置+阈值)
- 定时任务:robfig/cron(超时检查、状态未知检查、统计聚合)
- API 风格:RESTful,版本化(`/api/v1/...`)
- AI 预测桩:`/api/prediction/analyze` 返回 `not_enabled`

---

## 2. Go 原型目录结构

```
flight-collaboration-platform/
├── cmd/
│   └── server/              # 主程序入口 main.go
├── internal/
│   ├── config/              # 配置加载(viper)
│   ├── server/              # HTTP 服务器 + 路由装配
│   ├── middleware/          # JWT 鉴权、日志、Recovery、CORS
│   ├── model/               # GORM 数据模型
│   │   ├── flight.go
│   │   ├── task.go
│   │   ├── event.go
│   │   ├── personnel.go
│   │   └── ...
│   ├── module/              # 业务模块(模块化单体核心)
│   │   ├── flight/          # 航班模块
│   │   ├── task/            # 任务模块
│   │   ├── event/           # 事件模块
│   │   ├── personnel/       # 人员状态模块
│   │   ├── rule/            # 规则引擎
│   │   ├── analytics/       # 运行数据分析中心
│   │   ├── push/            # 精准推送(通道抽象)
│   │   ├── device/          # 设备状态上报(RFID 预留)
│   │   ├── prediction/      # AI 预测桩
│   │   └── auth/            # 认证/权限
│   ├── common/              # 审计、异常、工具
│   └── store/               # Redis/DB 连接管理
├── pkg/                     # 可复用包(可选公共库)
├── api/                     # API 定义(OpenAPI/请求响应结构)
├── configs/                 # 配置文件
│   └── config.yaml
├── deployments/
│   └── docker-compose.yml   # MySQL + Redis + App
├── web-admin/               # Vue 3 管理端(后续)
├── miniapp/                 # 微信小程序(后续)
├── scripts/                 # 迁移/种子脚本
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 3. 选型与 6 项要求对照

| 要求 | 如何满足 |
|------|----------|
| 成本低 | Go 单二进制;资源占用极低;Docker Compose 免 K8s;小程序免 App 渠道 |
| 好维护 | Go 语法简单显式;单仓多模块;模块边界清晰 |
| 加功能成本低 | 模块化结构;Gin 路由分组;GORM 自动迁移 |
| 稳定 | Go 运行时成熟;协程并发安全;Docker 隔离 |
| 安全 | JWT + Casbin RBAC;HTTPS;数据范围中间件;审计日志 |
| 响应快 | Go 协程;Redis 缓存;WebSocket 实时;单二进制启动快 |

---

## 4. 未来扩展路径(企业阶段)

若未来企业扩展阶段遇到生态瓶颈(规则引擎/状态机/安全),可参考 `tech-selection-report.md` 中的 Java 方案评估迁移,模块化结构保证迁移成本可控。

AI 预测未来抽离 Python FastAPI 微服务,主系统通过 HTTP 调用,Go 主系统不改。

硬件接入:设备接入网关(可 Go/Java)+ device API,主系统不改。

---

*本文档为 Go 原型技术栈,下一步初始化项目骨架。*
