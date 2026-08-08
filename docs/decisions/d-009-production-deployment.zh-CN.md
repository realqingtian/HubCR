# D-009 生产部署

[English](d-009-production-deployment.md) | **简体中文**

- 状态：`ACCEPTED`
- 批准日期：2026-08-08
- 决策负责人：产品负责人
- 阻塞内容：M3-07 部署、迁移与恢复演练

## 背景

HubCR 必须先明确一个 MVP 部署契约，才能有意义地实现并测试备份与恢复。此前的本地
开发 Compose 文件只启动基础设施，API、Worker 与 Web 应用仍作为宿主机进程运行。
如果把该拓扑称为受支持部署，应用镜像、服务顺序、Secret 挂载、入口和恢复边界都会
保持未定义。

## 决策

首个受支持的 MVP 部署目标是运行 Docker Compose 的单台宿主机。生产 Compose 模型
包含 Go API、Worker、迁移命令、Next.js Web 应用、Gateway、CNCF Distribution、
PostgreSQL、Redis 与 MinIO。

只有 Gateway 向宿主机 Loopback 发布端口。由运维人员管理且受信任的 HTTPS 反向
代理必须终止 TLS，并把已批准的公开 Origin 转发到该监听地址。在此拓扑中，HubCR
使用安全 Web Cookie 与 HTTPS Registry 外部 Origin。PostgreSQL、Redis、MinIO、
API、Web 应用和 Distribution 运维端点均只在 Compose 网络内可访问。

Registry 签名材料与部署 Secret 通过受保护的运维文件或环境注入在源码仓库外提供。
生产启动不会生成这些材料，普通数据备份也不会包含它们。

## 备选方案

- Kubernetes 优先：MVP 阶段不采用，因为在单节点契约得到证明前就会增加 Ingress、
  Secret Controller、StorageClass、Rollout 与多副本决策。
- 同时支持 Compose 与 Kubernetes：不采用，因为会使受支持的验收与恢复矩阵翻倍。
- 宿主机进程加仅含基础设施的 Compose：不采用，因为它属于开发拓扑，而不是可复现
  的部署契约。

## 后果

- M3-07 必须构建并启动完整 Compose 拓扑，并针对实际 PostgreSQL 与 MinIO 边界演练
  恢复。
- Kubernetes、高可用、多副本、零停机发布与多区域运行都不属于 MVP 支持声明。
- 进程内认证 Limiter 只对已选择的单 API 副本拓扑成立。后续多副本决策必须采用共享
  Redis 准入状态。
- 外部 TLS 反向代理、证书、DNS、宿主机加固与加密异地备份存储仍是运维人员责任和
  Release Limitation。
