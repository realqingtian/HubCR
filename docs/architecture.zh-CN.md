# HubCR 架构

[English](architecture.md) | **简体中文**

HubCR 将业务控制面与 OCI 数据面分离。

```text
浏览器 ----------> Next.js Web 应用
Docker CLI -----> 网关 ------- /api/* --> Go 控制面
                         |----- /token --> 作用域令牌接口
                         `----- /v2/* ----> CNCF Distribution --> S3 / MinIO
                                               `-- Push 事件 --> Go 控制面

Go 控制面 --> PostgreSQL
          --> Redis
          --> PostgreSQL 任务表 --> Go Worker --> Trivy / Cosign
```

## 仓库边界

- `backend/cmd/api`：控制面进程入口
- `backend/cmd/worker`：异步 Worker 进程入口
- `backend/internal/app`：只负责应用组合
- `backend/internal/modules`：具有明确所有权的业务能力
- `backend/internal/platform`：配置与基础设施适配器
- `backend/internal/modules/registry`：Registry Token 与 Distribution 事件业务契约
- `backend/internal/modules/artifacts`：Artifact/Tag 校验与持久化契约
- `backend/internal/platform/httpapi/artifacthandler`：经过授权的 Artifact/Tag 读取 Adapter
- `backend/internal/platform/httpapi/registryeventhandler`：经过认证的内部事件 Adapter
- `backend/internal/platform/observability`：有界进程指标及内部 Exposition Adapter
- `backend/internal/platform/postgres/artifactstore`：GORM/PostgreSQL Artifact Adapter
- `backend/migrations`：PostgreSQL 数据库迁移
- `frontend/app`：Next.js 路由与布局
- `frontend/features`：产品功能代码
- `frontend/lib`：共享的类型化 API 客户端与工具
- `frontend/providers`：应用级客户端 Provider
- `deployments/compose`：本地基础设施

当前 Web 路由边界使用共享登录态 Shell 包裹 `/`、`/namespaces/[namespace]` 与
`/namespaces/[namespace]/repositories/[repository]`，并在 Repository 路由下通过
`/artifacts/[digest]` 提供不可变 Artifact 详情。Route 文件校验动态 OCI Name 与 Digest
Component，并把 API 驱动的展示委托给 `namespaces`、`repositories` 与 `artifacts`
Feature；Route 不拥有授权决策。Artifact Client 使用 Zod 校验每个响应，TanStack Query
管理 Loading、Error、Retry 与成功的服务端状态。Repository Detail 只包含集中 Policy
的结果（`can_pull` 与 `can_push`），使 Quick-start 无需在浏览器复制角色矩阵即可选择命令。

## 模块规则

1. 模块拥有自己的领域行为与持久化契约。
2. HTTP Handler 和数据库适配器依赖业务模块，而不是由业务模块反向依赖它们。
3. 跨模块行为必须通过明确的应用服务完成。
4. Distribution 负责 Blob 和 Manifest 的传输与存储行为。
5. HubCR 负责 Repository 级 Artifact/Tag 业务元数据。经过认证的 Distribution Push
   事件负责协调这些元数据；经过授权的读取 API 在不接管 OCI 传输的前提下对外提供它们。
6. HubCR 负责用户、命名空间、可见性、授权和安全策略。
7. 扫描与签名结果必须绑定不可变的 Artifact Digest。
8. Registry 运维信号遵循相同所有权边界：Gateway 与 Distribution 观测 `/v2/`
   Challenge 和投递队列；Go 控制面观测 Token 决策与通知协调。
9. 认证模块拥有密码尝试准入。Web Login 与 Registry Basic Authentication 在执行
   Argon2 前汇聚到同一个有界进程内 Limiter 与并发门；未来多副本部署必须使用获批的
   Redis 共享 Adapter 替换该状态。

本地基础设施边界把 Go Listener 与每个 Compose 发布端口绑定到 `127.0.0.1`。
Registry Streaming Timeout 只作用于 `/v2/`；Token 与业务 API Route 使用有界且带
缓冲的 Proxy 行为。已审查攻击路径与剩余部署限制见
[Registry MVP 威胁模型](security-threat-model.zh-CN.md)。

## 延后确认的产品决策

当前骨架不会擅自确定注册策略、组织角色、仓库级权限继承、Pull 阻断策略、
签名信任根或最终部署目标。这些决策会影响数据库结构与 API 契约，必须在实现
对应模块之前确认。
