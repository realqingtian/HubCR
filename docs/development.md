# HubCR development standards

**English** | [简体中文](development.zh-CN.md)

This document defines the engineering rules for HubCR contributors. The root
[`AGENTS.md`](../AGENTS.md) is the primary repository-wide entry point. The
`backend/AGENTS.md` and `frontend/AGENTS.md` files add rules only within their own
directories. The [product requirements](requirements.md) define intended behavior,
and the [executable development plan](development-plan.md) controls sequencing,
decision gates, and completion evidence. This document provides the fuller engineering
rationale and workflow.

## 1. Architectural baseline

The following choices are project constraints, not suggestions:

- HubCR is a modular monolith during the MVP phase.
- Go implements the business control plane and the asynchronous worker.
- CNCF Distribution implements the OCI data plane. Do not reimplement `/v2/`, blob,
  manifest, upload, or download behavior in the control plane.
- PostgreSQL is the durable business database and initial job queue.
- Redis is reserved for cache, rate-limit state, and short-lived coordination.
- OCI content is stored through Distribution in S3-compatible storage; local
  development uses MinIO.
- Long-running scans and verification work run asynchronously in the worker.
- The web application uses Next.js App Router, React, TypeScript, Tailwind CSS,
  TanStack Query, and Zod.

Changing one of these decisions requires a documented architecture decision and
maintainer approval before implementation.

## 2. Required development workflow

For every change:

1. Read `AGENTS.md`, the requirements, the active development-plan milestone, the
   relevant architecture documentation, and any nearer directory-specific
   instructions.
2. Inspect the current implementation and Git status before editing. Preserve
   unrelated user changes.
3. State the behavior or invariant being changed and identify the owning module.
4. Add or update focused tests together with behavior changes.
5. Keep English and Simplified Chinese documentation synchronized.
6. Run targeted checks while developing, then run `make check` before declaring the
   work complete.
7. Report what was verified and what could not be verified, especially Docker or
   external-service behavior.
8. Proactively ask whether the user wants the completed changes committed and pushed
   to the configured remote repository.

For planned product delivery, reference the relevant requirement and work-package IDs.
Do not begin a blocked work package by choosing an unresolved policy. Update plan state
only when acceptance evidence has changed.

Do not commit, push, tag, publish, delete data, or change external systems unless the
user explicitly authorizes that action. The required question in step 8 is not
authorization by itself.

## 3. Repository and dependency boundaries

| Path | Responsibility | Must not do |
| --- | --- | --- |
| `backend/cmd/api` | Parse startup concerns and run the control plane | Contain domain behavior |
| `backend/cmd/worker` | Parse startup concerns and run background processing | Implement job business rules directly |
| `backend/internal/app` | Compose modules and adapters | Become a general utility package |
| `backend/internal/modules` | Own business capabilities and their contracts | Depend on HTTP, CLI, or process entry points |
| `backend/internal/platform` | Configuration and infrastructure adapters | Own product policy |
| `backend/migrations` | Evolve the PostgreSQL schema | Rewrite published migration history |
| `frontend/app` | Next.js routes, layouts, loading, and error boundaries | Accumulate reusable domain logic |
| `frontend/features` | Own feature UI, hooks, and feature models | Depend on unrelated features without an explicit shared boundary |
| `frontend/lib` | Shared typed clients and low-level utilities | Become a dumping ground for feature behavior |
| `frontend/providers` | Application-wide client providers | Turn the whole application into a Client Component |
| `deployments` | Local and production deployment assets | Encode application domain rules |

Cross-module backend behavior must go through explicit application services or
interfaces. Avoid package cycles, hidden global state, and generic `utils` packages.
Start a new process or service only when there is a measured deployment or scaling
need; do not split the modular monolith by default.

## 4. Go backend standards

- Use the Go version declared in `backend/go.mod`.
- Format all Go code with `gofmt`; keep `go vet ./...` clean.
- Prefer the standard library unless a dependency clearly reduces project risk or
  complexity. Explain new production dependencies in the change description.
- Pass `context.Context` through request, persistence, and worker boundaries.
- Wrap errors with operational context while preserving the underlying error.
- Use structured `log/slog` logging. Never log credentials, tokens, authorization
  headers, or sensitive personal data.
- Validate environment configuration at startup and fail with an actionable error.
- Keep `main` functions and HTTP handlers thin. Business decisions belong to modules.
- Define interfaces where they are consumed, not in infrastructure packages.
- Avoid mutable package globals and implicit initialization side effects.
- Use graceful shutdown for servers and workers.

Tests should be colocated as `*_test.go`, deterministic, and independent of external
services unless explicitly marked as integration tests.

## 5. Frontend standards

