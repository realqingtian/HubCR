# HubCR Registry 认证协议

[English](registry-authentication.md) | **简体中文**

- 状态：`ACCEPTED`
- 批准日期：2026-08-01
- 工作包：M2-01
- 需求：FR-REG-001、FR-REG-002
- 决策：D-003、D-004、D-005、D-006
- 实现状态：M2-02 至 M2-04 已于 2026-08-01 完成验收

本文档定义 OCI 客户端、HubCR Token 服务、Gateway 与 CNCF Distribution 之间的
认证契约。M2-02 至 M2-04 现已实现该获批契约；事件驱动的 Artifact 协调不在其范围内。

## 1. 所有权与请求流程

HubCR 继续分离控制面与 OCI 数据面：

```text
Docker / OCI client
    |
    | 1. unauthenticated /v2/* request
    v
gateway ------------------------------> CNCF Distribution
    ^                                      |
    | 4. retry /v2/* with Bearer token     | 2. 401 Bearer challenge
    |                                      v
    +---------------- client --------> gateway /token
                                           |
                                           | 3. authenticate, authorize,
                                           |    sign short-lived token
                                           v
                                      Go control plane
```

- Gateway 将 `/v2/*` 路由到 Distribution。Go 控制面不得实现 Manifest、Blob、
  Upload、Download 或 OCI 错误行为。
- Gateway 将 `/token` 路由到 Go 控制面。
- Distribution 生成 Registry Challenge 并验证 Bearer Token。
- HubCR 认证 Registry 调用方、评估仓库策略并签名 Token。
- 浏览器 `/api/v1` Session 与 Registry 凭据相互独立。Token Endpoint 忽略
  Session Cookie，绝不把 Web Session 转换成 Registry 凭据。

外部 Gateway 的实现和生产部署目标不属于 M2-01。M2-04 可以选择本地 Gateway
实现，但不得借此决定 D-009。

## 2. 协议标识与配置

下列值是显式的可信配置，绝不能从不可信的 `Host` 或转发请求头推导：

| 概念 | M2 契约 | 约束 |
| --- | --- | --- |
| 外部 Registry Origin | 运维人员配置的 URL | 除明确记录的本地开发环境外必须使用 HTTPS |
| Token Realm | 外部 Registry Origin 加 `/token` | Distribution Challenge 中暴露的绝对 URL |
| Service | 默认 `hubcr-registry` | 精确、区分大小写匹配 |
| JWT Audience | 与 Service 相同 | Distribution 强制精确匹配 |
| JWT Issuer | 默认 `hubcr-token-service` | 精确、区分大小写匹配 |
| Token TTL | 默认 300 秒 | 可配置范围为 60 至 900 秒 |
| 时钟偏差容差 | 30 秒 | 只应用于 `nbf`，不会延长 `exp` |

部署可以覆盖 Service 与 Issuer 以避免冲突，但 Token 服务与 Distribution 配置必须
使用完全相同的值。Realm 不是绝对 URL、Service 或 Issuer 为空、TTL 超出范围，或
签名配置无效时，启动必须失败。

Distribution 的 Token Auth 配置使用相同的 `realm`、`service` 和 `issuer`，
以及可信公钥 Bundle。`autoredirect` 保持关闭，防止代理请求头静默改变 Realm。

## 3. Distribution Challenge 契约

Distribution 拥有全部 `/v2/*` 响应。当请求需要授权时，它返回
`401 Unauthorized` 以及等价于以下内容的 Challenge：

```http
WWW-Authenticate: Bearer realm="https://hubcr.example/token",service="hubcr-registry",scope="repository:team/image:pull,push"
```

规则：

1. `realm` 是配置的公开 Token URL。
2. `service` 是配置的 Service，因此也是 JWT 必须使用的 Audience。
3. 初始 `/v2/` 能力检查等不需要仓库操作的请求会省略 `scope`。
4. Distribution 可以为一次 OCI 操作包含一个或多个 `scope` 值。
5. HubCR 不重写或代理 Challenge Body。Gateway 保留 `WWW-Authenticate` 与
   `Docker-Distribution-Api-Version`。

## 4. Token 请求契约

M2 支持：

```http
GET /token?service=hubcr-registry&scope=repository:team/image:pull,push&client_id=docker
```

