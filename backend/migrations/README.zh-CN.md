# 数据库迁移

[English](README.md) | **简体中文**

HubCR 使用 GORM 及其 PostgreSQL Driver 负责持久化，并用 Gormigrate 执行带版本的
仅向前 Schema Migration。GORM 让常规持久化与 Schema 定义保留在类型化 Go 代码中；
Gormigrate 提供普通 `AutoMigrate` 不具备的显式迁移 ID 与校验，同时 CI 与
`db-migrate` 命令执行相同代码。

## 约定

- Migration 是 `all()` 返回的 Go 条目，使用有序的 `NNNNNN_lowercase_name` ID。
- 每条 Migration 使用与运行时 Adapter Record 分离的迁移专用 GORM Record，避免未来
  领域模型变化改写历史 Schema 意图。
- 所有待执行 Migration 在 PostgreSQL Advisory Lock 下通过同一个 GORM 事务执行。
- `hubcr_schema_migrations` 记录已应用 Migration ID，并拒绝数据库中未知的 ID。
- 重复应用同一组 Migration 是安全的。
- 迁移仅向前执行。已发布迁移的问题通过新迁移修正；运维回退使用经过验证的备份与
  恢复流程。
- 产品策略相关模型必须等待其需求和决策门获批。`000001_foundation` 建立版本基础；
  G-01 关闭后，`000002_identity_persistence` 增加 `users`、`local_credentials`、
  `web_sessions` 与 `user_invitations`，并持久化约束外键、唯一性、过期、Token Digest
  和终止状态。`000003_personal_namespaces` 增加全局唯一且规范化的个人 Namespace，
  强制每个用户只能拥有一个，并在事务中回填兼容的已有用户。
  `000004_organizations` 将 Namespace 归属扩展到组织，并增加组织及四角色成员关系。
  `000005_repositories` 增加归属于 Namespace 的 Repository 身份、没有数据库默认值的
  显式 `PUBLIC`/`PRIVATE` 可见性、创建者与初始可见性变更证据，以及 Namespace/名称
  唯一性。`000006_artifact_metadata` 增加 Repository 级不可变 Artifact Digest、当前
  Tag 引用、有序 Manifest Descriptor，以及防止跨 Repository 引用的复合外键。

在仓库根目录针对配置的数据库执行迁移：

```bash
make db-migrate
```

`HUBCR_DATABASE_URL`、`HUBCR_DATABASE_CONNECT_TIMEOUT` 与
`HUBCR_DATABASE_MAX_CONNECTIONS` 使用和 API 相同的配置。错误与日志不得暴露带凭据
的 URL。

## 集成测试隔离

执行迁移、PostgreSQL 连接、持久化生命周期、约束与并发检查：

```bash
make test-integration
```

Harness 创建固定 Compose 项目 `hubcr-integration`，使用独立的 `hubcr_test` 数据库，
默认宿主端口为 `55432`，数据目录使用容器 `tmpfs`。清理只针对这个准确的测试项目并
仅删除其临时卷；不会对调用者提供的数据库执行清理 SQL。必要时可通过
`HUBCR_TEST_POSTGRES_PORT` 覆盖宿主端口。
