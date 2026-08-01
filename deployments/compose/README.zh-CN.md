# 本地基础设施

[English](README.md) | **简体中文**

该 Compose 配置用于在本地开发环境启动 PostgreSQL、Redis、MinIO 和 CNCF
Distribution。API、Worker 和 Web 应用不会在 Compose 中启动，以便各进程使用
原生热更新独立运行。

在 `deployments/compose` 目录下启动 Stack：

```bash
docker compose --env-file ../../.env.example up -d
```

Registry 默认使用宿主机 `5000` 端口，可通过 `HUBCR_REGISTRY_PORT` 修改。在
macOS 上，AirPlay 接收器可能通过 `ControlCenter` 进程占用 `5000` 端口。遇到该
情况时无需关闭 AirPlay，可改用其他本地端口：

```bash
HUBCR_REGISTRY_PORT=5001 docker compose --env-file ../../.env.example up -d
```

仓库 Make Target 提供日常本地开发使用的可重复流程。它们默认读取 `.env`；只有在
使用文档所列本地默认值时才应指定 `.env.example`：

```bash
HUBCR_ENV_FILE=.env.example make infra-config
HUBCR_ENV_FILE=.env.example make infra-up
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
HUBCR_ENV_FILE=.env.example make infra-down
```

覆盖 Registry 端口时，应向 `infra-up` 与 `infra-smoke` 传入相同值，例如
`HUBCR_REGISTRY_PORT=5001`。

本地服务地址：

- PostgreSQL：`localhost:5432`
- Redis：`localhost:6379`
- MinIO S3 API：`http://localhost:9000`
- MinIO 控制台：`http://localhost:9001`
- OCI Distribution：`http://localhost:5000`

覆盖端口后，请使用配置的 `HUBCR_REGISTRY_PORT` 代替 `5000`。

认证与事件通知目前尚未启用。Registry Token 授权流程确定后，再接入这些能力。

## 冒烟检查

Stack 启动后，在仓库根目录执行：

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml ps --all
docker compose --env-file .env.example -f deployments/compose/compose.yaml exec -T postgres pg_isready -U hubcr -d hubcr
docker compose --env-file .env.example -f deployments/compose/compose.yaml exec -T redis redis-cli ping
curl --fail http://localhost:9000/minio/health/live
curl --fail http://localhost:5000/v2/
```

预期结果是 PostgreSQL 容器 Healthy、输出 `accepting connections`、Redis 返回
`PONG`、MinIO 健康检查成功、`minio-init` 容器成功完成，并且 Registry 返回
`200 OK` 与 `{}`。必要时将 `5000` 替换为配置的 `HUBCR_REGISTRY_PORT`。

可使用一个小镜像验证当前未认证的开发 Registry：

```bash
docker pull alpine:3.22
docker tag alpine:3.22 localhost:5000/hubcr/m0-smoke:local
docker push localhost:5000/hubcr/m0-smoke:local
docker image rm localhost:5000/hubcr/m0-smoke:local
docker pull localhost:5000/hubcr/m0-smoke:local
```

上面的 `docker image rm` 只移除本地测试标签，使下一条命令可以证明镜像能够从
Distribution 拉回。覆盖 Registry 端口时，五条命令都应使用配置后的端口。

## 停止与本地数据

常规停止命令会删除项目容器和网络，但保留 PostgreSQL、Redis 与 MinIO 命名卷：

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml down
```

下面的命令具有破坏性，会删除 HubCR 全部本地基础设施数据。只有在明确需要全新的
本地数据库、缓存和对象存储时才可执行：

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml down --volumes
```

## 已验证环境

完整冒烟测试已于 2026-08-01 在 Apple Silicon 上完成，环境为 Docker Engine
`29.6.2`、Docker Compose `v5.3.1` 和 `linux/arm64` Docker Server。PostgreSQL
进入 Healthy，Redis 返回 `PONG`，MinIO 创建 `hubcr-registry` Bucket，Distribution
返回 `200 OK`。测试将 `alpine:3.22` 推送到 Registry，移除本地测试标签后又成功
拉回，Registry Digest 为
`sha256:2c9d26f410d032d5b1525aa8a873e238b05b90c4ae8618743d4311f0cc827e37`。
常规执行 `down` 后三个命名卷均保留，重启后 Registry Catalog 仍包含测试仓库。

在本次验证的 macOS 宿主上，`ControlCenter` 占用了 `5000` 端口，因此 Registry
部分使用 `HUBCR_REGISTRY_PORT=5001` 完成测试。这属于宿主端口冲突，不是 OCI
或 Apple Silicon 镜像兼容性问题。
