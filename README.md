# e-cmdb-service

CMDB 配置管理数据库服务 —— 从 [e-cam-service](https://github.com/Havens-blog/e-cam-service) 拆分出来的独立子模块，由运维平台网关统一提供入口。

## 定位

管理多云资产的模型（Model）、实例（Instance）、关系（Relation）与拓扑（Topology）。原先与 CAM 耦合在同一进程，现拆为可独立部署、独立维护的服务。

```
┌──────────────┐      HTTP /api/v1/cmdb/internal      ┌─────────────────┐
│ e-cam-service │ ───────────────────────────────────▶ │  e-cmdb-service │
│ (servicetree) │   RemoteCMDBAdapter → port.CMDBPort   │  (本服务)        │
└──────────────┘                                        └────────┬────────┘
                                                                 │
                                                        ┌────────▼────────┐
                                                        │  MongoDB (ecam) │
                                                        └─────────────────┘
```

CAM 侧只依赖 `port.CMDBPort` 接口。进程内模式走 `LocalCMDBAdapter`，拆分后走 `RemoteCMDBAdapter` 调本服务，业务代码零改动。

## 快速开始

```bash
# 本地运行
go run . start -f config/prod.yaml

# Docker
docker build -f deploy/Dockerfile -t e-cmdb-service:0.1.0 .
docker compose -f deploy/docker-compose.yml up -d
```

默认监听 `:8002`。健康检查：`GET /api/v1/cmdb/health`。

## 配置

`config/prod.yaml`：

```yaml
e-cmdb-service:
  port: "8002"
logger:
  default:
    level: "info"
mongodb:
  dsn: mongodb://host:27017/ecam?authSource=admin
  db: ecam
  username: ecmdb
  password: ****
```

与 e-cam-service 共用同一 `ecam` 库及 CMDB 集合（`ecam_instance` / `ecam_model` 等），**拆分后数据零迁移**。

## 目录结构

```
cmd/            cobra 启动入口
config/         配置文件
ioc/            依赖装配（db / logger / web）
internal/
  domain/       领域模型
  errs/         错误码
  repository/   仓储 + dao
  service/      业务服务
  web/          HTTP handler
  middleware/   租户上下文 + 网关鉴权透传
  module.go     模块装配与路由注册
```

## API 分组

| 前缀 | 鉴权 | 用途 |
|------|------|------|
| `/api/v1/cmdb/*` | 网关租户透传 + `RequireTenant` | 面向终端用户 |
| `/api/v1/cmdb/internal/*` | 服务间可信调用（租户显式传参） | 供 e-cam-service `RemoteCMDBAdapter` 调用 |

内部 API 契约对齐 e-cam-service 的 `port.CMDBPort`：

- `POST /internal/instances/batch-get` — 批量按 ID 查实例
- `GET  /internal/instances/unbound` — 查未绑定服务树的实例
- `GET  /internal/instances/unbound/count` — 未绑定计数
- `POST /internal/stats/by-ids` — 按 ID 列表聚合统计
- `GET  /internal/stats/all` — 全量聚合统计
- `GET  /internal/stats/unbound` — 未绑定聚合统计

## 依赖

- [e-cloudx-sdk](https://github.com/Havens-blog/e-cloudx-sdk) — 多云 SDK（domain 模型）
- [e-common-go](https://github.com/Havens-blog/e-common-go) — 通用库（mongox / ginx）

## 鉴权模型

本服务作为内网微服务置于运维平台网关之后：网关完成 session/JWT 校验后，以 `X-Tenant-Id` / `X-Request-User` 请求头透传身份，`GatewayAuth` 中间件解析入上下文。直连（绕过网关）时租户为 0，被 `RequireTenant` 拦截。

> 安全前提：本服务仅在内网监听，仅接受网关流量；`X-Tenant-Id` 外部不可伪造。
