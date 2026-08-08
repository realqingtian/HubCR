# HubCR Registry MVP 威胁模型

[English](security-threat-model.md) | **简体中文**

- 审查工作包：M3-06 至 M4-05 安全 Workflow 扩展
- 审查日期：2026-08-09
- 范围：Git Snapshot `f691ff0899fb63a9371d29fd5991f5a90ae686b9` 的全部 226 个文件，以及活动 M3 Worktree 中的后续修复
- 方法：全仓静态审查、Source-to-Sink 验证、攻击路径校准、聚焦回归测试，以及真实 Registry/Web 验收 Runner

本文只记录已实现的安全边界。D-009 与 D-010 的 M3-07 子集选择了单机部署和手动恢复
契约。D-007 约束已实现的仅展示信息安全流程，D-008 约束版本化 Namespace Signature
Trust。保留、删除、Pull Enforcement 与产品化 Trust Policy 管理仍不属于有效控制。

## 资产与信任边界

| 资产 | 必须成立的边界 |
| --- | --- |
| 密码与 Web Session | 每次密码校验都通过同一个有界准入控制；浏览器 Session 可撤销，并与 Registry Token 分离。 |
| Registry Token | `/token` 独立认证，只为精确的 Service、Repository 与允许操作子集签发短期 JWT。 |
| 私有元数据 | Artifact/Tag 访问前必须完成 Namespace 与 Repository 授权；一个浏览器 Principal 不能继承前一 Principal 的 Query Cache。 |
| OCI Manifest 与 Blob | Distribution 拥有 `/v2/` 传输；Digest 的物理去重永远不会授予 Repository 访问权。 |
| Registry 事件协调 | Distribution 事件需要独立生成的 Secret，并继续被视为不可信、有界、幂等输入。 |
| Scan 与 SBOM 证据 | 每个 Workflow 绑定精确 Repository 与不可变 Digest；经校验的 Finding、CycloneDX Document 与工具版本持久化在 PostgreSQL。 |
| Signature 与 Trust 证据 | 密码学有效性与版本化 Namespace Policy Trust 独立验证；精确公钥或 Keyless 身份规则及不可变历史结果均绑定 Repository 与 Digest。 |
| Worker Registry 凭据 | Worker 签发限制到精确 Repository `pull` 的短期 Token；不复用浏览器 Session 或宽 Scope Registry 凭据。 |
| PostgreSQL、Redis 与 MinIO 数据 | 发布的开发端口只监听 `127.0.0.1`；生产只发布 Loopback Gateway，数据服务保持在 Compose 私有网络。 |
| CI 执行 | 第三方 Action 使用经过审查的不可变 Commit SHA 标识，并只获得仓库只读权限。 |

## 攻击者能力

- 向公开 Login、Token 与 API Route 发送未认证、并发、畸形或故意缓慢的请求。
- 控制用户名、密码、Registry Scope、请求 Header 与请求时序。
- 使用合法低权限账号尝试跨租户读取。
- 在宿主发布端口时，从同一网络访问开发者工作站。
- 复用仓库内记录的值，并在同一浏览器 Profile 中依次使用两个账号。
- 利用可变第三方 CI 引用的移动或上游失陷。
- 发布刻意复杂或存在漏洞的 OCI Artifact，并在 Registry 传输限制内影响扫描器输出
  大小、耗时与解析输入。

## 已审查威胁与控制