| 输入 | 行为 |
| --- | --- |
| `service` | 必须且只能出现一次，并与配置的 Service 相同 |
| `scope` | 可选且可重复；分别解析每个值 |
| `client_id` | 可选；验证语法后只作为不含秘密的审计上下文 |
| `offline_token` | 可选的布尔兼容提示；允许传入，但 M2 永不签发 Refresh Token |
| HTTP Basic 凭据 | 可选；存在时通过 Registry 凭据边界认证 |
| Cookie | 忽略 |

M2 只要求 `GET`。不支持的方法返回 `405 Method Not Allowed` 和
`Allow: GET`。

### 4.1 调用方认证

- 没有 `Authorization` 请求头表示匿名调用方。
- `Authorization: Basic ...` 通过 Registry 专用应用边界使用 D-003 的同一本地
  用户名/密码身份存储，但不会创建 Web Session。
- 有效凭据为 `sub` 生成稳定且不透明的用户 ID；Token 中不嵌入用户名或电子邮件
  地址。
- 已提供但格式错误、不支持或无效的凭据返回 `401 Unauthorized`，绝不能降级为
  匿名访问。
- 响应可以包含 `WWW-Authenticate: Basic realm="HubCR Registry"`，让客户端识别
  凭据交换失败。
- M2 不接受 Bearer 凭据或 Web Session Cookie 作为签发新 Registry Token 的凭据。

## 5. 仓库 Scope

### 5.1 语法

HubCR 接受 Distribution Resource Scope 结构：

```text
repository:{namespace}/{repository}:{action[,action...]}
```

例如：

```text
repository:team/image:pull,push
```

HubCR 的 MVP 约束比通用 Distribution 语法更严格：

- Resource Type 必须恰好为 `repository`。不生成也不接受
  `repository(plugin)` 等已弃用的 Resource Class。
- Resource Name 必须恰好包含 Namespace 与 Repository 两个路径组件。
- 每个组件必须是小写 ASCII、最长 64 字节，并匹配
  `[a-z0-9]+(?:[._-][a-z0-9]+)*`。
- 拒绝带 Host 前缀的 Resource Name、端口、空组件、多余路径组件、Unicode、
  空白字符和路径穿越语法。
- Parser 定位第一个和最后一个 `:` 分隔符，不进行任意数量字段的朴素分割。
- Action 区分大小写。识别的 Action 为 `pull`、`push` 和 `delete`。
- 对重复 Action 和完全相同的 Scope 去重。Claim 使用固定 Action 顺序
  `pull`、`push`、`delete`。
- 输入只做验证，绝不归一化为另一个仓库身份。

### 5.2 多 Scope

Token Endpoint 接受重复的 `scope` 参数，因为符合标准的客户端可能请求多个资源。
每个精确仓库分别解析并独立授权。Access Entry 按仓库名称排序，使等价请求生成确定性
Claim。

仓库 A 的授权绝不能为仓库 B 提供 Action。只包含 A 的 Token 仍不能用于 B。只有在
客户端明确同时请求 A 与 B，且策略分别授予两者操作时，一个 Token 才可以同时包含
A 与 B。实现必须在解析前限制请求大小和 Scope 数量；具体传输限制属于 M2-03。

缺少 `scope` 是有效请求，会生成空 `access` 数组，使客户端可以完成基础
`/v2/` 能力检查。空 Scope 值或格式错误的 Scope 是协议错误。

## 6. 授权与 Action 交集

对每个有效的仓库 Scope：

```text
token actions = requested actions ∩ policy-allowed actions
```

策略来源是中央 Authorization 模块，而不是 HTTP Handler 或 Distribution 配置。

| 仓库与调用方 | 策略允许的 Registry Action |
| --- | --- |
| 明确为 `PUBLIC`、匿名 | `pull` |
| 明确为 `PUBLIC`、已认证 | `pull` 加下列已认证能力 |
| 个人 Namespace Owner | `pull`、`push` |
| 个人 Namespace 非 Owner | 没有私有访问；公开规则仍适用 |
| 组织 `OWNER`、`ADMIN` 或 `WRITER` | `pull`、`push` |
| 组织 `READER` | `pull` |
| 缺少 Membership 或属于其他组织 | 没有私有操作 |
| 缺少 Repository、Visibility、Namespace Owner、Role 或 Policy 数据 | 没有操作 |

