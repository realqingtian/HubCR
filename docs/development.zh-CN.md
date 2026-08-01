# HubCR 开发规范

[English](development.md) | **简体中文**

本文档定义 HubCR 贡献者必须遵守的工程规则。根目录
[`AGENTS.md`](../AGENTS.md) 是仓库级主要入口，`backend/AGENTS.md` 与
`frontend/AGENTS.md` 只在各自目录中增加规则。[产品需求](requirements.zh-CN.md)
定义预期行为，[可执行开发计划](development-plan.zh-CN.md)管理开发顺序、决策门与
完成证据。本文档提供更完整的工程背景、边界和工作流程。

## 1. 架构基线

以下选择是项目约束，而不是建议：

- HubCR 在 MVP 阶段采用模块化单体。
- Go 实现业务控制面和异步 Worker。
- CNCF Distribution 实现 OCI 数据面。控制面不得重新实现 `/v2/`、Blob、
  Manifest、上传或下载行为。
- PostgreSQL 是持久化业务数据库和初期任务队列。
- Redis 用于缓存、限流状态和短期协调。
- OCI 内容通过 Distribution 存储到 S3 兼容存储；本地开发使用 MinIO。
- 耗时的扫描与验签工作在 Worker 中异步执行。
- Web 应用使用 Next.js App Router、React、TypeScript、Tailwind CSS、
  TanStack Query 和 Zod。

修改上述任何决定之前，必须记录架构决策并获得维护者确认。

## 2. 必须遵守的开发流程

每次修改都必须：

1. 阅读 `AGENTS.md`、产品需求、开发计划当前里程碑、相关架构文档和更接近目标
   目录的指令文件。
2. 编辑前检查当前实现与 Git 状态，保留用户无关的现有修改。
3. 说明要改变的行为或不变量，并确认其所属模块。
4. 在修改行为的同时增加或更新聚焦的测试。
5. 保持英文和简体中文文档同步。
6. 开发过程中运行针对性检查，宣布完成前运行 `make check`。
7. 报告已经验证和无法验证的范围，尤其是 Docker 或外部服务行为。
8. 主动询问用户是否需要将已完成的改动提交并推送到已配置的远程仓库。

规划内的产品交付应引用相关需求和工作包 ID。不得通过自行选择未决策略来启动被
阻塞的工作包。只有验收证据发生变化时才能更新计划状态。

除非用户明确授权，否则不得提交、推送、打 Tag、发布、删除数据或修改外部系统。
第 8 步要求的询问本身不代表已经获得授权。

## 3. 仓库与依赖边界

| 路径 | 职责 | 禁止行为 |
| --- | --- | --- |
| `backend/cmd/api` | 处理启动逻辑并运行控制面 | 包含领域行为 |
| `backend/cmd/worker` | 处理启动逻辑并运行后台任务 | 直接实现任务业务规则 |
| `backend/internal/app` | 组合模块与适配器 | 变成通用工具包 |
| `backend/internal/modules` | 拥有业务能力及其契约 | 依赖 HTTP、CLI 或进程入口 |
| `backend/internal/platform` | 配置与基础设施适配器 | 拥有产品策略 |
| `backend/migrations` | 演进 PostgreSQL Schema | 重写已经发布的迁移历史 |
| `frontend/app` | Next.js 路由、布局、加载与错误边界 | 堆积可复用领域逻辑 |
| `frontend/features` | 拥有功能 UI、Hook 与功能模型 | 未经明确边界依赖无关功能 |
| `frontend/lib` | 共享类型化客户端与底层工具 | 变成功能逻辑垃圾场 |
| `frontend/providers` | 应用级客户端 Provider | 把整个应用变成 Client Component |
| `deployments` | 本地与生产部署资源 | 编码应用领域规则 |

后端跨模块行为必须通过明确的应用服务或接口完成。禁止包循环、隐式全局状态和
泛化的 `utils` 包。只有出现明确的部署或伸缩需求时才新增进程或服务，不得默认
拆分模块化单体。

## 4. Go 后端规范

- 使用 `backend/go.mod` 声明的 Go 版本。
- 所有 Go 代码必须经过 `gofmt`，并保持 `go vet ./...` 无错误。
- 优先使用标准库。只有依赖能够明确降低风险或复杂度时才引入，并在改动说明中
  解释新增生产依赖。
- 在请求、持久化和 Worker 边界持续传递 `context.Context`。
- 错误必须增加有助于定位的上下文，同时保留原始错误。
- 使用结构化 `log/slog` 日志。禁止记录凭据、Token、Authorization Header 或
  敏感个人数据。
- 启动时验证环境配置，错误信息必须可操作。
- `main` 函数和 HTTP Handler 保持轻量，业务决策属于业务模块。
- 接口应定义在使用它的包中，而不是基础设施包中。
- 禁止可变包级全局状态和隐式初始化副作用。
- Server 和 Worker 必须支持优雅退出。
- 持久化 ORM 统一使用 GORM 及其 PostgreSQL Driver。GORM Record 留在基础设施
  Adapter，并与领域实体及 HTTP DTO 保持分离。
- Schema 变更使用具有稳定有序 ID 的显式 Gormigrate 条目。API 或 Worker 启动时
  禁止执行不受限制的 `AutoMigrate`。

测试使用同目录的 `*_test.go`，保持确定性，除非明确标为集成测试，否则不得依赖
外部服务。

## 5. 前端规范

- 修改框架代码前，阅读 `frontend/node_modules/next/dist/docs` 中已安装版本的
  Next.js 文档以及 `frontend/AGENTS.md` 的附加规则。
- 默认使用 Server Component。只有确实需要状态、Effect、浏览器 API 或客户端
  库时，才在最小边界添加 `"use client"`。
