# HubCR 架构

[English](architecture.md) | **简体中文**

HubCR 将业务控制面与 OCI 数据面分离。

```text
浏览器 ----------> Next.js Web 应用
Docker CLI -----> 网关 ------- /api/* --> Go 控制面
                         |----- /token --> 作用域令牌接口
                         `----- /v2/* ----> CNCF Distribution --> S3 / MinIO

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
- `backend/migrations`：PostgreSQL 数据库迁移
- `frontend/app`：Next.js 路由与布局
- `frontend/features`：产品功能代码
- `frontend/lib`：共享的类型化 API 客户端与工具
- `frontend/providers`：应用级客户端 Provider
- `deployments/compose`：本地基础设施

## 模块规则

1. 模块拥有自己的领域行为与持久化契约。
2. HTTP Handler 和数据库适配器依赖业务模块，而不是由业务模块反向依赖它们。
3. 跨模块行为必须通过明确的应用服务完成。
4. Distribution 负责 Blob 和 Manifest 的传输行为。
5. HubCR 负责用户、命名空间、可见性、授权和安全策略。
6. 扫描与签名结果必须绑定不可变的 Artifact Digest。

## 延后确认的产品决策

当前骨架不会擅自确定注册策略、组织角色、仓库级权限继承、Pull 阻断策略、
签名信任根或最终部署目标。这些决策会影响数据库结构与 API 契约，必须在实现
对应模块之前确认。
