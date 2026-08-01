# HubCR 产品需求

[English](requirements.md) | **简体中文**

- 状态：工作基线
- 最后复核：2026-08-01
- 适用范围：HubCR MVP 与明确延期的路线图

本文档将 2026-08-01 的项目讨论整理为与当前仓库真实状态一致的需求基线。文中
描述的是预期产品行为，并不表示所有需求都已实现。当前实现情况以
[README.md](../README.zh-CN.md) 为准，交付顺序则由[开发计划](development-plan.zh-CN.md)
管理。

## 1. 信息来源与决策优先级

本基线结合了项目会话记录以及当前代码、配置、架构和开发规范。当不同来源存在
冲突时：

1. 当前代码和配置定义“已经实现了什么”；
2. 已确认决策与仓库架构定义“允许如何实现”；
3. 本文档定义产品意图与验收边界；
4. 路线图设想在对应决策门通过前都保持延期状态。

未决问题不代表实现者或 AI 可以自行选择产品策略。第 10 节列出的决策必须先获批
并留下记录，之后才能确定依赖它们的公开数据库结构、API 或用户流程。

## 2. 产品定义

HubCR 是面向个人和组织的开源 OCI 容器镜像中心。它在保留身份、命名空间、仓库
可见性、授权、元数据和软件供应链策略自主权的同时，提供类似 Docker Hub 的产品
体验。CNCF Distribution 始终负责 OCI 协议与内容传输。

标准镜像路径为：

```text
hubcr.io/{namespace}/{repository}:{tag}
```

主要用户包括：

- 在个人命名空间发布和使用镜像的个人开发者；
- 通过组织仓库协作的组织成员；
- 管理仓库可见性和访问权限的仓库管理员；
- 评估漏洞、签名和可信状态的安全审查人员；
- 部署和维护 HubCR 实例的运维人员。

## 3. 目标与非目标

### 3.1 MVP 目标

- 提供已认证的用户会话以及自动关联的个人命名空间。
- 提供组织、成员关系和经过确认的基础角色模型。
- 创建和管理显式设为公开或私有的仓库。
- 允许 OCI 客户端认证并获取短期、仓库作用域的 Registry Token。
- 通过 CNCF Distribution 支持镜像 Push 和 Pull，而不重新实现 Distribution API。
- 在控制面和 Web 应用中展示仓库、Tag、Manifest、Artifact Digest 和基础归属元数据。
- 通过 PostgreSQL、Redis、MinIO 和 Distribution 提供可复现的本地开发环境。
- 从第一个可用 MVP 开始就保持授权与不可变 Digest 约束。

### 3.2 MVP 之后的目标

- 异步 Trivy 扫描、SBOM 生成、Cosign 签名发现和策略感知的验证。
- Robot Account、个人访问令牌、审计日志、配额、Webhook、保留策略、垃圾回收、
  镜像复制和代理缓存。
- 邮箱验证、密码找回、MFA、滥用防护、计费、高可用和多区域分发等公网服务能力。

### 3.3 非目标

- 重新实现 `/v2/`、Manifest、Blob、上传、下载或存储驱动。
- 在 MVP 阶段直接拆分为大量独立部署的微服务。
- 将可变 Tag 当成 Artifact 或安全结果的身份。
- 将“发现签名”直接当成“签名有效”或“符合信任策略”。
- 在对应决策获批前承诺公网 SaaS、计费、Kubernetes 或 Pull 阻断行为。

## 4. 当前实现基线

截至 2026-08-01，仓库当前包含：

| 区域 | 当前已实现 | 尚未实现 |
| --- | --- | --- |
| Go API | 进程装配、PostgreSQL 生命周期、健康检查、本地 Session、组织/成员、受 Policy 保护的 Repository 与 Artifact/Tag API、Registry Token 签发及经过认证的 Distribution Push 事件接入 | 账号 Bootstrap/邀请兑换 |
| Go Worker | 可配置轮询循环与优雅退出 | PostgreSQL 任务领取与安全任务 |
| Web | 最小登录态工作区，以及经运行时校验的认证、组织/成员和 Repository 客户端与流程 | 账号 Bootstrap/邀请兑换、公开发现、Artifact/Tag 和 Registry 工作流 |
| OCI 数据面 | 已有 MinIO 支持且受 Token 保护的本地 Distribution Gateway，并通过授权 Docker/OCI 检查及向控制面投递 Push 事件 | Delete 事件协调与获批的生命周期行为 |
| 基础设施 | PostgreSQL、Redis、MinIO、Distribution 的 Compose 定义；控制面 PostgreSQL 连接及覆盖到 Artifact/Tag 元数据的带版本 GORM 迁移 | Worker/Redis 连接、任务 Schema 迁移和生产部署 |
| 质量保障 | Go 与 Web 单元检查、隔离 PostgreSQL 持久化/HTTP/跨租户测试、稳定 Playwright 状态测试、真实 M1 全栈浏览器流程、完整 M2 Docker/OCI 授权矩阵、事件驱动元数据及 Artifact API 检查和仓库级 `make check` | M2 运维遥测及后续安全端到端套件 |

