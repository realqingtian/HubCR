# HubCR 控制面 API 约定

[English](api.md) | **简体中文**

这些约定适用于 `/api/v1` 下的 JSON 控制面 API。Registry 协议端点 `/token` 与
`/v2/` 保持各自协议契约。

## 传输

- 请求与响应 Body 使用 `application/json`。
- JSON 请求 Body 上限为 1 MiB，拒绝未知字段，并且只能包含一个值。
- 成功响应使用端点自身结构，不套通用 Data Envelope。
- 时间使用带明确 `Z` 时区的 UTC RFC 3339；存在小数秒时予以保留。

## 错误

错误使用稳定 Envelope：

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request validation failed",
    "fields": [
      {"field": "name", "message": "must not be empty"}
    ]
  },
  "request_id": "7f35c89a99194586b19dfba975b5e11b"
}
```

统一错误码为 `invalid_request`、`validation_failed`、`not_found`、
`method_not_allowed`、`authentication_failed`、`rate_limited`、`forbidden`、
`conflict` 和 `internal_error`。
认证失败不会区分用户名不存在、密码错误、Session 缺失、Session 过期或 Session
已撤销。错误消息不得暴露数据库错误、SQL、堆栈、内部路径、凭据、Token 或
Authorization Header。

## 请求关联

客户端可发送 `X-Request-ID`，内容限制为 1–128 个 ASCII 字母、数字、`.`、`_` 或
`-`。Header 缺失或不合法时，API 生成新的 128-bit 十六进制值。每个响应通过
`X-Request-ID` 返回接受或生成的 ID，错误 Envelope 同时在 `request_id` 中包含它。

## 分页

列表端点使用有界 Cursor 分页：

- `limit` 默认为 `20`，范围为 `1` 至 `100`；
- `cursor` 是最长 512 字符的不透明值；
- 重复提供 `limit` 或 `cursor` Query 参数属于无效请求；
- 响应提供 `meta.limit`，没有下一页时省略 `meta.next_cursor`。

## 状态行为

- JSON 格式错误或 Content Type 不支持时返回 `400 invalid_request`；
- 领域校验失败返回 `422 validation_failed`，可附带字段详情；
- 未知路由返回 `404 not_found`；
- 已知路径使用不支持的方法时返回 `405 method_not_allowed` 与 `Allow`；
- 未预期错误和恢复的 Panic 返回 `500 internal_error`，不包含内部原因。

Liveness 只反映进程状态。必需的 PostgreSQL 不可访问时，Readiness 返回 `503` 与
`{"status":"unavailable"}`，依赖恢复后自动恢复。

## Web 认证

- `POST /api/v1/auth/login` 接收本地用户名与密码，返回用户与 Session 过期时间。
  不透明 Session Secret 只通过 `hubcr_session` Cookie 返回，绝不进入 JSON。
- `GET /api/v1/auth/me` 返回已认证用户及显式 `personal_namespace`，否则返回
  `401 authentication_failed`。
- `POST /api/v1/auth/logout` 撤销服务端 Session、清除 Cookie；Cookie 缺失或未知时
  仍保持幂等。
- Cookie 使用 `HttpOnly`、`SameSite=Lax`、Path `/` 与显式过期时间。`Secure` 只在
  本地 HTTP 开发环境默认 `false`；HTTPS 部署必须设置
  `HUBCR_SESSION_COOKIE_SECURE=true`。
- Login 与 Logout 会拒绝标记为 `Sec-Fetch-Site: cross-site` 的浏览器请求。
- Login 在查询凭据前调用显式限流 Adapter。当前自托管基础暂用 Allow-All Adapter，
  直到接入 Redis 限流；公网服务模式仍不可用。

## Registry Token 协议

已批准的 [Registry 认证协议](registry-authentication.zh-CN.md) 定义不属于
`/api/v1` 的 `GET /token` 契约。

- Registry 认证由显式 Feature Gate 控制并默认关闭，直到 M2-04 连接 Distribution
  与本地 Gateway。
- Endpoint 接受精确配置的 `service`、可重复的规范 Repository `scope`、可选
  `client_id` 与可选 HTTP Basic 凭据。
- Basic 凭据验证不会创建 Web Session。Cookie 与 Bearer 凭据会按照协议被忽略或
  拒绝。
- 成功响应包含内容相同的 `token` 与 `access_token` JWT、`expires_in` 和
  `issued_at`；所有响应均不可缓存。
- JWT 只包含分别与策略求交集后的 `pull` 和 `push` Action。`delete` 可被识别，
  但 M2 永不授予。
- 协议错误使用 Distribution `errors` 数组，而不是业务 API Error Envelope。
- 启用路由需要显式外部 Origin、Service/Audience、Issuer、60–900 秒 TTL 和只读
  RS256 私钥绝对路径，以及包含活动密钥的可信公开 JWKS。

## 组织

所有组织端点都要求有效的 `hubcr_session` Cookie。

- `POST /api/v1/organizations` 在同一事务中创建组织、全局唯一 Namespace 和调用者的
  首位 `OWNER` 成员关系。
- `GET /api/v1/organizations` 列出调用者所属的组织。
- `GET /api/v1/organizations/{organization_id}` 及对应 `/members` 端点要求调用者至少
  是组织成员。
- `POST /api/v1/organizations/{organization_id}/members` 增加成员；对
  `/members/{user_id}` 执行 `PATCH` 或 `DELETE` 可修改角色或移除成员。
- `OWNER` 可管理全部角色；`ADMIN` 只能管理 `WRITER` 与 `READER`；`WRITER` 和
  `READER` 不能管理成员；最后一位 `OWNER` 不能被降级或移除。
- 组织与成员列表遵循统一的 `limit` 加不透明 `cursor` 契约。标记为
  `Sec-Fetch-Site: cross-site` 的成员写请求会被拒绝。

## Repository

所有 Repository 端点当前都要求有效的 `hubcr_session` Cookie，并使用规范路径
`/api/v1/namespaces/{namespace}/repositories/{repository}`。

- `POST /api/v1/namespaces/{namespace}/repositories` 创建显式为 `PUBLIC` 或 `PRIVATE`
  的 Repository。个人 Namespace 所有者和组织 `OWNER`、`ADMIN`、`WRITER` 可创建。
- `GET /api/v1/namespaces/{namespace}/repositories` 返回有界分页。Namespace 之外的
  调用者只能看到显式 `PUBLIC` Repository；个人所有者和全部组织角色还可发现
  `PRIVATE` Repository。
- `GET .../{repository}` 在 Repository 不存在或调用者无权发现私有 Repository 时均
  返回 `404`，避免泄露私有资源是否存在。
- `PATCH .../{repository}` 可修改 `description`、`visibility` 或二者。个人所有者与
  组织 `OWNER`/`ADMIN` 可修改可见性；组织 `WRITER` 可编辑说明但不能修改可见性；
  `READER` 不能修改 Repository 元数据。
- 可见性变更会原子更新 `visibility_updated_by_user_id`、
  `visibility_updated_at` 与 `updated_at`；仅修改说明时保持可见性证据不变。
- Namespace 与 Repository 名称都是最长 64 Byte、规范化为小写的 OCI 路径组件；
  Repository 名称在 Namespace 内唯一。Repository 列表使用统一的 `limit` 加不透明
  `cursor` 契约；标记为 `Sec-Fetch-Site: cross-site` 的变更请求会被拒绝。

## OpenAPI 所有权

[`openapi.yaml`](openapi.yaml) 是人工维护并经过审查的 OpenAPI 3.1 契约。公共 API
变更只有在 Handler、目标测试、本文档和 OpenAPI 契约一致时才算完成。以后可以生成
API 文档，但 MVP 阶段不把代码 Annotation 作为事实来源。
