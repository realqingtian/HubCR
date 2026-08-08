# HubCR MVP 运维指南

[English](operator-guide.md) | **简体中文**

本指南用于运维[已获批的单机 Docker Compose 部署](decisions/d-009-production-deployment.zh-CN.md)。
它属于 MVP 恢复契约，并非高可用设计。向用户开放 HubCR 前，请先阅读
[发布限制](release-limitations.zh-CN.md)。

## 受支持拓扑

生产 Compose 模型在一台宿主机上运行单 API 副本、PostgreSQL 支持的 Worker、单 Web 应用、
Gateway、CNCF Distribution、PostgreSQL、Redis 与 MinIO。只有 Gateway 向宿主机绑定
端口，默认地址为 `127.0.0.1`。

该监听地址前必须部署受信任的 HTTPS 反向代理。反向代理拥有公开证书与 DNS 名称，
并转发包括 `/`、`/api/`、`/token` 与 `/v2/` 在内的完整 Origin。PostgreSQL、Redis、
MinIO、API、Web 与 Distribution 运维端点必须保持在 Compose 私有网络内。

## 准备配置与 Secret

前置条件包括支持 Compose 的 Docker Engine、足够的持久磁盘、HTTPS 反向代理，以及
用于 Registry 签名材料的绝对宿主机目录。

1. 将 `.env.production.example` 复制为已被忽略的 `.env.production`，并限制为只有
   部署账号可读。
2. 替换所有必填空值。`HUBCR_DATABASE_URL` 使用 Compose Hostname `postgres`；密码中
   的保留字符必须进行 URL 编码。
3. 将 `HUBCR_REGISTRY_EXTERNAL_URL` 设置为精确公开 HTTPS Origin，不得包含 Path、
   Query、Fragment 或凭据。
4. 把 `private.pem` 与 `jwks.json` 放入绝对路径 `HUBCR_REGISTRY_AUTH_DIR`。通过受保护
   的运维环境提供至少 32 个可见 ASCII 字符的独立 Event Callback Secret
   `HUBCR_REGISTRY_EVENT_TOKEN`。
5. 保持 `HUBCR_SESSION_COOKIE_SECURE=true`。不安全覆盖只用于隔离 HTTP 验收 Runner。

生产工作流不会生成 Key 或 Secret。私钥、JWKS 轮换集合、Event Token、环境文件、
反向代理配置与 TLS Key 都不进入普通 HubCR 数据备份，必须另行建立受保护恢复流程。

## 校验、构建与启动

在仓库根目录运行：

```bash
make prod-config
make prod-build
make prod-up
make prod-status
```

`prod-up` 先启动持久化依赖，在 PostgreSQL Advisory Lock 下应用迁移，再等待 API、
Web、Registry、Worker 与 Gateway 就绪。生产应用镜像与基础设施镜像都固定到不可变
Image Digest。

Worker 镜像包含 Trivy 0.72.0 与 Cosign 3.0.6，生产环境会启用 Scan、SBOM、Signature
Verification 与 Trust Evaluation Handler。专用 Scratch 与 Cache 路径均非权威数据。
Worker 以只读方式挂载 Registry 签名目录，以便签发只允许对精确 Artifact Repository
执行 `pull` 的短期 Token；必须把该 Container 与 Key Mount 纳入 Registry 签名边界
保护。禁用任一工具会让其 Intent 保持 Queued，等待后续启用的 Worker，而不会伪造
成功证据。

配置外部反向代理后，校验公开 HTTPS Origin：

```bash
curl --fail https://registry.example.com/api/v1/health/live
curl --fail https://registry.example.com/api/v1/health/ready
curl --silent --dump-header - --output /dev/null https://registry.example.com/v2/
```

Liveness 与 Readiness 应返回 `200`。`/v2/` 应返回 `401`，Bearer Challenge 的 Realm
应等于已配置外部 Origin 加 `/token`。

## 停止与检查

```bash
make prod-status
make prod-down
```