协议明确识别 `delete`，但 M2 永不授予它。仓库删除、保留和垃圾回收策略仍然延后，
M2-04 在连接 Token 认证前必须关闭 Distribution 删除功能。

对于调用方有效但没有允许操作的格式正确请求，Token Endpoint 不返回授权错误。
HubCR 返回 `200 OK`，其中包含精确仓库 Entry 以及空 `actions` 数组。随后由
Distribution 拒绝原始 OCI 请求。这既保留标准的 Action 交集流程，也避免使用 Token
Endpoint 状态差异泄露私有仓库是否存在。

## 7. JWT 契约

返回的 Bearer Token 是签名 JWT，客户端应将其视为不透明值。

### 7.1 JOSE Header

| 字段 | 值 |
| --- | --- |
| `typ` | `JWT` |
| `alg` | 首个实现使用 `RS256` |
| `kid` | 活动公钥与 Distribution 兼容的 JWK Thumbprint |

禁止对称 HMAC 算法，因为它会让数据面持有签名权限。M2-02 只有在提供 Signer、
Distribution、轮换和负向测试证据后，才能增加其他非对称算法。

### 7.2 Claim

| Claim | 契约 |
| --- | --- |
| `iss` | 配置的 Issuer |
| `sub` | 稳定且不透明的用户 ID；匿名时为 `""` |
| `aud` | 配置的 Service，使用单个 JSON String |
| `exp` | `iat + TTL` |
| `nbf` | `iat - 30 seconds` |
| `iat` | UTC 签发时刻 |
| `jti` | 密码学随机值，至少 128 位 |
| `access` | 包含精确 `type`、`name` 与交集后 `actions` 的 Entry 数组 |

Token 不包含密码、Session ID、电子邮件地址、组织 Membership、仓库 Visibility 或
完整策略快照。

### 7.3 签名密钥生命周期

- M2-02 使用非对称密钥。Token 服务接收私有签名材料；Distribution 只接收公开验证
  材料。
- 生产私钥来自只读 Secret Mount 文件，绝不存入数据库或源代码树，也不得作为纯文本
  环境变量提供。
- 配置指定一个活动签名密钥。每个签发的 Token 都使用其 `kid`。
- Distribution Trust Bundle 可以包含活动公钥和正在退出的公钥。签发永不使用正在
  退出的密钥。
- 轮换按阶段执行：向 Distribution 添加新公钥并 Reload/Restart；把 Token 服务切换
  到匹配私钥；保留旧公钥至少最大 TTL 加时钟偏差；最后在后续 Reload/Restart 中移除。
- 密钥不可读、公私钥不匹配、`kid` 重复、算法不支持，或活动密钥不在配置的信任集合
  中时，启动必须失败。
- 仓库 Fixture 可以包含明确标记为不安全且只限本地使用的测试密钥。生产启动必须拒绝
  这些 Fixture。

## 8. Token 响应与生命周期

成功响应：

```json
{
  "token": "<signed-jwt>",
  "access_token": "<same-signed-jwt>",
  "expires_in": 300,
  "issued_at": "2026-08-01T00:00:00Z"
}
```

- 同时返回 `token` 与 `access_token`，且两者逐字节相同。
- `expires_in` 等于配置的 TTL，并且绝不小于 60。
- `issued_at` 使用 RFC 3339 UTC，并与 `iat` 保持整秒精度一致。
- 每个成功与错误响应都使用 `Content-Type: application/json`、
  `Cache-Control: no-store` 和 `Pragma: no-cache`。
- M2 不为 `/token` 开启跨 Origin 浏览器访问；OCI 客户端不需要 CORS。
- 默认 TTL 为五分钟。最大值为十五分钟，在允许正常镜像传输开始的同时限制重放暴露。
- M2 不签发 Refresh Token。Docker CLI 在登录时会发送 `offline_token=true`，因此
  HubCR 接受该提示，但只返回短期 Access Token。客户端在过期后请求另一个短期 Token。
- Token 不是服务端 Session，无法逐个撤销。密码或 Membership 变更影响后续签发；
  已签发 Token 在过期前仍有效，除非紧急移除签名密钥。

## 9. 错误契约

`/token` 是 Registry 协议 Endpoint，而不是 `/api/v1` 业务 API。它使用
Distribution JSON 错误结构：

