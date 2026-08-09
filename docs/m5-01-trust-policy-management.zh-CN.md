# M5-01 Trust Policy 管理 — 设计与实现计划

**简体中文** | [English](m5-01-trust-policy-management.md)

- 状态：`PROPOSED` 设计；产品负责人已于 2026-08-09 批准 M5-01 的范围与边界。本文档记录已确认的边界、选定的重新验证触发机制，以及有序的实现计划。
- 决策依据：[D-008 Signature trust](decisions/d-008-signature-trust.zh-CN.md)（`ACCEPTED`）定义了按 namespace 版本化的信任模型与信任评估。M5-01 在已建成并通过 M4 验收的模型之上增加经授权的管理入口，不改变 D-008 与 D-007。
- 计划参考：[开发计划第 10 节](development-plan.zh-CN.md)（Milestone 5 — 运维与公网服务就绪）。

## 1. 背景与非目标

### 已存在的内容（不要重新设计）

D-008 已 `ACCEPTED`，领域模型、存储与验证引擎均已通过 M4 验收：

- 领域 — `backend/internal/modules/security/trust.go`：`TrustPolicy`（按 namespace 版本化）、`PublicKeyTrust`（sha256 指纹）、`KeylessIdentity`（精确 issuer + subject，无通配符）、`EvaluateTrust` 纯函数、按 policy ID 作用域的 `VerificationIntentKey`。上限：`MaxTrustPolicySubjects=128`、`MaxPublicKeyBytes=16KiB`。
- 存储 — `backend/internal/platform/postgres/securitystore/truststore.go`：`CreateTrustPolicy` 锁定 namespace 行（`SELECT ... FOR UPDATE`）并计算 `MAX(version)+1`。所有外键为 `ON DELETE RESTRICT`（历史不可变）。"当前版本" 按 namespace 取 `ORDER BY version DESC LIMIT 1` 派生。
- 验证 — worker handler `trust_handler.go` + cosign adapter。cosign 仅做发现与密码学有效性；信任通过 `EvaluateTrust` 单独评估。
- 补齐 — `combinedSecurityRepairer`（`backend/internal/app/worker/app.go`）周期性运行，当当前策略变更时会重新验证 artifact，因为新策略版本有新的 `policy_id`，对应的 `(artifact, current_policy)` 工作流行在补齐前不存在。
- 只读入口 — 只读 `GET .../artifacts/{digest}/security`（`platform/httpapi/securityhandler/handler.go`）与只读前端面板 `features/security/artifact-security-panel.tsx`。详情读取器已在 `currentPolicy.ID != workflow.PolicyID` 时把结果标记为 **stale**。

### M5-01 新增内容（缺口）

- 无 HTTP 管理 API（无策略的 create / list / read 端点）。
- `modules/authorization/policy.go` 中无 `MANAGE_TRUST_POLICY` capability。
- 无前端 Trust Policy 管理 UI。
- `TrustService.CreatePolicy` 当前不做任何授权（设计如此 — 仅 seed CLI `testsupport/m4trustseed` 调用过它）。
- 策略变更后的重新验证仅在下个周期性 repair tick 懒触发；M5-01 增加一个即时的、namespace 作用域的触发，使延迟有界且可预测。

### 非目标（延后，需单独批准）

- 编辑或删除历史策略版本 — 保持只追加不可变历史。
- Repository 级或覆盖级信任策略 — 仅 namespace 作用域。
- 通配 OIDC issuer 或 subject 模式 — 仅精确匹配。
- 基于信任状态阻断 Pull — D-007 保持结果为信息性；重新阻断需单独决策。
- 按 namespace 或 policy 列举验证结果 — 仅按 artifact 读取。
- 策略管理 CLI — M5-01 交付 API + Web；CLI 可后续追加。

## 2. 已确认边界

下列边界由产品负责人于 2026-08-09 批准，并在范围讨论会中确认：

| 边界 | 取值 |
| --- | --- |
| 谁可管理 | 仅 Namespace `OWNER`。个人 namespace 为个人 owner；组织 namespace 为组织 `OWNER` 角色。 |
| 历史模型 | 只追加不可变版本。不可编辑、不可删除历史版本。外键保持 `ON DELETE RESTRICT`。 |
| 信任主体 | 精确公钥指纹；精确 OIDC issuer + subject。无通配符、无隐式信任。 |
| 重新验证触发 | 创建策略时，即时触发一次 namespace 作用域的 repair，剩余部分由周期性 repair 循环兜底完成。 |
| Pull 行为 | 不变 — 结果保持信息性。不阻断（D-007 有效）。 |
| 状态分离 | 签名存在性、密码学有效性、策略信任保持为独立状态，按 digest 索引。 |

## 3. 重新验证触发机制（已确认）