在第 9 节 MVP 退出标准全部通过前，当前脚手架既不能用于生产，也不能被描述成已可用
的多用户 Registry。

## 5. 功能需求

优先级含义：**MUST** 是 Registry MVP 必须具备的能力，**SHOULD** 是不阻塞 MVP 时
应当完成的能力，**DEFERRED** 属于后续里程碑。

### 5.1 身份与会话

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-ID-001 | MUST | 用户可以通过已确认的首期身份方式完成认证，并获得可撤销的 Web 会话。 |
| FR-ID-002 | MUST | 控制面在每个受保护 API 请求中识别当前用户，并拒绝无效、过期或已撤销的会话。 |
| FR-ID-003 | MUST | 每个用户拥有一个名称唯一且规范化的稳定个人命名空间。 |
| FR-ID-004 | MUST | 如果确认采用本地凭据，密码绝不以明文存储，认证接口应具备接入限流的边界。 |
| FR-ID-005 | DEFERRED | 邮箱验证、密码找回、MFA 和其他 OIDC Provider 随公网服务决策进入后续阶段。 |

[D-002](decisions/d-002-registration.zh-CN.md) 与
[D-003](decisions/d-003-initial-identity.zh-CN.md) 已为 MVP 选择管理员邀请、本地用户名/
密码凭据以及可撤销的服务端 Session。

### 5.2 组织、命名空间与授权

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-ORG-001 | MUST | 获得授权的用户可以创建具有全局唯一命名空间的组织。 |
| FR-ORG-002 | MUST | 组织成员拥有明确且经过确认的角色；能力检查不能只依赖 UI 是否显示入口。 |
| FR-ORG-003 | MUST | 获得授权的组织成员可按照已确认的角色矩阵查看和管理成员。 |
| FR-ORG-004 | MUST | 每个命名空间必须且只能归属于一个用户或组织，且归属关系可审计。 |
| FR-AUTHZ-001 | MUST | 每次受保护的控制面与 Registry 访问都根据用户、命名空间、仓库和请求操作执行授权。 |
| FR-AUTHZ-002 | MUST | 授权数据缺失或不可用时必须以拒绝方式失败，绝不能静默授予公开访问。 |
| FR-AUTHZ-003 | SHOULD | 授权决策通过统一的后端策略边界完成，并可在后续产生审计记录。 |

[D-004](decisions/d-004-organization-roles.zh-CN.md) 已确定 MVP 角色矩阵，
[D-005](decisions/d-005-grant-inheritance.zh-CN.md) 已选择仅使用组织角色授权。

### 5.3 仓库

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-REP-001 | MUST | 获得授权的命名空间成员可创建名称唯一且规范化的仓库。 |
| FR-REP-002 | MUST | 每个仓库显式保存 `PUBLIC` 或 `PRIVATE`；数据缺失不能导致隐式公开。 |
| FR-REP-003 | MUST | 用户可以查看和列出其有权发现的仓库；公开仓库发现规则必须符合已确认的产品定位。 |
| FR-REP-004 | MUST | 获得授权的用户可修改可见性，并记录操作时间和操作者。 |
| FR-REP-005 | SHOULD | 可编辑仓库说明和基础元数据，且不改变 OCI 身份。 |
| FR-REP-006 | DEFERRED | 仓库删除、保留、配额、转移和垃圾回收需要先确定运营策略。 |

### 5.4 Registry 认证与 OCI 数据面

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-REG-001 | MUST | `/v2/` 由 CNCF Distribution 提供，并在需要凭据时返回符合标准的认证质询。 |
| FR-REG-002 | MUST | `/token` 对调用方进行认证，并签发仅允许精确仓库及 `pull`、`push` 或 `delete` 操作的短期 Token。 |
| FR-REG-003 | MUST | 公开 Pull、私有 Pull、授权 Push、未授权访问、Token 过期及跨仓库复用均有自动化验收覆盖。 |
| FR-REG-004 | MUST | Blob 的物理去重绝不能绕过仓库级授权。 |
| FR-REG-005 | MUST | Docker 或其他 OCI 客户端可通过受支持的本地网关路径 Push 和 Pull 测试镜像。 |
| FR-REG-006 | SHOULD | Token 签发和授权失败生成结构化且不会泄露密钥的运维日志。 |

Web 会话与 Registry Token 是两种独立凭据。实现 FR-REG-002 前必须记录 Token 有效期、
签名密钥管理和网关拓扑。

