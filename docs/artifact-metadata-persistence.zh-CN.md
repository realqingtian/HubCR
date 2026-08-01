# Artifact 元数据持久化

[English](artifact-metadata-persistence.md) | **简体中文**

- 状态：`APPROVED`
- 批准日期：2026-08-01
- 工作包：M2-05
- 需求：FR-ART-001 至 FR-ART-005
- 实现状态：M2-05 已于 2026-08-01 完成验收

本文档定义 M2-06 事件协调与 M2-07 Artifact API 已经消费的持久化契约。这两项能力
都通过独立 Adapter 使用同一个共享领域边界。

## 决策与边界

- 即使 Distribution 在物理层按 Digest 去重，Artifact 身份仍为
  `(repository_id, digest)`。
- M2 只接受规范 SHA-256 Digest：
  `sha256:<64 个小写十六进制字符>`。
- Tag 只保存当前 Artifact 引用；M2-05 不保存 Tag 移动或删除历史。
- 移动或删除 Tag 永不删除原 Artifact。无 Tag Artifact 会继续保留，直到另行批准保留
  与垃圾回收策略。
- Index 保存有序且不可变的 Child Manifest Descriptor 集合；缺失 Platform 元数据
  继续保持缺失。
- M2-05 负责领域验证、GORM/Gormigrate Schema、原子持久化和 Repository 级读取。
  Distribution Push 事件接入已由 M2-06 实现，经过授权的 HTTP 读取已由 M2-07 实现。

## 模块所有权

`backend/internal/modules/artifacts` 负责值验证、领域实体、稳定错误、协调编排和 Store
接口。

`backend/internal/platform/postgres/artifactstore` 负责 GORM Record、PostgreSQL
事务与锁、持久化读取以及数据库错误分类。它不作授权，也不解释 Distribution Event。

`backend/migrations` 负责仅向前迁移 `000006_artifact_metadata`。

## Schema

### `artifacts`

每行包含不透明 UUID、Repository ID、Digest、不可变的 `MANIFEST` 或 `INDEX` Kind、
可空 Media Type/Size/来源创建时间、Descriptor 完成状态，以及 UTC 发现/更新时间。

持久约束包括：

- `(repository_id, digest)` 唯一；
- 规范 SHA-256 Digest Check；
- Size 非负；
- `updated_at >= discovered_at`；
- 只有 Index 可以拥有已完成的 Descriptor Set；
- Repository 与 Artifact 删除使用 `RESTRICT`，不进行隐式元数据清理。

### `tags`

复合身份为 `(repository_id, name)`。Name 区分大小写，最长 128 字节，并匹配
`[A-Za-z0-9_][A-Za-z0-9._-]{0,127}`。复合外键确保当前 Artifact 属于同一个
Repository。

### `manifest_descriptors`

每行保存 Repository ID、Index Artifact ID、从零开始的 Position、Child Manifest
Artifact ID，以及可空的 OS/Architecture/Variant。复合外键保证 Parent 与 Child
属于同一个 Repository。Check 会拒绝自引用与不完整 Platform。

`descriptors_complete` 用于区分未知 Descriptor Set 和已确认的空集合。完成后，同一
Digest 的有序 Descriptor Set 不可变。

## 原子协调

`ReconcileArtifact` 在一个事务中执行：

1. 插入或加载并锁定 Repository 级 Parent Artifact。
2. 要求不可变 Kind 一致。
3. 补全可空元数据；同一 Digest 出现不同的非空事实时返回冲突。
4. 提供完整 Index Descriptor Set 时协调 Child Manifest Row。
5. 首次插入完整 Descriptor Set，或与已有集合逐项严格比较。
6. 可选地在同一事务中创建或移动当前 Tag。
7. 处理并发冲突后返回实际持久化的获胜状态。

完全重放不会创建 Row 或修改时间。任何矛盾都返回 `ErrConflict` 并回滚全部改动。
`RemoveTag` 幂等且永不删除 Artifact 或 Descriptor Row。

## 读取与错误

所有读取都要求 Repository ID，并支持按 Digest 读取 Artifact、按 Name 读取 Tag、按
Digest 排序的 Artifact 分页、按 Name 排序的 Tag 分页，以及读取有序 Index
Descriptor。Limit 为 1 至 100，Cursor 必须是合法领域值。

稳定错误为 `ErrInvalidDigest`、`ErrInvalidTag`、`ErrInvalidArtifact`、
`ErrConflict`、`ErrNotFound` 与 `ErrUnavailable`。SQL 和 Schema 细节不能越过 Store
边界。

## 验收证据

M2-05 测试必须覆盖验证、空库/升级/重复迁移、数据库约束、完全重放、元数据补全与
冲突回滚、Tag 移动与删除、无 Tag Artifact 保留、Descriptor 不可变、跨 Repository
外键、有界分页，以及真实隔离 PostgreSQL 下的并发幂等性。

完成要求：

```bash
go -C backend test ./internal/modules/artifacts
go -C backend test ./internal/platform/postgres/artifactstore
make test-integration
make check
git diff --check
```

M2-05 工作包本身明确不包含 Distribution Notification Handler、HTTP API、Tag 审计
历史、Artifact 删除/保留/GC、安全任务或结果，以及前端视图。M2-06 现已提供通知处理，
M2-07 现已提供只读 HTTP API，但没有改变其余边界。