- 路由放在 `app`，产品行为放在 `features`，共享 API 代码放在 `lib/api`，全局
  客户端 Provider 放在 `providers`。
- 保持 TypeScript Strict，禁止使用 `any` 掩盖模型或 API 不匹配。
- 在边界使用 Zod 验证不可信的 API 响应。
- 客户端服务端状态使用 TanStack Query，禁止在组件状态中重复其缓存。
- 保持可访问性：交互控件必须使用语义元素，支持键盘操作、可见焦点与标签。
- 使用 Tailwind Utility 与现有设计 Token。未经确认不得增加一次性样式体系或新
  组件库。
- UI 状态必须真实区分加载、空数据、不可用、失败和完成。

前端测试使用 Vitest，与对应行为或共享 API Schema 放在一起。开始实现完整用户
流程后再增加 Playwright 覆盖。

## 6. API 与持久化规范

- 业务 REST API 使用 `/api/v1`；`/token` 与 `/v2/` 保留 Registry 协议路径。
- 遵循 [`api.zh-CN.md`](api.zh-CN.md) 中统一的响应、错误、Request ID、JSON、时间与
  分页契约，并同步维护经过审查的 [`openapi.yaml`](openapi.yaml)。
- 所有请求数据都视为不可信，调用领域逻辑前必须验证。
- 传输 DTO、数据库记录与领域实体相互分离。
- 响应中不得泄露内部错误、SQL 细节、密钥或堆栈信息。
- 可重试的写操作应尽可能具备幂等性。
- 所有组织与仓库能力判断统一经过 `internal/modules/authorization`；角色、成员关系、
  Owner、能力或资源数据缺失时默认拒绝。
- 必须原子完成的状态变化使用事务。
- 时间以 UTC 存储，序列化值必须携带明确时区。
- 索引必须基于已知访问模式，唯一性约束在数据库边界验证。
- Namespace 名称是最长 64 Byte 的小写 ASCII OCI 路径组件，匹配
  `[a-z0-9]+(?:[._-][a-z0-9]+)*`。规范化只转换小写；空白、Unicode、路径分隔符与
  连续分隔符直接拒绝，不做隐式改写。
- 第一次公开发布后迁移只增不改，禁止修改可能已在本机之外执行的迁移。
- 迁移通过显式命令在 PostgreSQL Advisory Lock 下执行，并在集成测试中针对隔离的
  空数据库进行验证。

## 7. OCI 与安全不变量

以下规则必须遵守：

- Distribution 负责 OCI 协议与 Blob 传输；HubCR 负责业务元数据与授权。
- 仓库必须明确为 `PUBLIC` 或 `PRIVATE`；不可用数据不能静默变为公开，也不能以
  成功的空值表示。
- Registry Token 必须是短期令牌，并包含精确的仓库和 `pull`、`push`、
  `delete` 等允许操作。
- 浏览器会话不能作为长期 Registry 凭据。
- Blob 的物理去重不能绕过仓库级授权。
- Artifact 使用不可变 Digest 标识，Tag 是可变引用。
- 扫描报告、SBOM、签名和验签结果按 Artifact Digest 保存；验签还必须记录签名
  与信任策略版本。
- 签名发现、密码学有效与策略可信是不同状态。
- 除非已确认的策略明确要求同步阻断，否则 Push 成功不能等待扫描或验签。
- 禁止提交真实密钥；开发默认值必须明确标记为不适用于共享或生产环境。

## 8. 产品决策

G-01 与 G-02 已由 [`docs/decisions`](decisions/README.zh-CN.md) 下的获批记录关闭。
这些记录选择了优先私有化/自托管部署、管理员邀请、本地凭据、四角色组织矩阵、
仅使用组织角色 Grant，以及明确公开仓库允许匿名 Pull。

不得静默选择或编码以下仍未确认的策略：

- 扫描等待或失败时是否阻止 Pull；
- 固定公钥、组织公钥、OIDC Keyless 或组合信任模型；
- 最终生产部署目标是 Compose、Kubernetes 还是两者都支持；
- 计费、配额、保留周期和公开内容治理。

实现依赖这些决策的数据库结构或公开 API 前，必须在 `docs/` 中记录已接受的决定。

## 9. 文档与本地化

- 英文是项目文档的权威语言。
- 每份面向用户的 Markdown 文档都必须有 `.zh-CN.md` 对应版本，并在顶部提供
  语言切换。
- 同一次改动必须同步修改两个语言版本，它们应描述相同的行为、状态、命令和限制。
- 翻译中保持代码标识符、路径、命令、环境变量、协议名称和 API 示例不变。
- 禁止把规划中的功能描述为已经实现。
- 项目边界或可用命令变化时，必须更新 README 状态表和架构文档。

`CLAUDE.md` 与 `GEMINI.md` 等 AI 指令入口导入离它最近的 `AGENTS.md`，
`.github/copilot-instructions.md` 指向根层级。根、后端与前端指令必须保持独立
作用域，不能复制成各适配器单独维护的版本。

## 10. 质量门禁与完成定义

仓库级质量门禁：

```bash
make check
```

它会验证 Go 格式、`go vet`、Go 测试、TypeScript、前端测试、ESLint 和 Next.js
生产构建。

只有满足以下条件，改动才算完成：

- 遵守所属模块和架构边界；
- 行为变化有合适的自动化测试；
- `make check` 在完整依赖环境中通过；
- 受影响的英文与简体中文文档保持同步；
- 没有包含密钥、构建产物、依赖目录或编辑器状态；
- 在可行时完成运行时检查，并明确报告尚未测试的外部服务路径；
- Git 状态只包含预期改动。
