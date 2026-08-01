# Registry 运维可观测性

[English](registry-observability.md) | **简体中文**

- 状态：`IMPLEMENTED`
- 实现日期：2026-08-01
- 工作包：M2-09
- 需求：FR-REG-006 与 FR-OPS-002

本文记录 Registry Challenge、Token 决策、Distribution 通知投递和 Artifact 协调已经
实现的运维信号。该设计保持控制面/数据面边界：Distribution 继续负责 `/v2/` 及其
Bearer Challenge，HubCR 只观测自身 Token 与通知决策，不代理 OCI 字节。

## 信号所有权

| 信号 | 所有者 | 已实现证据 |
| --- | --- | --- |
| `/v2/` Bearer Challenge | Gateway 与 Distribution | Gateway JSON 访问日志；`401` 时包含 `request_id`、`route="registry"`、响应状态和 `registry_challenge=true` |
| Token 请求及 Action 交集 | Go 控制面 | 结构化决策日志和有界 Prometheus Counter |
| 通知投递队列与重试 | Distribution | 仅监听 localhost 的 `/metrics` 与 `/debug/vars` Debug Server |
| 通知接收及协调失败 | Go 控制面 | 结构化请求日志和有界 Prometheus Counter |

Gateway 日志刻意不包含请求 URI、Query String、远端地址、User Agent、Cookie 和
Authorization Header，因此既能记录 Challenge 行为，也不会把 Token 请求参数或凭据
复制到访问日志。Distribution 应用日志使用 JSON 格式。

## 控制面指标

启用 Registry Auth 后，Go API 会在控制面直连 Listener 的 `GET /internal/metrics`
暴露 Prometheus 文本格式。该内部运维 Endpoint 不经过本地公开 Gateway，也不属于
业务 REST 或 OpenAPI 契约。Counter 只存在于当前进程，API 重启后归零。

| 指标 | Label | 含义 |
| --- | --- | --- |
| `hubcr_registry_token_requests_total` | `outcome` | Token 请求分为 `issued`、`invalid`、`unauthorized`、`unavailable` 或 `error` |
| `hubcr_registry_token_actions_total` | `action`、`decision` | Policy 交集后，每个 Scope 的 `pull`、`push`、`delete` 分为 `granted` 或 `denied` |
| `hubcr_registry_notification_requests_total` | `outcome` | 通知 HTTP 请求分为 `accepted`、`unauthorized`、`invalid`、`conflict`、`unavailable` 或 `error` |
| `hubcr_registry_notification_events_total` | `outcome` | 已接收 Envelope 中的事件分为 `processed` 或 `ignored` |
| `hubcr_registry_reconciliation_failures_total` | `class` | Processor 失败分为 `invalid`、`conflict`、`unavailable` 或 `unknown` |

全部 Label 值均来自代码内固定集合。Repository Name、Digest、用户标识、Request ID
和错误字符串都不会成为指标 Label，从而避免无界基数和敏感数据泄漏。

## 控制面结构化日志

成功的 Token 决策包含 `request_id`、`outcome`、`anonymous`、有界 Scope/Action 数量、
签名 `kid` 和 HTTP 状态。失败包含 `request_id`、有界 Outcome/Error Class 及状态。
日志不包含 Basic 用户名或密码、原始 Scope/Repository、Subject 标识、Authorization
Header、Cookie 或签名 Token。

接收的通知包含 `request_id`、`outcome`、Envelope Event 数量、Processed/Ignored 数量
和状态。拒绝的通知包含关联 Request ID、有界 Outcome/Error Class 及状态。日志绝不
包含认证 Token 或 Event Payload。即使 Distribution 未提供 Request ID，API 也会生成
并返回一个，因此每次投递尝试仍可独立追踪。

## Distribution Debug 可见性

本地 Compose Stack 在容器内端口 `5001` 启用 Distribution Debug Listener，并将其
绑定到宿主 Loopback `127.0.0.1:${HUBCR_REGISTRY_DEBUG_PORT:-5002}`。`/metrics` 暴露
Distribution Prometheus 指标（包括通知统计），`/debug/vars` 暴露排查重试积压与失败
所需的 Endpoint 队列状态。

Distribution 官方说明 Debug Endpoint 可能包含敏感运维数据，因此仓库内 Compose
映射仅监听 Loopback。后续生产目标必须把 Go 与 Distribution 运维 Endpoint 放在受保护
内部网络，或增加 Operator 认证；默认都不得通过公开 OCI Gateway 暴露。

## 验收证据

聚焦测试证明指标暴露、有界 Label、Policy 允许/拒绝计数、请求关联、协调失败分类和
日志不泄露 Secret。真实 Docker/OCI 套件在 Distribution 3.1.1 上执行成功与被拒绝的
Push/Pull，并证明相同信号：

```bash
go -C backend test ./internal/platform/observability \
  ./internal/platform/httpapi/registryhandler \
  ./internal/platform/httpapi/registryeventhandler
HUBCR_ENV_FILE=.env.example make infra-config
make test-m2-registry-e2e
make check
git diff --check
```

运行时套件还检查 Gateway Challenge 日志、控制面日志、控制面 Counter、Distribution
Prometheus Endpoint 与通知队列变量均存在，并确认受测密码和 Bearer Token 都未出现在
Gateway 或 API 日志中。