| 威胁 | 修复前 | 已实现控制 | 剩余限制 |
| --- | --- | --- | --- |
| 密码猜测与 Argon2 资源耗尽 | Web Login 使用 Allow-All Limiter，Registry 认证绕过该控制。 | 两条路径都调用 `AuthenticatePasswordAttempt`。单进程每分钟最多接收同一规范化账号 10 次、同一直接 Client 60 次尝试；最多保存 10,000 个 Counter，满载时 Fail Closed，并最多并发执行 4 次密码校验。Registry 返回 `429 TOOMANYREQUESTS`，Web 返回 `rate_limited`。 | 状态只在进程内。多副本或共享部署需要 Redis Limiter，以及明确可信的 Proxy/Client 地址策略。 |
| 跨 Session 浏览器 Cache 泄露 | Session 失效后重新登录只替换 `auth/me`，保留不含身份的私有 Query。 | 每个新认证 Principal 都先移除全部非 Session Query 与缓存的 Mutation，同时保留活动 Session Observer，再安装新的 Current User。 | 将身份加入 Query Key 仍可作为纵深防御。 |
| 慢请求资源耗尽 | Go Server 只限制 Header；Registry Streaming Proxy 设置还作用于 `/token` 与 `/api/`。 | API 现在限制 Read Header、完整读取、写入、Idle 与 Header Size。未缓冲的 900 秒长连接只用于 Distribution `/v2/`；API 与 Token Route 使用有界且带缓冲的默认值。 | 实际容量阈值仍需按部署进行负载测试。 |
| 可从网络访问的开发数据服务 | Compose 使用开发凭据把 PostgreSQL、Redis、MinIO 与 Gateway 发布到全部网卡。 | 所有开发宿主端口都绑定 `127.0.0.1`；生产重置全部基础设施端口，只为外部 HTTPS 反向代理发布 Loopback Gateway。 | 宿主机加固、TLS 终止、防火墙策略与容量仍由运维人员负责。 |
| 使用已知 Token 伪造 Registry 事件 | 提交到仓库的开发 Token 同时被 Distribution 与 API 接受。 | `registry-keygen` 生成随机且被忽略的 `event-token`，权限为 `0600`；Make 将其注入两个进程。示例环境不包含 Token 值，Compose 在未提供时失败。 | 共享部署必须注入并轮换独立 Secret。 |
| 可变 CI 依赖 | Workflow Action 使用可移动的 Major Tag。 | Checkout、Go Setup 与 Bun Setup 固定到完整官方 Commit SHA；`make check-workflows` 阻止回归。 | Pin 更新仍需人工审查对应的上游改动。 |
| 扫描器命令或输出滥用 | Private Image 与外部扫描器可能消耗资源或输出畸形/超大内容。 | 固定版本 Trivy Adapter 使用固定参数、仅环境传入的精确 Scope Token、每 Job 取消、串行 Cache 访问、受限输出、Schema 校验、规范化 Finding，且不记录原始 Stderr。数据库更新不携带 Private Token。 | Artifact 复杂度与扫描器/数据库可用性仍会影响有界 Worker 容量；运维必须监控 Job 并规划资源。 |
| Signature 工具或证据滥用 | Artifact 可携带畸形、超大、误导或密码学无效的 Signature Material，外部 Verifier 同时需要 Private Registry 访问。 | 固定版本 Cosign Adapter 使用固定参数、仅环境传入的短期精确 Scope Token、受限输出、每 Job 取消、Schema 校验及 Scratch 清理。进程内公钥验证只支持已审查算法；精确 Key/Keyless Policy 匹配与有效性分开，且每项结果记录工具与 Policy 版本。 | Artifact 复杂度与 Cosign/Registry 可用性仍会消耗有界 Worker 容量。Keyless 身份验证依赖 Cosign 的证书与 Transparency Log 验证；Trust Policy 管理尚未形成受支持产品 API/UI。 |
| 扫描 Worker 暴露签名 Key | 精确 Scope Pull Token 需要签名，使 Registry 私钥挂载从 API 扩大到 Worker。 | 生产环境只读挂载签名目录；应用代码只签发短期、精确 Repository 的 `pull` Scope；关闭 Distribution Info 日志，避免打印通知 Authorization 配置。 | 获得私钥的被入侵 Worker 进程可绕过应用 Scope 检查签发更宽 Token。单机 MVP 运维必须隔离 Worker，并在入侵后轮换 Registry Key；后续内部 Token Broker 应移除此挂载。 |
| Artifact 到 Workflow 崩溃间隙 | Artifact 持久化后进程崩溃可能导致安全工作缺失。 | Event Handler 会立即确保唯一 Workflow，Worker 周期修复缺失 Workflow 的 Artifact；数据库唯一性保证一个当前 Scan 与 SBOM Intent。 | 即时 Ensure 被中断时，发现延迟取决于配置的 Repair 周期。 |

当前精确 Repository、操作交集、Audience、过期、签名、Private `404` 与跨租户控制中，
授权和 Registry JWT 审查未发现可报告绕过。该结论只适用于已审查源码与已测试 MVP
流程，不适用于未来 Grant、Robot、保留或安全 Policy 功能。

## 验证契约

只有同一 Worktree 中以下检查全部通过，M3-06 才算完成：

- 聚焦 Go 认证、Registry Protocol、配置、HTTP Server 与 Key Generation 测试；
- 前端 Principal Transition 回归以及完整前端测试与 Build；
- 渲染后的 Compose 校验与真实 `make test-m3-artifact-e2e` 流程；
- `make check`、`make check-docs`、`make check-security-config`、
  `make check-workflows` 与 `git diff --check`。

M3-06 本身不声明生产或恢复能力。随后获批的 G-04 子集由
`make test-m3-backup-restore-e2e` 单独实现并验证：构建完整单机拓扑，确认写入服务停止，
为 PostgreSQL 与 Registry 对象备份生成 Checksum，排除 Registry Secret，在轮换签名
材料后恢复到干净隔离卷，应用当前迁移，并证明登录、Private Pull、Artifact/Tag 状态
与不可变 Digest 一致性。

M4-02 至 M4-05 扩展另由 Security、Registry Token、Trivy、Cosign、Trust Policy、配置、
前端契约与 HTTP 授权目标测试；隔离 PostgreSQL Workflow、Repair、并发、历史与结果
状态测试；`make test-integration`；前端单元/Build 与 Playwright 检查；以及真实
`make test-m4-security-e2e` Scan/SBOM、Signature Matrix、重试/重启、Policy 重新评估、
经授权 API 与工具版本 Runner 验证。