### 5.5 Artifact、Manifest 与 Tag

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-ART-001 | MUST | HubCR 以不可变 Digest 记录仓库 Artifact，并幂等地协调相关 Distribution 事件。 |
| FR-ART-002 | MUST | Tag 是指向 Artifact Digest 的可变引用；移动或删除 Tag 不会重写历史安全结果。 |
| FR-ART-003 | MUST | 获得授权的用户可以查看 Tag，并检查 Digest、Media Type、可用时的大小、创建或发现时间以及平台元数据。 |
| FR-ART-004 | MUST | 重复或重试的 Registry 事件不会产生重复的 Artifact、Tag 或任务。 |
| FR-ART-005 | SHOULD | 多平台 Index 展示其子 Manifest，且不会臆造缺失的平台元数据。 |

### 5.6 软件供应链安全

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-SEC-001 | DEFERRED | Manifest Push 成功后按 Artifact Digest 创建异步 Trivy 扫描任务。 |
| FR-SEC-002 | DEFERRED | 扫描记录包含状态、漏洞、严重级别、修复可用性、扫描器版本、漏洞库版本和时间。 |
| FR-SEC-003 | DEFERRED | HubCR 生成或关联绑定不可变 Artifact Digest 的 SBOM。 |
| FR-SEC-004 | DEFERRED | 发现并验证 Cosign 签名和证明材料，且不混淆“存在”“有效”和“可信”。 |
| FR-SEC-005 | DEFERRED | 验证记录包含 Artifact Digest、签名 Digest、身份或密钥证据、策略版本、结果和时间。 |
| FR-SEC-006 | DEFERRED | 失败任务可安全重试，并如实展示排队、执行中、完成、失败和过期状态。 |

除非另行批准的策略明确要求 Pull 决策等待安全结果，否则安全任务始终异步执行。

### 5.7 运维与管理

| ID | 优先级 | 需求 |
| --- | --- | --- |
| FR-OPS-001 | SHOULD | 服务提供依赖感知的就绪检查和进程级存活检查。 |
| FR-OPS-002 | SHOULD | 日志结构化并可关联请求与任务，且绝不包含凭据或 Authorization Header。 |
| FR-OPS-003 | DEFERRED | Robot Account 和访问令牌具有作用域、可撤销，并且仅在创建时展示一次 Secret。 |
| FR-OPS-004 | DEFERRED | 审计日志记录安全相关的操作者、操作、目标、结果和时间。 |
| FR-OPS-005 | DEFERRED | 配额、保留策略、Webhook 投递、复制、缓存和垃圾回收分别需要经过确认的运维策略。 |

## 6. 领域与数据约束

当前持久化模型已包含 `User`、本地凭据、可撤销 Web Session、管理员邀请、
`Organization`、`OrganizationMember`、`Namespace`、`Repository`、`Artifact`、当前
`Tag` 和有序 Manifest Descriptor 记录。安全与运维里程碑随后添加任务、扫描、SBOM、
签名、信任策略、Robot、Token 和审计记录。

强制数据约束包括：

- 内部使用稳定的不透明 ID，路径使用规范化的唯一名称；
- 时间以 UTC 保存，并通过 API 返回明确时区；
- 在数据库边界强制命名空间与仓库唯一性；
- 对必须保持原子性的成员、归属、可见性和 Tag 变更使用事务；
- Artifact 和安全结果绑定经过验证的 OCI Digest；
- Registry 事件与 Worker 写入按可幂等重试设计；
- 不直接把数据库记录暴露成公开传输契约。

## 7. 非功能需求

### 安全与隐私

- 身份、策略、仓库或可见性数据不可用时以拒绝方式失败。
- 验证全部外部输入，包括名称、分页、Registry Scope、Digest、事件 Payload 和 URL。
- 签名密钥和 Secret 必须在源码仓库外安全保存并支持轮换。
- 数据库、Redis、对象存储和 Distribution 均遵循最小权限。
- 第一次对外部署前完成会话认证与 Registry Token 交换的威胁模型。

### 可靠性与一致性

- 优雅退出时停止接收新工作，并限制未完成请求或任务的结束时间。
- 事件处理与 Worker 执行可承受重复投递。
- 只有真实流量所需依赖均可用时，就绪检查才能返回成功。
- 上生产前定义备份、恢复和灾难恢复目标。

### 性能与扩展性

- 镜像字节直接通过 Distribution 和对象存储传输，不经过 Go 控制面。
- 列表 API 使用有界分页和带索引的访问路径。
- 长耗时扫描和验证不得占用同步 API 或 Push Handler。
- 任何数值化延迟、吞吐与容量目标都必须先测量并获批，之后才能作为发布门槛。

### 兼容性与可访问性

