# Distribution 事件协调

[English](distribution-event-reconciliation.md) | **简体中文**

- 状态：`IMPLEMENTED`
- 实现日期：2026-08-01
- 工作包：M2-06
- 需求：FR-ART-001、FR-ART-004
- 依赖：[M2-05 Artifact 元数据持久化](artifact-metadata-persistence.zh-CN.md)

本文档记录 CNCF Distribution 与 HubCR 控制面之间已经实现的契约。Distribution
继续负责 OCI 数据面；Go API 只消费经过认证的 Push 通知，用于协调 Repository 级
Artifact 与当前 Tag 元数据。

实现遵循官方 [Distribution 通知
契约](https://distribution.github.io/distribution/about/notifications/)和[通知 Endpoint
配置](https://distribution.github.io/distribution/about/configuration/)。已验收的本地运行
版本为 Distribution 3.1.1。

## 范围与边界

- Distribution 使用
  `application/vnd.docker.distribution.events.v2+json` Content Type，把 `push` 事件发送到
  `POST /internal/registry/events`。
- Endpoint 处理 OCI 与 Docker v2 Manifest、Index/Manifest List Media Type。Blob 事件与
  不支持的 Media Type 会被接受但忽略。
- 本地 Distribution 配置过滤 `pull`、`delete` 和 `mount` Action。在保留、删除和垃圾
  回收策略获批前，删除能力继续禁用。
- 已启用 `events.includereferences`，使 Index 事件可以携带有序 Child Manifest
  Descriptor 与 Platform 元数据。
- 缺失或空的 `references` 字段继续表示未知 Descriptor Set，因为 Distribution Payload
  无法区分被省略的空集合与明确确认的空集合。
- 本工作包不提供 Artifact/Tag HTTP API，不保存 Tag 历史，不创建删除 Tombstone，也不
  改变 OCI 上传或下载行为。

## 认证与请求限制

Distribution 与 API 共享 `HUBCR_REGISTRY_EVENT_TOKEN`。启用 Registry 认证时必须提供
该值；它必须包含 32–512 个可见 ASCII 字符，并通过且仅通过一个
`Authorization: Bearer <token>` Header 发送。Handler 对已配置 Secret 的 SHA-256
Digest 进行常量时间比较，且永不记录 Secret、Authorization Header 或通知 Payload。

仓库中提供的 Token 明确只用于本地开发。部署时必须为 API 与 Distribution Endpoint
配置同一个独立 Secret。

Handler 每次最多接收 1 MiB 和 100 个事件。错误 JSON、尾随 JSON 值、缺失或重复认证
Header、不支持的方法和错误 Media Type 都会被拒绝。为了协议向前兼容，未知 JSON
字段会被接受。

## 协调与重试行为

每个相关事件都会解析精确的 `namespace/repository`，校验 Digest、Tag、Media Type、
Size、Timestamp 与 Descriptor，然后调用 M2-05 原子 GORM 协调服务。即使 Distribution
在物理层按 Digest 去重，Repository 身份仍属于 Artifact 身份的一部分。

Distribution 通知至少投递一次，且不保证顺序。因此完全重放是幂等的。较新的 Tag
Observation 可以移动当前 Tag；更旧或时间相同的事件不能把它从更新的持久化映射上移
开。原先的无 Tag Artifact 继续保留。

响应码会明确驱动 Distribution 的重试行为：

| 结果 | 状态码 | 重试含义 |
| --- | --- | --- |
| 已接受或有意忽略 | `202` | 投递完成 |
| 无效请求或事件 | `400` | 永久 Payload 错误 |
| 不可变元数据矛盾 | `409` | 永久协调冲突 |
| Repository 查询或持久化不可用 | `503` | 可重试的依赖故障 |
| Event Token 缺失或无效 | `401` | 配置或认证故障 |
| Media Type 错误或 Body 过大 | `415` / `413` | 永久请求错误 |

Distribution 使用有界本地 Endpoint Queue、两秒 Timeout、五次失败阈值和一秒 Backoff。
运维指标与更完整的事件可观测性属于 M2-09。

## 验收证据

聚焦测试覆盖事件映射、认证、请求限制、错误分类、重复投递、Tag 移动、陈旧事件保护、
Index Reference、Repository 隔离及依赖故障。真实 Docker/OCI 套件通过受 Token 保护的
Distribution Push 公开与私有镜像，并验证 PostgreSQL 中形成的 Artifact/Tag 状态，
包括 Repository 级身份以及被拒绝 Push 不产生持久化记录。

```bash
go -C backend test ./internal/modules/registry ./internal/platform/httpapi/registryeventhandler
make test-integration
make test-m2-registry-e2e
HUBCR_ENV_FILE=.env.example make infra-config
make check
git diff --check
```