```json
{
  "errors": [
    {
      "code": "UNAUTHORIZED",
      "message": "registry credentials are invalid"
    }
  ]
}
```

| 条件 | 状态 | Code |
| --- | --- | --- |
| 缺少、重复或不匹配的 `service` | `400` | `DENIED` |
| Scope 格式错误、Type/Action 不支持、`client_id` 无效 | `400` | `DENIED` |
| `offline_token` 重复或不是布尔值 | `400` | `DENIED` |
| 凭据格式错误、不支持或无效 | `401` | `UNAUTHORIZED` |
| 方法不支持 | `405` | `UNSUPPORTED` |
| Policy Lookup 或签名依赖不可用 | `503` | `UNAVAILABLE` |
| 意外内部错误 | `500` | `UNKNOWN` |

错误消息稳定且通用，不包含仓库是否存在、Visibility、Membership、密码细节、密钥路径、
SQL 错误、Token 内容或 Stack Trace。依赖失败绝不能变成空策略的成功结果；必须
Fail Closed 并返回 `503`。

## 10. 日志与安全控制

M2-03 和 M2-09 必须为 Token 请求与失败提供结构化且不含秘密的事件。允许的字段包括
Request ID、配置的 Service、已认证或匿名状态、不透明 Actor ID、精确 Canonical
Repository、请求 Action、授予 Action、决策原因分类、签名 `kid`、Status 和
Duration。

绝不记录：

- `Authorization`、`Cookie`、原始 Basic 凭据、密码、JWT、私钥，或包含凭据的
  完整请求 URL；
- 验证前的任意 Parser 输入；
- 包含敏感值的数据库错误。

M2-01 不定义 Rate Limit 产品策略，但 M2-03 必须限制 Request Body/Query 大小与
Scope 数量。部署级 Rate Limit 可以在不改变授权事实的情况下加入。

## 11. 实现后必须提供的验收证据

M2-02 至 M2-04 不能只凭 Unit Test 验收。实现必须覆盖：

| 用例 | 必需证据 |
| --- | --- |
| 基础 `/v2/` | Distribution Challenge 后使用有效的空 Access Token |
| 匿名公开 Pull | 精确仓库 `pull` 成功 |
| 匿名私有 Pull | 空 Action；Distribution 拒绝且不泄露存在性 |
| 已认证角色矩阵 | `OWNER`/`ADMIN`/`WRITER` 可 Push；`READER` 只能 Pull |
| 个人 Namespace | Owner 可 Push/Pull；非 Owner 除公开 Pull 外均拒绝 |
| Action 交集 | 请求 `pull,push`，只授予策略允许的子集 |
| 跨仓库隔离 | 仓库 A 的 Token 不能访问 B |
| 多个显式 Scope | 分别对每个仓库求交集 |
| 无效 Token | 过期、未来 `nbf`、Issuer/Audience/Signature/`kid` 错误均拒绝 |
| 密钥轮换 | 重叠期活动与退出密钥都可验证；移除后旧密钥失败 |
| 秘密安全 | 日志和错误不包含凭据或 Token 材料 |
| 真实客户端 | Docker 或受支持 OCI 客户端通过本地 Gateway Push/Pull |

## 12. 规范性参考

- [CNCF Distribution Token Authentication Specification](https://distribution.github.io/distribution/spec/auth/token/)
- [CNCF Distribution Token Scope Documentation](https://distribution.github.io/distribution/spec/auth/scope/)
- [CNCF Distribution JWT Authentication Implementation](https://distribution.github.io/distribution/spec/auth/jwt/)
- [CNCF Distribution Registry Configuration](https://distribution.github.io/distribution/about/configuration/#token)
- [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md)

## 13. 评审门

批准本文档已关闭 M2-01 并解除 M2-02 的阻塞。获批契约确认：

1. 固定 Service/Audience 与 Issuer 契约；
2. 默认 300 秒、范围 60–900 秒的 TTL；
3. RS256 与公钥重叠期的分阶段轮换模型；
4. 对重复 Repository Scope 分别独立授权；
5. 对格式正确但未授权 Scope 返回空 Action 的 `200 OK`；
6. Basic Registry 认证继续独立于 Web Session；
7. 同 Origin Gateway 路径契约，但不选择生产部署目标。