`prod-down` 删除 Container 与网络，但保留 PostgreSQL、Redis 和 MinIO 命名卷。除非
明确要丢失数据，或隔离恢复演练正在重置自己的测试项目，否则绝不能对部署使用
`down --volumes`。

## 创建数据备份

获批备份是离线一致的手动操作。执行前必须公告维护窗口，并阻止全部业务与 OCI 写入。

```bash
make prod-maintenance-stop
HUBCR_BACKUP_DIR=/secure/off-host/hubcr-2026-08-08 \
HUBCR_BACKUP_MAINTENANCE_CONFIRMED=true \
make prod-backup
make prod-up
```

API、Worker、Web、Gateway 或 Registry Container 仍在运行时，命令会拒绝执行；它也
拒绝覆盖已存在的目标。成功后会创建仅 Owner 可访问的文件、PostgreSQL Custom Dump、
Distribution Bucket 镜像与 SHA-256 Checksum 清单。

输出包含密码 Hash、Session 记录、私有元数据、Scan/SBOM 证据、Job 状态与 OCI 内容。
必须加密、移出部署宿主机、限制访问，并测试实际加密副本。Redis、可重建 Trivy Cache、
Registry Key、Event Token、环境文件、TLS Key 和反向代理配置不会包含在内。

## 恢复与迁移

恢复会替换目标 PostgreSQL Schema 与完整 Registry 对象 Bucket。应使用空恢复宿主机或
维护窗口，并确认另行保护的 Key、Secret、DNS 与 TLS 材料可用。

只启动持久化依赖：

```bash
scripts/production-compose.sh up --detach --wait postgres redis minio minio-init
```

然后恢复、迁移并启动应用：

```bash
HUBCR_BACKUP_DIR=/secure/off-host/hubcr-2026-08-08 \
HUBCR_RESTORE_CONFIRM=restore \
make prod-restore
make prod-up
```

遇到不支持的备份包、Checksum 不匹配、Symbolic Link 备份目录或仍在运行的应用/写入
服务时，恢复会拒绝执行。数据替换后会应用全部当前迁移。可以使用新 Registry 签名 Key；
此前签发的短期 Registry Token 将失效，Client 需要重新认证。

## 恢复验收清单

在以下项目全部验证前，不得声明恢复成功：

- 当前迁移已存在且 Readiness 为 `200`；
- 已有用户可以登录并获得正确的个人 Namespace；
- 获授权用户可读取 Private Repository，而 Outsider 不可读取；
- Docker 可以认证并 Pull 备份前已存在的镜像；
- 恢复后的 Tag 与 Artifact 可通过 API 和 Web 查看；
- 不可变 Digest 与备份前精确一致；
- Registry Key、Event Secret、TLS 证书与环境来自单独受保护来源，而不是数据备份包。

运行仓库自带的隔离演练：

```bash
make test-m3-backup-restore-e2e
```

它会构建生产镜像、Push 测试镜像、在停写窗口备份、仅删除自身隔离卷、轮换 Registry
签名材料、恢复、迁移、登录、Pull 私有镜像并比较 Digest。

单独使用以下命令验证扫描器链路：

```bash
make test-m4-security-e2e
```

该隔离 Runner 会 Push 漏洞与干净 Fixture，在 Worker 停止时持久化工作，演练一次
Registry 故障重试，并验证 Scan/SBOM 证据、可信、不可信、无效、Attested 与未签名
Signature、两个不可变 Trust Policy 版本、经授权 API 结果，以及 Trivy/Cosign 版本证据。

## 升级与回滚边界

升级前先创建并校验维护窗口备份。构建已审查 Revision，停止写入服务并运行
`make prod-up`；迁移是 Forward-only，并在恢复应用流量前执行。系统没有自动数据库
降级。发生不兼容迁移后，回滚意味着使用匹配的已审查应用 Revision 恢复升级前数据包。

自动调度、保留、固定 RPO/RTO、跨区域恢复、高可用、Kubernetes、删除与垃圾回收不在
已获批 MVP 策略范围内。