待决的关键问题是新策略版本创建后重新验证如何触发。已确认的选择是 **方案 C：先提交策略，再即时触发一次 namespace 作用域的 repair**，并以周期性 repair 循环作为兜底。

### 选择该方案的原因

- 机制已存在。`combinedSecurityRepairer.RepairMissingWorkflows` 已会对缺失 `(artifact, current_policy)` 工作流的 artifact 重新验证。新策略版本在提交后立即成为"当前版本"，其工作流行在补齐前即处于缺失状态。
- 即时触发限定延迟。仅依赖周期性循环（方案 A）对大 namespace（多个 tick 跨越多批 artifact）延迟不可控。即时触发在创建策略时即开始工作，而非等待下个 tick。
- 事务保持干净。策略写入先提交；repair 作为独立、分离的操作运行。在 `CreateTrustPolicy` 事务内入队（方案 B）会跨模块、跨表，风险更高。
- 负载有界。复用现有 `cfg.Scanner.RepairBatch` 作为单次上限，剩余由周期性循环排空。不引入新的并发参数。

### 与现有路径的组合方式

1. `CreatePolicy` 提交新版本（事务关闭）。
2. 新版本此时即为 namespace 的当前策略（`ORDER BY version DESC LIMIT 1`）。
3. `RepairTrustVerificationForNamespace(namespaceID, batch)` 运行一次，仅作用于该 namespace：选取缺失工作流的 `(artifact, current_policy)` 对，逐一调用现有 `EnsureCurrentVerification`。
4. 每个 `EnsureCurrentVerification` 在单个事务内入队一个 `COSIGN_VERIFY` 任务（带按 policy 作用域的 `VerificationIntentKey`：`security-signature:<repo>:<digest>:policy:<newPolicyID>`）并插入工作流行。去重幂等。
5. worker 按现有方式认领并执行任务；结果按 digest + policy 版本落地。读取路径在刷新前把旧版本结果标记为 `stale`。
6. 周期性 `combinedSecurityRepairer` 继续运行，排空首批 `RepairBatch` 之后的剩余部分，并覆盖后续推送的 artifact。

### 失败与并发语义

- 策略提交与即时 repair 解耦。若进程在提交与即时 repair 之间退出，周期性循环仍会补齐 — 不会丢失工作。
- 即时 repair 幂等：`EnsureCurrentVerification` 通过 `ON CONFLICT DO NOTHING` 按 `(repo, digest, policy_id)` 去重，任务队列按 `intent_key` 去重。即时 repair 与周期性循环并发运行是安全的。
- 即时 repair 失败不会回滚策略。策略已提交且为当前版本；验证以异步方式追赶。这与 D-008 一致：变更策略是排队重新验证，而非阻塞等待。

## 4. 后端设计

### 4.1 授权 capability

在 `modules/authorization/policy.go` 增加一个 capability 并复用一个现有 capability：

- `ManageTrustPolicy Capability = "MANAGE_TRUST_POLICY"`（新增）— 门控 POST（创建新版本）。
- `organizationCapabilities[ManageTrustPolicy] = { RoleOwner: true }` — 仅组织 owner。
- 扩展 `AllowsPersonalNamespace` 的 owner 分支以包含 `ManageTrustPolicy`，使个人 namespace owner 可管理自身策略。
- 复用现有 `ViewOrganization` capability（已定义并授予所有组织角色，但当前从未被强制）来门控 GET（读取当前策略）。不修改其定义；本端点成为其首个强制点。

`ManageTrustPolicy` 仅限 `OWNER`（已批准边界）。`ViewOrganization` 将读取授予所有组织成员及个人 namespace owner；非成员被拒绝。读取权限的理由见第 9 节。

### 4.2 Namespace 作用域 repair

在现有全局 repair 旁新增 namespace 作用域的 repair 方法。它镜像 `RepairMissingVerificationWorkflows`，但在同一 CTE 查询上增加 `namespace_id` 过滤，并通过 `TrustStore`、`TrustService`，以及控制面使用的、等价于 worker `combinedSecurityRepairer` 的入口暴露。

- `TrustStore.RepairMissingVerificationWorkflowsForNamespace(ctx, namespaceID, limit, now)`，位于 `securitystore/truststore.go`。
- `TrustService.RepairTrustVerificationForNamespace(ctx, namespaceID, limit)`，带与全局 repair 相同的 `[1, MaxRepairBatch]` 校验。
- 复用 `cfg.Scanner.RepairBatch` 作为单次上限（已确认：无新参数）。

即时触发在 `CreatePolicy` 提交后调用一次。它不会循环到耗尽；剩余由周期性循环排空。

### 4.3 经授权的 `CreatePolicy`

`TrustService.CreatePolicy` 增加授权：