- Follow the installed Next.js documentation in `frontend/node_modules/next/dist/docs`
  and the additional rules in `frontend/AGENTS.md` before changing framework code.
- Use Server Components by default. Add `"use client"` only at the smallest boundary
  that needs state, effects, browser APIs, or client-only libraries.
- Keep routes in `app`, product behavior in `features`, shared API code in `lib/api`,
  and global client providers in `providers`.
- Keep TypeScript strict. Do not introduce `any` to bypass a model or API mismatch.
- Validate untrusted API responses with Zod at the boundary.
- Use TanStack Query for client-side server state; do not duplicate its cache in
  component state.
- Preserve accessibility: semantic elements, keyboard operation, visible focus, and
  labels are required for interactive controls.
- Use Tailwind utilities and existing design tokens. Avoid arbitrary one-off styling
  systems or a new component library without approval.
- Keep user-facing state truthful: loading, empty, unavailable, failed, and completed
  states are distinct.

Frontend tests use Vitest and live next to the behavior or shared API schema they
cover. Add Playwright coverage when complete user workflows are introduced.

## 6. API and persistence standards

- Business APIs are REST endpoints under `/api/v1`; `/token` and `/v2/` retain their
  Registry-specific protocol paths.
- Treat all request data as untrusted and validate it before calling domain logic.
- Keep transport DTOs separate from database records and domain entities.
- Do not expose internal errors, SQL details, secrets, or stack traces in responses.
- Make retryable write operations idempotent where practical.
- Use transactions for state changes that must remain atomic.
- Store timestamps in UTC and return an explicit timezone in serialized values.
- Add indexes based on known access patterns and verify uniqueness constraints at the
  database boundary.
- After the first public release, migrations are append-only. Never edit a migration
  that may have been applied outside the local machine.

## 7. OCI and security invariants

These rules are mandatory:

- Distribution owns OCI protocol and blob transport; HubCR owns business metadata and
  authorization.
- A repository is explicitly `PUBLIC` or `PRIVATE`; unavailable data must not silently
  become public or be represented as a successful empty value.
- Registry tokens are short-lived and contain an exact repository plus allowed
  actions such as `pull`, `push`, or `delete`.
- Browser sessions are never used as long-lived Registry credentials.
- Physical blob deduplication never bypasses repository-level authorization.
- Artifact identity is the immutable digest. Tags are mutable references.
- Scan reports, SBOMs, signatures, and verification results are keyed by artifact
  digest; verification also records the signature and trust-policy version.
- Signature discovery, cryptographic validity, and policy trust are separate states.
- Push success does not wait for scanning or verification unless a confirmed policy
  explicitly requires synchronous blocking.
- Never commit real secrets. Development defaults must be visibly marked as unsafe
  for shared or production use.

## 8. Product decisions that remain open

Do not silently choose or encode the following policies:

- public SaaS first versus private deployment first;
- self-registration versus invitation-only accounts;
- final organization role names and capabilities;
- organization-role inheritance versus repository-specific grants;
- whether pulls are blocked while scans are pending or failing;
- fixed keys, organization keys, OIDC keyless signing, or a combined trust model;
- the final production target: Compose, Kubernetes, or both;
- billing, quotas, retention, and public-content governance.

Record an accepted decision in `docs/` before implementing schema or public API
contracts that depend on it.

## 9. Documentation and localization

- English is the canonical project-documentation language.
- Every user-facing Markdown document has a `.zh-CN.md` counterpart and a language
  switch at the top.
- Change both language versions in the same change. They must describe the same
  behavior, status, commands, and limitations.
- Code identifiers, paths, commands, environment variables, protocol names, and API
  examples remain unchanged in translations.
- Do not document planned functionality as implemented.
- Update README status tables and architecture documents when project boundaries or
  available commands change.

AI instruction adapters such as `CLAUDE.md` and `GEMINI.md` import the nearest
`AGENTS.md`; `.github/copilot-instructions.md` points to the root hierarchy. Keep the
root, backend, and frontend instruction scopes separate and do not fork them into
independently maintained adapter copies.

## 10. Quality gates and definition of done

The repository-wide gate is:

```bash
make check
```

It verifies Go formatting, `go vet`, Go tests, TypeScript, frontend tests, ESLint, and
the Next.js production build.

A change is complete only when:

- the owning module and architecture boundaries are respected;
- behavior changes have appropriate automated tests;
- `make check` passes from a clean dependency state;
- affected English and Simplified Chinese documents are synchronized;
- no secrets, generated build output, dependency directories, or editor state are
  included;
- runtime checks are performed when practical, and untested external-service paths
  are clearly reported;
- Git status contains only intentional changes.
