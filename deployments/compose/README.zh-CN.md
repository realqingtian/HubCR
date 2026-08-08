# 本地基础设施

[English](README.md) | **简体中文**

该 Compose 配置用于在本地开发环境启动 PostgreSQL、Redis、MinIO、CNCF
Distribution 与本地 Gateway。Distribution 强制使用带 Scope 的 Bearer Token：Gateway
把 `/v2/` 路由到 Distribution，把 `/token` 路由到独立运行的 Go 控制面。API、Worker
和 Web 应用不在 Compose 中启动，以便各进程使用原生热更新。

Registry 默认使用宿主机 `5000` 端口，可通过 `HUBCR_REGISTRY_PORT` 修改。在
macOS 上，AirPlay 接收器可能通过 `ControlCenter` 进程占用 `5000` 端口。遇到该
情况时无需关闭 AirPlay，可改用其他本地端口：

```bash
HUBCR_REGISTRY_PORT=5001 HUBCR_ENV_FILE=.env.example make infra-up
```

请在仓库根目录使用 Make 工作流。`infra-up` 会先生成或验证被忽略的本地 RSA 私钥、
JWKS 与随机事件 Token，再把信任材料挂载到 Distribution。全部开发宿主端口都绑定
`127.0.0.1`。Target 默认读取 `.env`；只有使用文档所列本地默认值时才应指定
`.env.example`：

```bash
HUBCR_ENV_FILE=.env.example make infra-config
HUBCR_ENV_FILE=.env.example make infra-up
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
HUBCR_ENV_FILE=.env.example make infra-down
```

请求 Token 前，应在另一终端启动 `make dev-api`。它会启用仅供本地 HTTP 使用的 Token
Endpoint，并复用同一份被忽略的签名材料。直接启动 API 时，除非完整提供 Registry
Auth 配置，否则保持 Fail Closed。
API 与 Distribution 通知 Endpoint 还必须获得相同的 `HUBCR_REGISTRY_EVENT_TOKEN`；
文档中的 Make 工作流会从被忽略的 `.data/registry-auth/event-token` 读取生成值。共享
部署必须注入并轮换独立 Secret。

覆盖 Registry 端口时，应向 `infra-up`、`dev-api` 与 `infra-smoke` 传入相同值，例如
`HUBCR_REGISTRY_PORT=5001`。Distribution 仅监听 localhost 的 Debug Listener 默认单独
使用 `HUBCR_REGISTRY_DEBUG_PORT=5002`。

本地服务地址：

- PostgreSQL：`localhost:5432`
- Redis：`localhost:6379`
- MinIO S3 API：`http://localhost:9000`
- MinIO 控制台：`http://localhost:9001`
- OCI Gateway（`/v2/` 与 `/token`）：`http://localhost:5000`
- Distribution 运维接口（`/metrics` 与 `/debug/vars`）：`http://127.0.0.1:5002`
- Go 控制面：`http://localhost:8080`

覆盖端口后，请使用配置的 `HUBCR_REGISTRY_PORT` 代替 `5000`。

本地 Make 工作流已启用 Registry Token 认证及经过认证的 Distribution Push 事件投递。
Manifest 与 Index 事件会在 PostgreSQL 中协调 Artifact 与当前 Tag 元数据。Pull、Delete
和 Mount 事件会被过滤；生命周期策略获批前，Distribution 删除能力保持禁用。

Gateway 输出不泄露 Secret 的 JSON 访问日志，并用 `registry_challenge=true` 标记
Registry `401` 响应。Distribution 使用 JSON 应用日志，只通过 Loopback Debug 端口暴露
Prometheus 和通知队列状态。启用 Registry Auth 时，Go 直连 Listener 在
`GET /internal/metrics` 暴露有界 Token/通知 Counter；Gateway 刻意不路由该 Endpoint。
详见 [Registry 运维可观测性](../../docs/registry-observability.zh-CN.md)。

## 冒烟检查

Stack 启动后，在仓库根目录执行：

```bash
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
```

预期结果是基础设施健康、PostgreSQL 输出 `accepting connections`、Redis 返回 `PONG`、
MinIO 返回 `200`，Registry 返回 `401` 与精确的 Scoped Bearer Challenge（Realm 为
`http://localhost:5000/token`），且仅监听 localhost 的 Distribution 指标和通知变量可
访问。能力检查返回 `401` 是正确行为；未认证返回 `200` 反而说明 Registry 授权被绕过。

隔离端到端 Target 会通过 GORM 创建仅供测试的用户与 Repository，启动真实 API，使用
Docker Push/Pull 小镜像，并在结束后删除对应容器、卷、镜像标签和临时 Docker 凭据文件：

```bash
make test-m2-registry-e2e
```

它验证 Owner Push、匿名公开 Pull、私有拒绝、Reader 只能 Pull、错误组织拒绝、错误
凭据、跨 Repository 与跨 Action Token 隔离、运行时拒绝过期及签名无效 Token、事件
生成的 Artifact/Tag 元数据及经过授权的 Artifact API 读取。元数据断言覆盖 Repository
级身份、Manifest/Index Descriptor、Tag 状态、被拒绝 Push 不产生持久化、Private `404`
不泄露及经过认证的 Public 访问。测试使用专用端口与独立 Compose Project，永不读写
用户 Docker Credential Store 或 macOS Keychain。
它还验证关联 Token/通知日志、Policy Action Counter、结构化 Challenge 日志、
Distribution 指标和队列可见性，以及 Gateway/API 日志不包含受测凭据与 Bearer Token。

## 停止与本地数据

常规停止命令会删除项目容器和网络，但保留 PostgreSQL、Redis 与 MinIO 命名卷：

```bash
HUBCR_ENV_FILE=.env.example make infra-down
```

下面的命令具有破坏性，会删除 HubCR 全部本地基础设施数据。只有在明确需要全新的
本地数据库、缓存和对象存储时才可执行：

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml down --volumes
```

## 已验证环境

认证 Registry 链路已于 2026-08-01 在 Apple Silicon 上验证，环境为 Docker Engine
`29.6.2`、Docker Compose `v5.3.1`、`linux/arm64` Docker Server、PostgreSQL 17、
Registry 3 与 Nginx 1.29。自动化矩阵使用隔离宿主端口 `55001` 与 `alpine:3.22`；授权、
跨 Repository 与跨 Action Scope 隔离、运行时 Token 过期与签名拒绝、事件驱动
Artifact/Tag 及 Artifact API 检查均通过。
同一隔离运行环境现也通过 Challenge/Token/通知遥测、Distribution 队列可见性、有界
指标及日志不泄露 Secret 的断言；Distribution Debug Listener 使用宿主端口 `55002`。

macOS 上的 `ControlCenter` 可能占用 `5000` 端口；应为 `infra-up`、`dev-api` 与
`infra-smoke` 统一传入其他 `HUBCR_REGISTRY_PORT`。这属于宿主端口冲突，不是 OCI
或 Apple Silicon 兼容性问题。
