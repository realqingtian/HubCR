# HubCR 可执行开发计划

[English](development-plan.md) | **简体中文**

- 状态：持续维护中的有效计划
- 计划开始：2026-08-01
- 当前阶段：里程碑 4 已退出；M4-01 至 M4-05 均已完成，里程碑 5 能力需要分别确定优先级并完成策略决策
- 需求基线：[HubCR 产品需求](requirements.zh-CN.md)

本计划将产品基线转化为有顺序、可测试的工作项。它是一份交付管理文档：完成工作后
要在这里更新任务状态与证据；需要时可以把实现细节拆成 Issue；在验收证据存在之前，
不能把某项能力标记为完成。

## 1. 如何使用本计划

### 状态标签

- `DONE`：已经实现，并记录了验收证据。
- `IN PROGRESS`：当前正在实现，有一名负责人负责下一次状态更新。
- `READY`：需求和依赖已经足够明确，可以开始。
- `BLOCKED`：被明确的决策或依赖阻塞。
- `PLANNED`：已经排入顺序，但尚未达到可开始状态。

### 计划维护规则

1. 每项任务都必须有 ID、交付结果、依赖、验收检查和证据。
2. 只有决策门和上游依赖完成后，任务才能进入 `READY`。
3. 只有检查通过后，任务才能进入 `DONE`；存在代码不代表已经完成。
4. 实现改变产品行为时，必须在同一变更中同步更新中英文需求和计划。
5. 证据应记录为命令、测试名称、决策记录链接或 Commit ID。
6. 每周或每完成三项任务（以先发生者为准）复核一次当前里程碑。
7. 里程碑退出时更新 README 状态表，并根据最新证据重新规划下一阶段。

下文工作量是假设一名开发者使用 AI 协助进行专注开发时的粗略估算，仅用于规划，
不是交付承诺；不包含等待产品决策、外部审查或环境下载的时间。

## 2. 已验证的起点

