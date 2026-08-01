# Database migrations

**English** | [简体中文](README.zh-CN.md)

HubCR uses GORM with its PostgreSQL driver for persistence and Gormigrate for
versioned, forward-only schema migrations. GORM keeps normal persistence and schema
definitions in typed Go code. Gormigrate adds explicit migration IDs and validation
that plain `AutoMigrate` does not provide, while CI exercises the same code as the
`db-migrate` command.

## Conventions

- Migrations are Go entries returned by `all()` and use ordered
  `NNNNNN_lowercase_name` IDs.
- Each migration uses migration-local GORM records that are separate from runtime
  adapter records, so later domain-model changes cannot rewrite historical schema
  intent.
- All pending migrations run in one GORM transaction under a PostgreSQL advisory
  lock.
- `hubcr_schema_migrations` records applied migration IDs and rejects unknown database
  migration IDs.
- Reapplying the same migration set is safe.
- Migrations are forward-only. Correct a published migration with a new migration;
  use tested backup/restore procedures for operational rollback.
- Product-policy models must not be added until their requirements and decision gates
  are approved. `000001_foundation` establishes versioning; after G-01 closed,
  `000002_identity_persistence` added `users`, `local_credentials`, `web_sessions`, and
  `user_invitations` with durable foreign keys, uniqueness, expiry, token-digest, and
  terminal-state constraints. `000003_personal_namespaces` adds globally unique,
  normalized personal namespaces, enforces one namespace per user, and transactionally
  backfills compatible pre-existing users. `000004_organizations` extends namespace
  ownership for organizations and adds organizations plus four-role memberships.
  `000005_repositories` adds namespace-owned repository identities, explicit
  `PUBLIC`/`PRIVATE` visibility without a database default, creator and initial
  visibility-change evidence, and namespace/name uniqueness.

Run migrations against the configured database from the repository root:

```bash
make db-migrate
```

`HUBCR_DATABASE_URL`, `HUBCR_DATABASE_CONNECT_TIMEOUT`, and
`HUBCR_DATABASE_MAX_CONNECTIONS` use the same configuration as the API. Errors and
logs must never expose the credential-bearing URL.

## Integration test isolation

Run the migration, PostgreSQL connectivity, and auth-store lifecycle checks with:

```bash
make test-integration
```

The harness creates the fixed Compose project `hubcr-integration` with a dedicated
`hubcr_test` database on host port `55432` by default. Its data directory is a
container `tmpfs`. Cleanup targets that exact test project and removes only its
ephemeral volumes. It never issues cleanup SQL against a caller-provided database.
Override the host port with `HUBCR_TEST_POSTGRES_PORT` when necessary.
