# HubCR Registry MVP 发布限制

[English](release-limitations.md) | **简体中文**

HubCR 是具有验收证据的 Registry MVP 候选版本，不是通用生产服务。以下限制属于发布
契约的一部分。

## 已具备的支持证据

- 位于运维人员管理的 HTTPS 反向代理之后，在单台 Docker Compose 宿主机运行单 API
  副本。
- 使用 Docker Engine `29.6.2`、Compose `v5.3.1`、`linux/arm64` 与 Apple Silicon
  宿主的证据，并以 `alpine:3.22` 执行 Push/Pull。
- 本地用户名/密码 Session、Organization 与获批四角色矩阵、Public/Private
  Repository，以及精确 Scope 的短期 Registry Token。
- 由 Distribution 负责的 Push/Pull、认证 Push Event 协调、Repository 级 Artifact/Tag
  读取、Quick-start 与不可变 Digest Detail。
- 维护窗口手动 PostgreSQL 加 Registry 对象备份、Checksum 校验后的破坏性恢复、当前
  迁移应用、登录、私有 Pull 与 Digest 一致性演练。
- 绑定 Digest 的异步 Trivy 0.72.0 漏洞扫描与 CycloneDX SBOM，包括经授权状态 API
  以及真实漏洞/干净 Fixture 证据。
- 异步 Cosign 3.0.6 签名发现、密码学验证与版本化 Namespace 信任评估，包括经授权
  API 与 Web 对未签名、无效、未验证、可信、不可信、不可用和过期状态的如实展示。

## 运维人员必须提供

- HTTPS 反向代理、公开证书、DNS、宿主机防火墙、操作系统加固、监控集成与容量规划。
- 强 PostgreSQL/MinIO 凭据，以及单独保护的 Registry 签名 Key、JWKS 轮换材料、Event
  Secret、环境与 TLS Key。
- 加密、异地传输、访问控制、HubCR 外部保留策略，以及真实备份副本的定期测试。

## 不受支持或尚未证明

- 公共 SaaS、公开注册、受支持的管理员邀请兑换、密码找回、邮箱验证、MFA、所有权
  转移或计费。
- Kubernetes、多 API 副本、共享认证 Limiter 状态、高可用、零停机升级、多区域分发
  或数值化容量保证。
- 自动备份计划、固定 RPO/RTO、跨区域灾难恢复或自动数据库降级。
- Repository 删除、保留、Tag 历史、Distribution 垃圾回收、配额、Audit 导出、Robot
  Account、Access Token、Webhook、复制或代理缓存。
- 产品化 Trust Policy 管理 API/UI、SBOM 下载，或基于 Scan/Trust 的 Pull 阻断。
- 已记录 Apple Silicon 与 Docker 版本之外的 Host/Client 兼容性；Linux 与 Windows
  部署宿主需要独立证据。

Worker 已有持久化 Trivy 扫描/SBOM 与 Cosign/Trust Handler，但结果只展示信息，不影响
Push 或 Pull。Trust Policy Seed 仍是隔离验收 Helper，不是受支持的产品管理 Endpoint。
Redis 不保存权威业务状态。认证 Limiter 仍为进程内状态。完整安全模型与残余风险记录在
[Registry MVP 威胁模型](security-threat-model.zh-CN.md)。