- 新参数或包装器，接收 actor 并解析 namespace 访问权限（`repositories.Service.access` / `allows` 模式，或按 namespace 名称/ID 的等价 `NamespaceAccess` 查询）。
- 默认拒绝：若 actor 不具备 `ManageTrustPolicy`，返回一个被 HTTP 层映射为 `403` 的 forbidden 错误。
- 输入校验保持不变（subject 数量、key/identity 校验）。

### 4.4 HTTP 管理 API（namespace 层级）

在现有 `*httpapi.Router` 上新增路由，于 `app/controlplane/app.go` 注册。资源为 namespace 层级且单数（每个 namespace 一个当前策略），与领域模型一致：

| Method | Path | 行为 | 授权 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/namespaces/{namespace}/trust-policy` | 返回当前策略（最高版本）。无则 `404`。 | `ViewOrganization`（组织 Owner/Admin/Writer/Reader）或个人 namespace owner；非成员得到 `404`。 |
| `POST` | `/api/v1/namespaces/{namespace}/trust-policy` | 创建新版本。只追加。提交后触发 namespace 作用域 repair。返回 `201` 与新版本。 | `ManageTrustPolicy`（namespace `OWNER`）。 |

注意：

- 无 `PUT`/`PATCH`/`DELETE` — 只追加不可变历史。
- POST body 携带公钥（含指纹）和/或 keyless identity，校验上限与领域一致（`MaxTrustPolicySubjects`、`MaxPublicKeyBytes`）。
- GET/POST 响应包含 `id`、`version`、`public_keys`、`keyless_identities`、`created_by_user_id`、`created_at`。
- POST 响应最小化为 `201` + 新版本；不含 eager-repair 结果（见第 9 节）。
- 错误映射复用现有 `mapError` 模式（`ErrForbidden` → `403`、`ErrNotFound` → `404`、`ErrInvalid`/`ErrConflict` → `400`/`409`）。非成员的读取拒绝解析为 `404`（非 `403`），与现有 repository discovery 对未授权调用方隐藏存在性的行为一致。

### 4.5 装配

在 `app/controlplane/app.go`：

- `trustService` 已构造（第 124 行），但仅传给 registry scheduler。将其（连同 `authorization.Policy` 与 namespace 访问查询）传给一个新的 trust-policy handler。
- 在 `securityhandler.RegisterRoutes` 旁注册新路由。

## 5. 前端设计

namespace 层级的 Trust Policy 管理页，范围限定为 M5-01 最小集：

- **查看当前策略**：展示版本、公钥指纹、OIDC issuer + subject 对、创建者、创建时间。无策略时展示空状态。
- **创建新版本表单**：添加公钥（含计算/输入的指纹）和/或精确 keyless identity；通过 `POST` 提交。成功后刷新视图。
- 只读历史列表 **不在** M5-01 范围内（延后）。

需遵循的约定（来自现有 `features/security/artifact-security-panel.tsx` 与 TanStack Query + Zod 校验 API 契约模式）：

- 在 `frontend/features/` 下新增功能目录（如 `features/trust-policy/`）。
- GET 用 TanStack Query，POST 用 mutation，API 契约用 Zod schema。
- 授权由后端强制；UI 在当前用户非 namespace owner 时隐藏/禁用管理控件（以 namespace 视图其他位置使用的同一访问信号为门控）。
- 在任何复用的渲染中保持存在性/有效性/信任的分离。

## 6. 安全与策略评审清单

- [ ] `ManageTrustPolicy` 对个人与组织 namespace 均仅限 `OWNER`；默认拒绝测试覆盖 admin/writer/reader 与匿名调用方。
- [ ] `ViewOrganization` GET 读取权限：组织 Owner/Admin/Writer/Reader 与个人 namespace owner 可读；非成员（无成员关系）得到 `404`，策略绝不泄露。
- [ ] `CreatePolicy` 在任何写入前完成授权。
- [ ] 不引入新的 Pull 阻断行为（D-007 有效）。增加回归测试，确认失败/不受信任的验证不影响 pull token。
- [ ] 结果保持按 digest + policy 版本索引；不增加按 tag 索引的信任状态。
- [ ] 仅公钥材料（无私钥）；指纹校验；大小上限强制。
- [ ] 授权绕过测试：namespace A 的用户不能管理 namespace B 的策略；集成套件中的跨 namespace 隔离测试。
- [ ] 任何测试或验收脚本不上传真实签名到 Sigstore transparency log（保持 M5 事件防护）。

## 7. 有序实现计划

每一步可独立测试，且足够小以便作为一个单元评审。

### 第 1 步 — 授权 capability

- 将 `ManageTrustPolicy` 加入 capability 枚举、`organizationCapabilities` 映射（仅 `RoleOwner`）与 `AllowsPersonalNamespace` 的 owner 分支。
- 文件：`backend/internal/modules/authorization/policy.go`。
- 测试：扩展 policy 表测试，覆盖个人 owner、个人非 owner、组织 owner 与各非 owner 组织角色（除 owner 外全部拒绝）。

### 第 2 步 — Namespace 作用域 repair（store + service）

- 在 `TrustStore` 与 postgres 实现中新增 `RepairMissingVerificationWorkflowsForNamespace`（带 namespace 过滤的 CTE），并在 `TrustService` 新增 `RepairTrustVerificationForNamespace`（带 `[1, MaxRepairBatch]` 校验）。
- 文件：`backend/internal/platform/postgres/securitystore/truststore.go`、`backend/internal/modules/security/trust_service.go`、`TrustStore` 接口。
- 测试：集成测试，创建策略 + artifact，确认作用域 repair 仅对目标 namespace 入队工作流且不影响其他 namespace；幂等测试（运行两次只入队一次）。

### 第 3 步 — 经授权的 `CreatePolicy` + namespace 访问

- 使用 namespace 访问模式为 `CreatePolicy` 增加授权，默认拒绝。
- 文件：`backend/internal/modules/security/trust_service.go`（及其所需的访问查询）。
- 测试：owner 成功；非 owner 与跨 namespace 调用方被拒绝；输入校验不变。

### 第 4 步 — HTTP 管理 API

- 实现 namespace 层级的 GET（当前）与 POST（创建版本）handler，在 `app/controlplane/app.go` 装配路由与依赖，并在提交后增加 POST → 即时 namespace 作用域 repair 触发。
- 文件：新增 `platform/httpapi/trustpolicyhandler/`（或扩展 `securityhandler`）、`app/controlplane/app.go`、`docs/api.md` + `docs/openapi.yaml` + 中文对应文件。
- 测试：handler 单元测试（200/201/400/403/404/409）；针对 PostgreSQL 的"创建后重新验证"全链路集成测试；信任状态不影响 pull token 的回归测试。

### 第 5 步 — 前端管理 UI

- 新增查看当前 + 创建新版本功能，TanStack Query + Zod 契约，按 owner 门控管理控件。
- 文件：新增 `frontend/features/trust-policy/`、namespace 视图中的装配、前端 API client + Zod schema。
- 测试：查看与创建流程的 Vitest 单元测试（成功、校验错误、禁止）；TypeScript、ESLint、生产构建。

### 第 6 步 — 文档与验收

- 更新 `docs/api.md`、`docs/openapi.yaml`、`docs/user-guide.md` 与 release limitations，双语同步；在 `docs/development-plan.md` 记录证据。
- 验收：扩展 M4 e2e 脚本（或新增 M5-01 脚本），通过 API 创建策略，确认 artifact 按新版本重新验证，并确认 Pull 不受影响 — 保持 transparency log 上传禁用。

## 8. 验证

- 每步：聚焦的 Go/前端测试，随后完整 gate。
- 完成 gate：`make check`（沙箱中使用 Go cache 变通 `/private/tmp/hubcr-go-cache`）、相关集成测试，以及一次运行时验收。
- 如实报告已验证内容；说明任何未测试的外部路径（如真实 OIDC provider、真实 transparency log）。

## 9. 已决项

于 2026-08-09 范围讨论会确认，依据现有代码：

- **GET 读取权限复用 `ViewOrganization`。** `GET .../trust-policy` 端点对组织 Owner/Admin/Writer/Reader 及个人 namespace owner 可读。理由：组织成员已能通过按 artifact 的 `GET .../artifacts/{digest}/security` 端点（按 repository discovery 门控，组织 Reader 对私有仓库通过 discovery）间接读到同样的信任主体。仅 owner 的 GET 会造成不一致的不对称。`ViewOrganization` 已定义并授予所有组织角色，但当前从未被强制；本端点成为其首个实际用例，无需 schema 变更。POST（创建新版本）经 `ManageTrustPolicy` 保持仅 owner。非成员（无成员关系）不能读取策略 — 得到 `404`，绝不泄露。
- **POST 响应最小化为 `201 Created` + 新策略版本。** 不含即时 repair 结果（如入队工作流数）。这与每个现有 create 端点（仓库创建、组织创建）一致，它们都返回 `201` + 仅创建的资源。没有任何现有 JSON 响应返回异步工作计数；唯一触发异步工作的端点（registry 事件 webhook）返回 `202` + 空 body，计数只进日志/metrics。前端 create mutation 使用 invalidate-on-success，不消费副作用元数据。即时 repair 进度通过按 artifact 的读取与现有 `stale` 标记观察。
- **JSON 字段命名。** `id`、`version`、`public_keys`（每项含 `fingerprint` 与 key 元数据）、`keyless_identities`（每项含 `issuer` 与 `subject`）、`created_by_user_id`、`created_at`。镜像现有 `TrustPolicy` 领域与只读 security 响应形状。