| 基线 | 2026-08-01 状态 | 证据 |
| --- | --- | --- |
| 仓库 | `main` 跟踪 `origin/main`，已有脚手架与双语文档 | Git 检查 |
| Go 控制面 | 已有存活/就绪接口、配置、HTTP Server 和优雅退出 | 单元测试及此前运行冒烟测试 |
| Worker | 已有轮询与优雅退出脚手架 | 代码与仓库检查 |
| Web | 登录态 Overview、Namespace、Repository、Artifact/Tag 列表与不可变 Digest Detail 路由使用 TanStack Query 和经 Zod 校验的 API 契约 | 单元测试、生产构建、Mock 状态旅程及真实 Push-to-Web 浏览器证据 |
| Docker 宿主 | Docker Engine `29.6.2`、Compose `v5.3.1`、`linux/arm64` Server | `docker --version`、`docker compose version`、`docker info` |
| Compose 定义 | PostgreSQL 17、Redis 7、MinIO、Distribution 3 配置可成功解析 | `docker compose ... config --quiet` |
| Compose 运行 | 完整 Stack 已在 Apple Silicon 通过冒烟测试；macOS 占用 `5000` 时可覆盖 Registry 宿主端口 | [Compose 冒烟证据](../deployments/compose/README.zh-CN.md#已验证环境) |
| 应用集成 | API 与 Worker 均持有 PostgreSQL 连接池；API 提供依赖感知就绪检查，Worker 提供有界租约执行；Redis 应用状态仍待实现 | 单元测试、实时依赖中断/恢复、优雅退出与 Worker 重启检查 |
| Registry 集成 | Scoped Token 签发、受 Token 保护的本地 Gateway、Push 事件协调、经过授权的 Artifact/Tag 读取 API 及运维遥测已完成 | Go/PostgreSQL 集成测试与 Docker/OCI/API/遥测验收测试 |

项目已完成里程碑 0 至 3。登录态 Web 体验现已覆盖导航、Policy
派生的 Quick-start、Artifact 发现、稳定和真实浏览器验收，以及具有源码证据的安全
整改。命名的 G-04 子集已经获批，完整部署、恢复演练与双语发布文档已有证据。产品
负责人于 2026-08-08 把延期 Job Foundation 移至 M4 并接受 D-007/D-008，因此 M3
退出审计已经完成，M4 实现已获授权。

## 3. 决策门

这些决策门来自[需求的未决策登记表](requirements.zh-CN.md#10-未决策登记表)。产品
负责人确认产品决策，实现工作则在 `docs/decisions/` 下用中英文记录它们。

| 决策门 | 包含决策 | 阻塞内容 | 当前状态 |
| --- | --- | --- | --- |
| G-01 产品入口 | D-001 产品模式、D-002 注册方式、D-003 首期身份 | 会话结构、登录与注册 UI | 2026-08-01 `CLOSED`；[已获批记录](decisions/README.zh-CN.md#m0-决策会议) |
| G-02 授权 | D-004 组织角色、D-005 权限继承、D-006 公开 Pull | 成员结构、授权服务、Registry Token | 2026-08-01 `CLOSED`；[已获批记录](decisions/README.zh-CN.md#m0-决策会议) |
| G-03 安全策略 | D-007 Pull 执行、D-008 签名信任 | 扫描策略与验签契约 | 2026-08-08 `CLOSED`；[已获批记录](decisions/README.zh-CN.md#m4-安全决策会议) |
| G-04 运维 | D-009 生产目标、D-010 运维策略 | 生产部署、删除、保留、GC 与备份 | [M3-07 单机 Compose 与备份子集](decisions/README.zh-CN.md#m3-运维决策会议)已于 2026-08-08 `CLOSED`；生命周期策略仍延期 |
| G-05 开源 | D-011 许可证 | 第一次公开发布 | `BLOCKED`，公开发布前确认 |

决策记录必须说明背景、选项、被否决方案、后果和日期。G-01 至 G-04 已在记录范围内
关闭；后续决策门仍约束各自对应的里程碑。

## 4. 里程碑总览

```mermaid
flowchart LR
    M0["M0 基础"] --> M1["M1 身份与授权"]
    M1 --> M2["M2 Registry 集成"]
    M2 --> M3["M3 Registry MVP 候选版本"]
    M3 --> M4["M4 软件供应链安全"]
    M4 --> M5["M5 运维与公网服务准备"]
```

| 里程碑 | 结果 | 粗略专注工作量 | 进入条件 |
| --- | --- | --- | --- |
| M0 | 可复现的基础设施、持久化和 API 基础 | 1–2 周 | 当前脚手架 |
| M1 | 用户、会话、命名空间、组织、仓库与策略检查 | 4–6 周 | G-01 与 G-02 |
| M2 | 授权 OCI Push/Pull 与基于 Digest 的元数据协调 | 3–5 周 | M1 |
| M3 | 如实的 Web 流程与自动化 Registry MVP 验收 | 2–4 周 | M2 |
| M4 | 异步 Trivy、SBOM、Cosign 与信任状态流程 | 4–7 周 | G-03 与 M3 |
| M5 | Robot 访问、审计、配额、生命周期和可部署性 | 持续增量 | G-04 与 M4 |

G-01 与 G-02 关闭后已重新估算 M1。管理员邀请、本地密码凭据、可撤销 Session 与
四角色授权矩阵提高了原估算的下限。D-006 只保留一种固定的公开 Pull 模式，没有
增加部署可配置分支，因此 M2 仍估算为 3–5 周。

## 5. 里程碑 0——集成基础

目标：将脚手架转化为可复现的开发基础，同时避免提前固化尚未确认的产品策略。

### M0 工作包

| ID | 状态 | 交付结果 | 依赖 | 工作量 |
| --- | --- | --- | --- | --- |
| M0-01 | `DONE` | 需求基线与可执行双语计划符合当前仓库真实情况 | 无 | 1–2 天 |
| M0-02 | `DONE` | Compose Stack 可在 Apple Silicon 启动，服务通过文档化冒烟检查 | Docker Desktop | 0.5–1 天 |
| M0-03 | `DONE` | 可重复执行的 `infra-up`、`infra-down`、`infra-status` 和基础设施冒烟命令 | M0-02 | 0.5 天 |
| M0-04 | `DONE` | PostgreSQL 连接生命周期、配置验证与依赖感知的就绪检查 | M0-02 | 1–2 天 |
| M0-05 | `DONE` | 迁移工具和首期 Schema 规范，具备正向与空库测试 | M0-04 | 1–2 天 |
| M0-06 | `DONE` | 统一 API 响应/错误、Request ID、JSON 和分页规范 | 无 | 1–2 天 |
| M0-07 | `DONE` | CI Workflow 已在获准 Push 的功能分支上通过 Hosted `make check` 与隔离 PostgreSQL Run | M0-01 | 1–2 天 |
| M0-08 | `DONE` | G-01 与 G-02 决策记录已于 2026-08-01 以中英文获批 | 产品负责人 | 0.5–1 天决策会议 |
| M0-09 | `DONE` | 集成测试 Harness 提供隔离 PostgreSQL 并执行迁移 | M0-04、M0-05 | 1–2 天 |

### M0 验收检查

**M0-02 本地基础设施**

这些检查记录的是 M2 引入强制 Token 认证前的 M0 退出配置；当前工作流与响应以 Compose
指南为准。

- 在没有项目容器的状态下执行
  当时的 Compose 工作流。
- PostgreSQL 进入 Healthy；Redis 响应 `PING`；MinIO Bucket 存在；Registry `/v2/`
  返回 M0 配置文档当时描述的响应。
- 在该 M0 未认证开发配置下，可向配置的本地 Registry 端口 Push 并 Pull 一个最小镜像。
- 停止 Stack 时不删除命名卷；另行记录会移除本地数据的显式破坏性命令。
- 在 Compose 指南中记录准确命令、容器状态、端点结果、宿主架构和 Apple Silicon 限制。

**M0-04 持久化基础**

- API 启动时验证数据库 URL 与连接设置。
- PostgreSQL 是必需依赖但不可用时，Readiness 失败；依赖恢复后自动恢复；Liveness
  始终只反映进程状态。
- 退出时在配置的超时时间内关闭连接池。
- 日志包含有用上下文，但不包含数据库密码或完整的带凭据 URL。
- 单元测试覆盖无效配置，集成测试覆盖连接行为。

**M0-05 数据库迁移**

- 选择一种迁移机制，并说明它为何适合 Go 服务和 CI。
- 新数据库可从零迁移到当前版本，重复执行安全。
- Schema 元数据表记录已应用迁移。
- 已发布迁移文件只增不改；本地测试清理绝不指向非测试数据库。

**M0-06 API 契约基础**

- 定义 `/api/v1` 下的错误 Envelope、错误码、Request/Correlation ID、JSON Content
  Type、时间、分页和校验错误行为。
- Handler 测试覆盖格式错误输入、不支持的方法、Not Found、内部失败和 Request ID 传播。
- 在第一个产品接口前，明确 OpenAPI 是由代码生成还是作为需审查的契约维护。

### M0 退出标准

- M0-02 至 M0-09 均为 `DONE`。
- G-01 与 G-02 已有获批的决策记录。
- 干净 Checkout 能按照文档安装依赖、启动基础设施、迁移空数据库、运行 API 与
  Worker，并通过全部 M0 检查。
- README 与 Compose 文档符合经过测试的真实流程。

2026-08-01 的 M0 退出审计已通过：基础设施冒烟、迁移、依赖感知健康行为、API
契约测试、集成测试、`make check` 与文档检查均在本地通过。获准 Push 功能分支后，
提交 `4a8232309101dc41ed2beb60f2b935b4b984e8b6` 的 Hosted GitHub Actions
[Run 30685778563](https://github.com/realqingtian/HubCR/actions/runs/30685778563)
也成功完成，因此 M0-07 已关闭。

## 6. 里程碑 1——身份、归属与授权

目标：实现 Registry 授权所依赖的业务控制面核心。本里程碑不签发 Registry Token。

| ID | 状态 | 交付结果 | 依赖 | 主要需求 |
| --- | --- | --- | --- | --- |
| M1-01 | `DONE` | 按确认的身份模式实现用户、本地凭据、Session 与管理员邀请持久化 | G-01、M0 本地退出 | FR-ID-001–004 |
| M1-02 | `DONE` | 登录、退出、当前用户 API，支持撤销和安全会话处理 | M1-01 | FR-ID-001–002 |
| M1-03 | `DONE` | 个人命名空间创建与名称规范化规则 | M1-01 | FR-ID-003、FR-ORG-004 |
| M1-04 | `DONE` | 按确认角色矩阵实现组织与成员关系结构 | G-02、M1-01 | FR-ORG-001–004 |
| M1-05 | `DONE` | 组织创建/列表/详情与成员管理 API | M1-04 | FR-ORG-001–003 |
| M1-06 | `DONE` | 集中式授权策略服务与表驱动能力测试 | G-02、M1-03、M1-04 | FR-AUTHZ-001–003 |
| M1-07 | `DONE` | 仓库模型、显式可见性和唯一性约束 | M1-03、M1-04 | FR-REP-001–002 |
| M1-08 | `DONE` | 使用策略检查的仓库创建/列表/详情/更新 API | M1-06、M1-07 | FR-REP-001–005 |
| M1-09 | `DONE` | 类型化前端 API 契约与最小认证/组织/仓库流程 | M1-02、M1-05、M1-08 | 必需流程 1–3 |
| M1-10 | `DONE` | 跨租户隔离集成测试套件 | M1-08 | FR-AUTHZ-001–002 |

M1-07 在 2026-08-01 的证据：Repository 领域测试覆盖名称规范化和显式可见性；隔离
PostgreSQL 套件覆盖个人/组织 Namespace、Namespace/名称冲突、数据库检查约束和并发
唯一性；`make check` 通过后端、前端、文档与密钥检查。

M1-08 在 2026-08-01 的证据：表驱动 Service 测试覆盖个人 Namespace 与组织四角色
能力矩阵；HTTP 测试覆盖认证、校验、私有资源不披露、跨站拒绝和变更拒绝；隔离
PostgreSQL HTTP 流程验证 `WRITER` 创建/编辑说明、`OWNER` 修改可见性、无关用户发现
公开仓库、私有仓库过滤以及原子可见性证据。`make test-integration`、`make check`、
双语 API 文档与经审查的 OpenAPI 契约均通过。

M1-09 在 2026-08-01 的证据：前端通过 Zod 校验认证、组织、成员和 Repository
响应，使用携带凭据的类型化客户端，并由 TanStack Query 管理服务端状态。最小工作区
呈现 Session 加载/登录、明确个人 Namespace、组织/成员以及个人/组织 Repository
流程，同时区分空数据、拒绝、校验和不可用状态。Vitest 的 7 个测试、生产 Build、
Playwright 的 3 个稳定浏览器层工作流/失败状态/拒绝测试均通过；人工浏览器检查覆盖未登录、
已认证、桌面及 390 px 响应式状态。浏览器客户端使用同源 `/api` 请求和 Next.js
服务端 Rewrite，因此本地运行不依赖当前不存在的跨 Origin CORS Policy。

M1-10 在 2026-08-01 的证据：隔离 PostgreSQL 套件组合真实 GORM Store、服务端
Session 认证器、Repository Service 与集中 Policy，验证个人 Namespace 隔离、两个
组织租户隔离、OWNER/ADMIN/WRITER/READER 的变更和发现边界、公开/私有发现、缺失
Namespace，以及缺失、无效、过期和撤销 Session。完整 `make test-integration` 套件
通过。

### M1 强制测试矩阵

- 命名空间和仓库名称规范化、非法名称和冲突；
- 用户与组织命名空间归属；
- 每个已确认角色与每项成员/仓库操作的组合；
- 未认证、无效会话、过期会话和已撤销会话；
- 公开与私有仓库发现；
- 策略或数据库数据缺失时以拒绝方式失败；
- 并发创建组织/仓库和数据库唯一性冲突；
- UI 的加载、空、校验、拒绝与服务端失败状态。

### M1 退出标准

- 必需用户旅程 1–3 可通过 API 与 Web UI 完成。
- 任何受保护操作都不只依赖前端是否显示入口。
- 授权行为集中在明确的模块契约后，其能力矩阵得到完整测试。
- 迁移既能升级 M0 Schema，也能从零创建完整的 M1 Schema。

M1 退出审计已于 2026-08-01 通过。`make test-m1-e2e` 创建空的隔离 PostgreSQL，
应用 GORM/Gormigrate Schema，通过 GORM Auth Store 写入两个仅供测试的身份，并启动
真实 Go API 与生产模式 Next.js Server。Chromium 随后完成必需流程 1–3：Session
登录并读取明确个人 Namespace、创建私有 Repository 后改为公开、创建组织并添加
成员。迁移集成测试还会从 M0 Foundation 迁移状态升级到全部 M1 迁移，并验证重复
应用。`make test-integration`、`make check`、7 个 Vitest、3 个 Mock Playwright 状态
测试和真实全栈 Playwright 流程均通过。仅供测试的身份夹具不会作为产品注册或
Bootstrap 入口呈现；管理员邀请 API 不属于这三个 M1 退出流程。

## 7. 里程碑 2——Registry 认证与元数据

目标：连接控制面与 Distribution，同时保持控制面和数据面边界。

| ID | 状态 | 交付结果 | 依赖 | 主要需求 |
| --- | --- | --- | --- | --- |
| M2-01 | `DONE` | [Registry 认证协议设计](registry-authentication.zh-CN.md)，覆盖 Challenge、Service、Audience、Scope 和 TTL | M1、G-02 | FR-REG-001–002 |
| M2-02 | `DONE` | 签名密钥配置及支持轮换的 Token 签名/验证边界 | M2-01 | FR-REG-002、非功能安全需求 |
| M2-03 | `DONE` | `/token` 解析 Scope、认证调用者，并取请求操作与策略允许操作的交集 | M2-02、M1-06 | FR-REG-002–004 |
| M2-04 | `DONE` | Distribution Token 认证配置与本地网关路由 | M2-03 | FR-REG-001、FR-REG-005 |
| M2-05 | `DONE` | [以 Digest 为键的 Artifact、Manifest/Index 和 Tag 持久化](artifact-metadata-persistence.zh-CN.md) | M1-07 | FR-ART-001–005 |
| M2-06 | `DONE` | [经过认证的 Distribution Push 事件协调](distribution-event-reconciliation.zh-CN.md)，支持幂等与重试 | M2-05 | FR-ART-001、FR-ART-004 |
| M2-07 | `DONE` | [Repository Artifact/Tag 列表与详情 API](api.zh-CN.md) | M2-05、M2-06 | FR-ART-003、FR-ART-005 |
| M2-08 | `DONE` | 覆盖公开/私有 Pull、Push、过期和 Scope 隔离的 Docker/OCI 端到端套件 | M2-04、M2-06 | FR-REG-003–005 |
| M2-09 | `DONE` | Challenge、Token 决策与事件处理的[运维日志和指标](registry-observability.zh-CN.md) | M2-03、M2-06 | FR-REG-006、FR-OPS-002 |

### M2 安全验收矩阵

至少自动化以下场景：

| 仓库 | 调用方 | 请求操作 | 预期结果 |
| --- | --- | --- | --- |
| 公开 | 匿名 | `pull` | 与 D-006 完全一致 |
| 公开 | 获得授权的成员 | `push` | 只有具备 Push 能力才允许 |
| 私有 | 匿名 | `pull` | 拒绝 |
| 私有 | 获得授权的成员 | `pull` | 允许 |
| 私有 | 其他组织成员 | `pull` | 拒绝 |
| 任意 | 其他仓库的有效 Token | 任意 | 拒绝 |
| 任意 | 仅 Pull Token | `push` | 拒绝 |
| 任意 | 过期或签名无效的 Token | 任意 | 拒绝 |
| 仓库/策略数据缺失 | 任意 | 任意 | 拒绝且不泄露资源是否存在 |

M2-01 的评审证据是同步的
[Registry 认证协议](registry-authentication.zh-CN.md)。它确定 Challenge 与 Gateway
边界、Service/Audience 与 Issuer 标识、Repository Scope 语法、Action 交集、匿名公开
行为、JWT Claim、默认五分钟 TTL、非对称密钥轮换、失败语义和实现验收矩阵。该设计已
于 2026-08-01 获批。

M2-02 至 M2-04 在 2026-08-01 的证据包括：标准库 RS256 签名、JWKS 活动/退出密钥
验证、启动密钥与配置校验、严格解析重复 Repository Scope、无需 Web Session 的 Basic
凭据认证、策略交集 Claim、不泄露秘密的协议错误与日志，以及位于 Token Auth
Distribution 前的同源本地 Gateway。目标与完整 Go 测试、`go vet ./...`、真实
PostgreSQL/GORM 集成测试及 Compose 配置校验均通过。随后
`make test-m2-registry-e2e` 在 Apple Silicon `linux/arm64` Server 的 Docker Engine
`29.6.2` 上证明 Owner Push、匿名公开 Pull、私有拒绝、Reader 只能 Pull、错误组织拒绝、
错误凭据及跨 Repository Token 隔离。过期、篡改、错误 Audience 与密钥重叠由 Verifier
测试覆盖。

M2-05 在 2026-08-01 的证据包括获批的
[Artifact 元数据持久化契约](artifact-metadata-persistence.zh-CN.md)、类型化
Artifact/Tag/Manifest Descriptor 领域校验、GORM 持久化，以及仅向前 Gormigrate
迁移 `000006_artifact_metadata`。目标领域测试与隔离 PostgreSQL 集成套件证明完全
重放、可空元数据补全、冲突回滚、当前 Tag 移动/删除、无 Tag Artifact 保留、未知与
已确认空 Descriptor Set 的区分、有序 Descriptor 不可变性、跨 Repository 复合外键、
有界分页、迁移重复执行和并发幂等性。

M2-06 在 2026-08-01 的证据包括已经实现的
[Distribution 事件协调契约](distribution-event-reconciliation.zh-CN.md)、经过认证的内部
HTTP Handler、Push 事件映射与 Distribution 重试配置。聚焦测试与真实 PostgreSQL
测试证明了重复投递、Repository 级身份、当前 Tag 移动、陈旧事件保护、有序 Index
Descriptor、请求限制、不泄露 Secret 的失败处理及可重试依赖故障。
`make test-m2-registry-e2e` 还通过受 Token 保护的 Distribution 3.1.1 数据面 Push
公开与私有镜像，并验证事件生成的 Artifact/Tag 状态。Pull、Delete 和 Mount 事件被
过滤；删除与保留仍不在已批准范围内。

M2-07 在 2026-08-01 的证据包括同步的 [API 契约](api.zh-CN.md)及 Repository 级
Artifact/Tag 列表与详情 OpenAPI 3.1 Path。查询 Service 与轻量 HTTP Adapter 会校验
Digest、Tag Name、不透明 Cursor 及返回的 Repository 身份；所有读取都会认证 Web
Session，并在访问 Artifact Store 前复用 Repository 可发现性 Policy。聚焦测试与真实
PostgreSQL 测试证明 Private `404` 不泄露、经过认证的 Public 访问、确定性分页、
Tag 到 Artifact 解析及如实区分未知与已确认空 Index Descriptor。真实 Docker/OCI
套件还证明 Push 事件到 API 读取、被拒绝 Push 不产生持久化，以及日志不泄露 Secret。

M2-08 在 2026-08-01 的证据为：在 Apple Silicon 的 `linux/arm64` Server 上，针对
Docker Engine `29.6.2` 与 Distribution `3.1.1` 运行
`make test-m2-registry-e2e`。套件证明 Owner Push、匿名公开 Pull、授权私有 Pull、匿名
与错误组织私有拒绝、READER Push 拒绝、错误凭据、跨 Repository Token 拒绝、
Pull-only Token 用于 Push 时拒绝，以及在运行时拒绝过期和签名无效 Token。同一次运行
还保留了 M2-06 与 M2-07 所需的事件生成 Artifact/Tag、Repository 级身份、被拒绝 Push
不产生持久化、授权 API 读取及日志不泄露 Secret 证据。Registry 聚焦测试、文档检查及
仓库级 `make check` 门禁也均通过。

M2-09 在 2026-08-01 的证据包括：为 Token Outcome、Policy Action 交集、通知结果、
Processed/Ignored Event 和协调失败 Class 提供固定 Label 的进程内 Prometheus Counter。
Token/Event 日志携带 Request ID 与有界决策字段，不包含 Subject、Repository、Payload、
凭据或 Token。Gateway 以不泄露 Secret 的 JSON 记录 `/v2/` `401` Challenge，Distribution
只在 Loopback Debug 端口暴露通知指标和 Queue 变量。聚焦 Go 测试及
`make test-m2-registry-e2e` 证明信号契约、真实成功/拒绝活动、Queue 可见性、请求关联与
Secret 隔离。运行时套件还修正签名无效检查：改为修改 JWT Signature Segment 的首字节，
避免只改变未使用的 Base64URL 尾部 Bit。

### M2 退出标准

- 必需用户旅程 4–8 可使用 Apple Silicon 上受支持的 Docker 客户端完成。
- 镜像字节由 Distribution 而不是 Go API 传输。
- Token Claim 仅包含策略允许的请求操作子集。
- 重复 Distribution 事件只产生一份正确 Artifact/Tag 状态，且不会丢失合法 Tag 移动。

2026-08-01 完成的 M2 退出审计通过真实 Docker/OCI 套件和聚焦集成测试证明旅程 4–6、8
及其余技术标准。`make test-m3-artifact-e2e` 复用该隔离 Push 与协调状态，再启动生产版
Web，并在 Chromium 中证明已 Push 的 `smoke` Tag 与不可变 Digest 可以被发现。因此
旅程 7 已完成，里程碑 2 已退出。

## 8. 里程碑 3——Registry MVP 候选版本

目标：完成用户可见 Registry 流程、进行加固并生成发布证据，但不宣称已具备供应链
安全能力。

| ID | 状态 | 交付结果 | 依赖 |
| --- | --- | --- | --- |
| M3-01 | `DONE` | Web 导航、认证状态、命名空间与仓库页面 | M1-09、M2-07 |
| M3-02 | `DONE` | 根据真实可见性与用户能力生成仓库快速开始说明 | M2-03、M3-01 |
| M3-03 | `DONE` | Tag/Artifact 列表与 Digest 详情，如实展示不可用与错误状态 | M2-07、M3-01 |
| M3-04 | `DONE` | 登录、组织、仓库与 Artifact 发现的 Playwright 流程 | M3-01–03 |
| M3-05 | `DONE` | 在 CI 或文档化集成环境运行 OCI 验收 | M2-08 |
| M3-06 | `DONE` | 会话、授权和 Token 交换的威胁模型审查与整改 | M1、M2 |
| M3-07 | `DONE` | 受支持 MVP 部署的备份/恢复和迁移演练 | G-04 子集、M2 |
| M3-08 | `DONE` | 双语运维、API、用户文档与发布限制 | M3-01–07 |

M3-01 在 2026-08-01 的证据包括共享登录态 Shell，以及用于 Namespace Discovery 和
Repository Detail 的类型化动态路由。Route Parameter 复用规范 OCI Name Schema；
Client Response 继续由 Zod 校验并交给 TanStack Query 管理。Breadcrumb、键盘 Focus、
移动端换行、Loading、Empty、Unavailable、Retry、无效 Route 与 Private `404` 不披露
状态均明确展示。Repository Detail 如实把 Artifact/Tag Discovery 标为不可用，不推断
Scan、Signature 或 Trust 数据。9 个 Vitest、5 个桌面/390px 宽度 Mock Chromium 旅程、
Next.js 生产构建及真实 PostgreSQL/Go/Next.js M1 浏览器回归均通过。

M3-03 在 2026-08-01 的证据包括经 Zod 校验的 Artifact/Tag 列表与 Digest Detail Client、
TanStack Query 状态所有权及严格校验的动态 Digest Route。UI 分开表示可变 Tag 与不可变
Artifact Identity，区分 Loading、Empty、Access Denial、API Failure、Connection
Unavailable 和 Success，保留 Index Descriptor 未知与已确认空集合的差别，并对不存在或
不可见的 Artifact 返回同一个不披露 `404`。Scan、Signature Validity 与 Trust 明确保持
不可用。14 个 Vitest 与 10 个 Mock Chromium 旅程覆盖契约及 UI 状态；
`make test-m3-artifact-e2e` 还在一次隔离运行中证明真实 Distribution Push、事件协调、
登录态 API 读取及 Web 发现。

M3-02 在 2026-08-01 的证据为 Repository Detail 增加调用者专属 `can_pull` 与
`can_push` 结果。Repository Service 在同一次读取中根据已校验 Namespace Access、显式
Visibility 和集中 Policy 派生二者；API 不暴露角色，浏览器也不重建 Capability Matrix。
Quick-start 区分 Web Session 与 Registry Credential，私有 Pull 要求登录，匿名公开 Pull
省略登录，且仅在 `can_push` 为 true 时展示 Push 命令。Service、Handler 和 PostgreSQL
集成测试覆盖个人 Owner、组织 Writer/Reader 及公开 Outsider 结果。

M3-04 与 M3-05 在 2026-08-01 的证据包括 12 个稳定 Chromium 旅程，覆盖登录、组织、
Repository、Quick-start Capability 分支、Artifact 发现、Descriptor Knowledge、拒绝、
失败、不披露、无效 Route 与移动宽度。`make test-m1-e2e` 保留真实旅程 1–3 证据；已记录
的 `make test-m3-artifact-e2e` Runner 提供隔离 PostgreSQL/MinIO/Distribution Stack，
验证真实 Docker 授权、Push 协调、Policy 派生的私有 Quick-start，以及生产版 Web 中的
Chromium Artifact 发现。

M3-06 在 2026-08-08 的证据包括对 Snapshot 全部 226 个文件的标准全仓安全审查、一个
Canonical Threat Model，以及六条已验证 Finding（三条 Medium、三条 Low）。整改让 Web
与 Registry 密码校验共享有界的进程内尝试/并发门；Principal 替换时移除前一账号的
浏览器 Query 与 Mutation State；把本地 Listener 与 Compose 端口绑定到 Loopback；
只允许 `/v2/` 使用长 Streaming Proxy；在 Git 外生成事件 Callback Secret；并把 CI Action 固定到不可变
Commit，同时加入回归检查。聚焦 Go/前端测试、渲染后的 Compose 校验、真实
Push-to-Web Runner 与完整仓库门禁构成验收证据。[威胁模型](security-threat-model.zh-CN.md)
记录了限制：多副本认证仍需要共享 Redis 状态，容量仍需按部署做负载测试，且没有暗示
任何 G-04 生产或备份选择。

M3-07 在 2026-08-08 的证据包括已接受 D-009/D-010 记录、固定 Digest 且非 Root 的
API 镜像与 Standalone Web 镜像、只发布 Loopback Gateway 的生产 Compose 覆盖文件、
显式迁移顺序，以及 Fail-safe 手动 Backup/Restore 命令。
`make test-m3-backup-restore-e2e` 构建并启动完整拓扑，Push Public/Private 镜像，停止
全部写入服务，为 PostgreSQL 加 Registry 对象备份生成 Checksum，只删除隔离卷，轮换
单独保护的 Registry Key，恢复到干净卷，应用当前迁移，并证明登录、Private Pull、
Artifact/Tag 状态与 Digest 不变。

M3-08 在 2026-08-08 的证据包括同步的 Operator/User Guide、API 入口文档、Release
Limitation、部署说明、Requirements、Architecture、Threat Model 与 README 状态。文档
明确排除账号 Bootstrap、软件供应链安全、生命周期操作、Kubernetes、高可用、自动
备份、数值化 RPO/RTO 与未测试 Host 兼容性。`make check-docs` 校验全部 67 份双语
Markdown 与本地链接。

只有[需求第 9 节](requirements.zh-CN.md#9-registry-mvp-验收标准)的所有 Registry MVP
验收项都有证据时，M3 才能退出。UI 中的安全卡片必须不存在或明确标为不可用，绝不能
伪造尚未实现的扫描或签名状态。

2026-08-08 退出审计最初发现第 9 节把延期 Job Foundation 放在 M3，而其实现由 M4
负责。产品负责人于同日批准把该条件移至 M4，并接受 D-007/D-008。同步后的需求只在
M3 保留已经实现的 Registry MVP Foundation；上述全部 M3 验收证据均已记录，里程碑 3
现已退出。

## 9. 里程碑 4——软件供应链安全

仅在 G-03 获批且 Registry MVP 稳定后开始。

2026-08-08 的就绪审计发现当时只有可取消 Worker 骨架，迁移止于
`000006_artifact_metadata`。此后 M4-01 至 M4-05 已提供持久化 Job 引擎、Trivy
扫描/SBOM Workflow、Cosign 验证、版本化信任评估、经授权安全 API 与 Web 体验、
直至 `000009_signature_trust` 的迁移，以及可复现的真实运行时退出 Runner。

获批执行计划如下：

| ID | 状态 | 交付结果 | 验收证据 |
| --- | --- | --- | --- |
| M4-00 | `DONE` | 把延期 Job Foundation 移至 M4，并接受 D-007/D-008 | 获批双语决策、同步需求、关闭 G-03，以及记录 M3 退出审计 |
| M4-01 | `DONE` | 由 Jobs 模块拥有、Worker 组合的 PostgreSQL 有界任务引擎 | 空数据库迁移；原子领取；唯一 Intent；租约过期/回收；有界并发；确定性重试/退避；取消；最终死信；崩溃/重启集成测试 |
| M4-02 | `DONE` | 绑定 Digest、状态与版本证据如实表达的 Trivy 扫描和 SBOM Job | 重复 Registry Event 只生成一个当前 Intent；修复通知到 Job 的崩溃间隙；固定扫描器执行；漏洞/干净 Fixture；扫描器/数据库版本；排队/执行中/完成/失败/过期 API 测试 |
| M4-03 | `DONE` | Cosign 发现、密码学验证与版本化信任评估 | 已签名、未签名、无效、有效但不可信、有效且可信 Fixture；精确密钥与 Keyless 身份检查；依赖不可用状态；策略变更后重新验证；不可变历史证据 |
| M4-04 | `DONE` | 基于现有 Repository 授权 API、如实表达的 Artifact 安全 Web UI | Zod 契约测试；Loading/Absent/Queued/Running/Failed/Stale/Unavailable/Invalid/Untrusted/Trusted UI 状态；键盘/移动端检查；客户端不推断信任 |
| M4-05 | `DONE` | 可复现的软件供应链安全验收 Runner 与 M4 退出审计 | 真实 OCI Push、Scan/SBOM、签名矩阵、重试/重启、策略重新评估、数据库迁移、`make check`，以及记录运行时/工具版本 |

M4-01 必须以 PostgreSQL 作为权威 Queue，不引入外部 Broker。模块接口拥有 Job 与安全
行为；PostgreSQL、Registry、Trivy 和 Cosign Adapter 保持在 Platform Package。
Claim 必须原子执行并具备租约，Handler 必须幂等，尝试次数必须有界；进程关闭时停止
领取新任务，并对活动任务设置明确上限。

M4-01 在 2026-08-08 的证据包括迁移 `000007_job_foundation`、Jobs 模块、PostgreSQL
原子 `SKIP LOCKED` Claim、唯一 Intent 持久化、租约过期回收、确定性重试/退避、最终
Dead-letter 状态、有界 Worker 并发、取消及不泄露 Secret 的 Outcome 日志。单元测试
证明领域、Handler、配置、并发与关闭行为；隔离 PostgreSQL 测试证明并发 Enqueue/Claim、
租约所有权、重试耗尽，以及停止的 Worker 可由新 Worker 实例回收并完成。
`make check`、`make test-integration` 与干净卷生产 Compose Backup/Restore Runner 均在
Worker 已连接且新迁移已应用的情况下通过。

M4-02 在 2026-08-09 的证据包括 `000008_security_scan`、绑定 Digest 的唯一
Scan/SBOM Intent、Artifact 到 Workflow 崩溃间隙的周期修复、精确 Scope 的内部 Pull
Token、固定 Digest 的 Trivy 0.72.0、受限输出解析、规范化 Finding、CycloneDX JSON、
扫描器/数据库版本证据，以及对 Private Repository 使用 `404` 不披露的经授权安全
Endpoint。单元与隔离 PostgreSQL 测试覆盖全部如实表达的 Job/Result 状态。真实
`make test-m4-security-e2e` Runner 通过生产拓扑 Push 漏洞与干净 Fixture，并证明两个
Workflow、四个唯一 Job、漏洞与零 Finding 结果、两份 SBOM，以及已记录扫描器/数据库
版本。

M4-03 在 2026-08-09 的证据包括 `000009_signature_trust`、不可变版本化 Namespace
信任策略、精确公钥与 Keyless 身份规则，以及有界的 Cosign 3.0.6 执行；短期 Registry
凭据不进入进程参数。进程内验证器支持 ECDSA、RSA 与 Ed25519 公钥，并将密码学有效性
与策略信任分开表达。单元与 PostgreSQL 测试覆盖已签名、未签名、无效、不可用、
未验证、可信、不可信、过期及策略重新评估状态，同时保留历史证据。

M4-04 在 2026-08-09 的证据包括 Repository 授权后的签名契约、严格 Zod 校验、
TanStack Query 集成及 Artifact 安全面板。前端单元测试、类型检查、Lint、生产 Build
与 13 条 Playwright 流程均通过；浏览器检查确认键盘刷新、桌面布局和 390 像素移动端
Viewport 均无水平溢出。

M4-05 在 2026-08-09 的证据包括真实生产拓扑 Runner。它证明 Worker 停止时四个
持久化 Job 保持 Queued、Registry 故障期间发生一次可恢复重试、漏洞与干净 Scan/SBOM
完成，并区分可信、不可信、无效、Attested 与未签名状态；两个不可变策略版本触发
重新评估，经授权 API 返回第二版策略结果，同时记录 Trivy/Cosign 版本。

M4-02 与 M4-03 按 Repository 加不可变 Artifact Digest 保存结果。Scan Record 包含
扫描器和漏洞数据库版本；Signature Record 保留签名标识、签名者证据、密码学有效性、
策略信任、策略版本、验证时间和机器可读原因。缺失或不可用证据绝不能变成成功的零值。

M4-04 只在对应持久化语义通过测试后添加公共契约。读取安全存储前复用 Repository
Discovery 授权；Web 通过 TanStack Query 使用 Zod 校验后的响应，不重新实现策略决策。
按 D-007，M4 全部安全工作保持异步且只展示信息。Pull Enforcement 仍是后续需要
单独批准的变更。

退出证据包括：已签名测试镜像、未签名镜像、无效签名、有效但不受信任的签名、有效
且受信任的签名、存在漏洞的镜像、任务失败/重试，以及信任策略重新评估。

## 10. 里程碑 5——运维与公网服务准备

根据真实运维需要分别安排以下能力：

- Robot Account 和可撤销访问令牌；
- 审计轨迹与安全事件导出；
- 存储和带宽计量、配额与保留策略；
- 安全删除与 Distribution 垃圾回收协调；
- 带签名、重试和死信可见性的 Webhook 投递；
- 镜像复制与代理缓存；
- 限流与滥用防护；
- 受支持的生产部署、升级、备份/恢复与可观测性；
- 如果确认提供公网服务，则加入邮箱验证、找回、MFA、邀请、所有权转移和计费。

每项能力都需要独立的策略决策、威胁审查、迁移计划、运维流程和故障恢复测试。本
里程碑并不是一个单独版本。

## 11. 立即执行队列

里程碑 4 已完成。开始实现前，应先选择并批准一项独立范围的里程碑 5 能力。D-007
继续要求 Scan 与 Trust 结果只展示信息；Pull Enforcement 需要另行完成产品决策。

Commit 应保持足够小，使一个工作包及其测试可一起审查。

## 12. 验证策略

### 每次变更检查

- Go 行为：先运行目标 Package 的 `go test`，再运行 `go vet ./...` 和 `go test ./...`。
- 前端行为：目标 Vitest 测试、TypeScript、ESLint 和生产构建。
- 数据库变更：空数据库迁移及升级路径集成测试。
- Registry 变更：Docker/OCI 验收场景，不能只有 Handler 单元测试。
- 文档：中英文一致性、本地链接、空白字符和文件末尾换行。
- 每次完成仓库变更：`make check` 和 `git diff --check`。

### 里程碑证据包

每个里程碑记录：

- 已确认决策记录与覆盖的需求；
- 命令和自动化测试结果；
- 已应用迁移以及回退/恢复说明；
- 运行版本与支持的宿主/客户端矩阵；
- 安全检查与已知限制；
- 已更新文档；
- 在用户明确授权 Git 操作后记录 Commit 或 Release 标识。

## 13. Ready 与 Done 定义

当行为与非目标明确、决策门关闭、负责人和受影响模块已知、依赖可用，并且可以在
实现前说明验收测试时，一个工作项才是 **Ready**。

当代码和迁移符合模块边界、自动化测试覆盖成功与失败、安全和租户隔离得到考虑、
在可行时检查了运行行为、中英文文档一致、`make check` 通过，并记录证据与剩余限制
时，一个工作项才是 **Done**。完成并不自动授权 Commit 或 Push；Git 操作仍需用户
明确确认。

## 14. 风险登记表

| 风险 | 早期信号 | 缓解措施 |
| --- | --- | --- |
| 产品策略漂移 | Schema 或 UI 引入未确认角色/注册规则 | 强制决策门并让变更引用决策 ID |
| 授权绕过 | 直接查询仓库或访问 Distribution 而跳过策略检查 | 集中策略契约、默认拒绝测试和 Token Scope 矩阵 |
| 控制面/数据面混淆 | Go API 开始代理镜像字节或实现 `/v2/` | 架构审查与边界测试 |
| 事件不一致 | Artifact 重复、Tag 移动丢失或任务重复 | 幂等键、数据库约束和重试集成测试 |
| Secret 泄漏 | 日志/配置 Diff 出现 Token、密码或 URL | 脱敏测试、暂存内容 Secret 扫描和仅开发默认值 |
| 错误安全声明 | UI 根据签名存在或当前 Tag 显示可信 | Digest 模型及存在/有效/可信分离 |
| 只在本机成功 | 行为只在一台机器通过，缺少可复现自动化 | 干净环境 CI 和运行时/客户端兼容矩阵 |
| 范围膨胀 | M1 开始加入计费、GC 或安全阻断 | 里程碑退出标准与明确延期范围 |
| 迁移锁定风险 | 开发过程中改写已发布迁移 | 只增不改规则与升级路径测试 |

## 15. 计划复核清单

每次复核时：

- 根据代码和运行行为确认当前阶段描述；
- 只有具备证据的任务才能移到 `DONE`；
- 找出阻塞任务和准确的决策负责人；
- 确认接下来三个 `READY` 任务仍是到达里程碑的最短路径；
- 使用已完成工作的证据更新估算；
- 协调需求、README 状态、架构和两种文档语言；
- 移除过时假设，同时保留已确认决策历史。
