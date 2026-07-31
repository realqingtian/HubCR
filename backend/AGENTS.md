# HubCR backend instructions

**English** | [简体中文](AGENTS.zh-CN.md)

These instructions apply only to files under `backend/`. They supplement the root
`AGENTS.md`. Do not use this file to impose Go or backend implementation rules on
`frontend/` or other sibling directories.

## Backend scope

The backend is one Go module, `hubcr.io/hubcr`, with two executable processes:

- `cmd/api`: the HubCR business control plane;
- `cmd/worker`: asynchronous jobs such as scanning and signature verification.

Before backend work, read:

- `backend/go.mod` for the active Go version;
- `docs/architecture.md` and the backend sections of `docs/development.md`;
- the relevant module, adapter, migration, and tests already present.

For a backend-only task, do not modify `frontend/` unless the user explicitly requests
a coordinated frontend change. If an API contract changes, report the frontend impact
without editing the frontend by default.

## Package ownership

- `cmd/api` and `cmd/worker` own process startup, signal handling, and exit status only.
- `internal/app/controlplane` composes API modules and infrastructure adapters.
- `internal/app/worker` composes job handlers and worker infrastructure.
- `internal/modules/<capability>` owns business behavior, domain types, application
  services, and the ports that capability consumes.
- `internal/platform` owns configuration and concrete infrastructure adapters.
- `migrations` owns PostgreSQL schema evolution.

Business modules must not import `cmd`, HTTP handlers, process code, or concrete
database/cache clients. Platform packages implement module-owned interfaces and must
not make product-policy decisions. Cross-module behavior goes through explicit
application services; do not reach into another module's internal storage.

Avoid package cycles, mutable package globals, hidden initialization, and broad
`common`, `helpers`, or `utils` packages. Place code with the capability that owns it.

## Go implementation rules

- Use the Go version declared in `go.mod` and format all code with `gofmt`.
- Prefer the standard library. Explain and justify every new production dependency.
- Pass `context.Context` through request, persistence, network, and worker boundaries.
- Wrap errors with operational context using `%w`; preserve errors needed for
  classification and do not compare error strings.
- Use `errors.Is` or `errors.As` for stable error handling.
- Keep `main` functions, HTTP handlers, and storage adapters thin.
- Define interfaces in the consuming module, not in the implementing platform package.
- Prefer explicit constructors and dependency injection over global registries.
- Validate configuration at process startup and return actionable errors.
- Use structured `log/slog` fields. Never log credentials, tokens, authorization
  headers, raw cookies, signing keys, or sensitive user data.
- Servers and workers must honor cancellation and shut down gracefully.
- Do not launch unbounded goroutines. Every background task needs ownership,
  cancellation, error handling, and a bounded concurrency policy.

## HTTP and API rules

- Business REST endpoints live under `/api/v1`.
- Registry authentication retains the protocol-specific `/token` path.
- Distribution owns `/v2/`; do not proxy its business logic into control-plane
  handlers.
- Treat path, query, header, and body data as untrusted and validate before calling
  application services.
- Keep HTTP DTOs separate from domain entities and persistence records.
- Use the correct status code and a stable error representation; never return SQL,
  stack traces, internal paths, or secret values.
- Make retryable commands idempotent where practical.
- Health liveness must prove the process is running. Readiness may include dependency
  checks once those dependencies are connected; do not make liveness depend on slow
  external services.

When a public API contract changes, update its tests and both language versions of
the affected documentation. Do not edit frontend implementation unless the task is
explicitly cross-stack.

## Persistence and jobs

- PostgreSQL is the source of truth for HubCR business records.
- Use transactions for state changes that must be atomic.
- Store timestamps in UTC and serialize them with an explicit timezone.
- Enforce uniqueness, foreign keys, and other durable invariants in PostgreSQL as well
  as application code.
- Add indexes for demonstrated query patterns, not speculation.
- Keep transport DTOs, domain models, and database records separate.
- Once the first public release exists, migrations are append-only and immutable.
- Initial asynchronous work uses a PostgreSQL job table. Claim jobs atomically, make
  handlers safe to retry, record failures, and prevent concurrent duplicate work.
- A process crash must not silently lose a persisted job.

Do not select an ORM, migration framework, or external message broker without an
explicit project decision.

## Registry and security rules

- Authorization is evaluated against namespace, repository, requested action, and
  repository visibility before a scoped token is issued.
- Token scopes must not use user-supplied repository names before canonical validation.
- Physical digest deduplication never grants access across repository boundaries.
- Tags are mutable; artifacts, scans, SBOMs, and verification records use immutable
  digests as their identity.
- Signature discovery, cryptographic verification, and trust-policy evaluation are
  separate stored outcomes.
- Record the signature digest, signer identity, policy version, and verification time
  when verification is implemented.
- Registry events and worker tasks are untrusted inputs and must be authenticated or
  validated, idempotent, and safe to replay.
- Scan and signature jobs must not block successful pushes unless an accepted policy
  explicitly says otherwise.

## Backend tests

- Colocate unit tests as `*_test.go` and prefer table-driven tests when they improve
  clarity.
- Test behavior and invariants rather than private implementation details.
- Unit tests must be deterministic and must not require PostgreSQL, Redis, MinIO,
  Distribution, network access, or wall-clock sleeps.
- Mark integration tests clearly and give them an explicit setup and cleanup path.
- Add regression tests with every bug fix.
- Use `httptest` for HTTP behavior and injectable clocks or sources for time-dependent
  logic.

## Backend validation

During backend development, run from `backend/`:

```bash
gofmt -w .
go vet ./...
go test ./...
```

Before declaring the overall task complete, run `make check` from the repository root.
Run an API or worker smoke test when startup or runtime behavior changes. If a required
service is unavailable, report that limitation explicitly.

## Backend code review rules

- Flag domain behavior in `cmd`, HTTP handlers, or `internal/platform`.
- Flag module-to-module storage access that bypasses an application service.
- Flag contexts that are dropped across I/O or worker boundaries.
- Flag authorization that defaults to allow or accepts an unvalidated repository path.
- Flag jobs that are not idempotent, replay-safe, cancellable, or bounded.
- Flag artifact security data keyed only by tag.
- Flag secrets or sensitive values in logs and error responses.