- Registry 流程面向符合标准的 Docker 和 OCI 客户端。
- Web 应用支持键盘导航、语义化控件、可见焦点，并区分加载、空、不可用、失败和
  完成状态。
- Apple Silicon 本地开发环境支持 Docker Desktop、Go 和 Bun；其他宿主平台需要
  独立验证证据。

## 8. 必须通过的用户旅程

Registry MVP 必须端到端演示以下流程：

1. 用户完成认证并看到正确的个人命名空间。
2. 获得授权的用户创建组织，并按照已确认角色模型管理成员。
3. 获得授权的用户创建私有仓库，随后将其改为公开。
4. Docker 客户端获取正确作用域的 Token 并 Push 镜像。
5. 获得授权的客户端可 Pull 私有镜像，未授权客户端被拒绝。
6. 公开仓库按照已确认规则允许匿名或已认证 Pull。
7. 事件协调后，Web UI 展示已 Push 的 Tag 和不可变 Digest。
8. 某个仓库或操作的 Token 无法用于其他仓库或操作。

安全里程碑还需增加扫描结果、SBOM、签名有效性、信任策略变化、任务重试和过期
结果等流程。

## 9. Registry MVP 验收标准

只有以下条件全部成立，MVP 才算完成：

- 影响 MVP 数据库结构与公开 API 的决策均已确认并记录；
- 迁移可从空数据库创建身份、命名空间、组织、仓库、Artifact、Tag、会话和任务基础；
- 第 8 节所有必需流程均可通过受支持的本地入口运行；
- 授权测试覆盖公开/私有可见性、成员关系、允许操作、拒绝、Token 过期和跨仓库隔离；
- Registry 事件可按 Digest 幂等协调 Artifact 和 Tag；
- Web UI 如实表示加载、空、拒绝、不可用和成功状态；
- `make check`、后端集成测试、前端测试和 OCI 端到端测试均通过；
- 仓库没有真实 Secret，开发默认值明确标注为不能用于本地环境以外；
- 英文和简体中文文档均覆盖安装、运维、API 行为与限制；
- 发布候选版本具有数据库迁移、备份/恢复及受支持 Docker/OCI 客户端的明确证据。

## 10. 未决策登记表

| 决策 | 最迟确认时间 | 问题 |
| --- | --- | --- |
| [D-001 产品模式](decisions/d-001-product-mode.zh-CN.md) | 身份与发现契约前 | `ACCEPTED`：优先私有化/自托管部署；公共 SaaS 延后。 |
| [D-002 注册方式](decisions/d-002-registration.zh-CN.md) | 用户生命周期结构和 UI 前 | `ACCEPTED`：管理员签发一次性、会过期的邀请。 |
| [D-003 首期身份](decisions/d-003-initial-identity.zh-CN.md) | 会话实现前 | `ACCEPTED`：本地用户名/密码凭据与可撤销服务端 Session。 |
| [D-004 组织角色](decisions/d-004-organization-roles.zh-CN.md) | 成员迁移与 API 前 | `ACCEPTED`：`OWNER`、`ADMIN`、`WRITER`、`READER` 及记录中的能力矩阵。 |
| [D-005 权限继承](decisions/d-005-grant-inheritance.zh-CN.md) | 授权策略前 | `ACCEPTED`：仅使用组织角色；仓库级 Grant 延后。 |
| [D-006 公开 Pull](decisions/d-006-public-pull.zh-CN.md) | Registry Token 流程前 | `ACCEPTED`：明确 `PUBLIC` 的仓库通过精确 Scope 的短期 Token 允许匿名 Pull。 |
| D-007 安全执行 | Pull 策略前 | 首期仅展示扫描信息，还是支持可选或强制阻断 Pull？ |
| D-008 签名信任 | 验签数据结构前 | 固定公钥、组织公钥、OIDC Keyless 身份，还是组合模型？ |
| D-009 生产部署 | 部署契约前 | Compose、Kubernetes 或两者都支持；首期支持哪个？ |
| D-010 运维策略 | 删除与保留功能前 | 配额、删除、保留、垃圾回收、备份和审计的期望是什么？ |
| D-011 开源许可证 | 第一次公开发布前 | HubCR 采用哪一种开源许可证？ |

每个已确认决策都应作为简短的架构或产品决策记录保存在 `docs/decisions/`，从本表
链接过去，并同步更新两个语言版本。

## 11. 需求维护方式

- 在可行时让计划、Issue、测试和变更说明引用需求 ID。
- 只有经过维护者确认并同步更新计划后，才能改变 `MUST` 范围。
- 功能真正可用后及时更新当前实现基线表。
- 将新出现的不确定性加入决策登记表，不要直接在代码中固化假设。
- 每个里程碑退出时以及规划下一个里程碑前复核本文档。
