# HubCR web

**English** | [简体中文](README.zh-CN.md)

The HubCR web application uses Next.js App Router, React, TypeScript, Tailwind CSS,
TanStack Query, and Zod.

```bash
bun install
bun run dev
```

Application routes live in `app`, product code is grouped in `features`, typed API
access lives in `lib/api`, and client-only global providers live in `providers`.

The authenticated workspace shell now provides navigation and session state across
the overview, `/namespaces/[namespace]`, and
`/namespaces/[namespace]/repositories/[repository]` routes, including immutable
`/artifacts/[digest]` detail below a repository. The overview retains
personal and organization repository management, organization creation, and member
addition. Namespace discovery and repository detail use typed, Zod-validated API
responses and TanStack Query. Loading, empty, validation, denial, not-found, and
unavailable states remain distinct; backend authorization remains the security
boundary. Every successful principal replacement removes all non-session queries and
cached mutations before installing the new current user, while preserving the active
session observer. A session-expiry login therefore cannot render the previous
account's private repository or Artifact metadata.

Repository detail independently loads current mutable Tags and immutable Artifacts.
It also renders Registry quick-start commands from explicit repository Visibility and
the caller-specific `can_pull`/`can_push` policy result returned by the detail API.
Public read-only callers do not see login or Push commands; private readers see login
and Pull only. The page never treats the Web session as a Registry credential.
Every Tag links to the exact Digest detail, and Index details distinguish an unknown
descriptor set from a confirmed empty set. Access denial, API failure, connection
unavailability, loading, and empty results remain explicit. The page does not infer
scan, signature, cryptographic-validity, or trust state from Artifact metadata. Its
authorized, digest-bound security panel consumes the backend contract and keeps
loading, absent, queued, running, failed, stale, unavailable, unsigned, invalid,
unverified, untrusted, and trusted states distinct. Missing evidence is never rendered
as a clean scan or trusted signature.

Browser requests use the same-origin `/api` path. In local development and standalone
Next.js operation, `HUBCR_CONTROL_PLANE_URL` selects the server-side rewrite target and
defaults to `http://127.0.0.1:8080`. A deployment gateway may route `/api` directly.
`NEXT_PUBLIC_API_BASE_URL` remains an optional public override for an already
CORS-enabled endpoint and must never contain a secret.

The production image uses the installed Next.js standalone-output contract and runs
as a non-root user. In the accepted single-host Compose topology, the gateway routes
Web and `/api/` traffic through one HTTPS origin; the Web container has no published
host port. User-visible behavior is summarized in the [MVP user guide](../docs/user-guide.md)
and bounded by the [release limitations](../docs/release-limitations.md).

```bash
bun run typecheck
bun run test
bun run lint
bun run build
bun run test:e2e
```

The Playwright suite builds and starts the production web server, then uses
browser-level control-plane mocks for deterministic workflow, dynamic-route,
Artifact descriptor-knowledge, Visibility/Capability quick-start, security-state,
non-disclosure, denial/failure, keyboard, and mobile-width tests. Run the backend PostgreSQL integration suite
separately for persistence and authorization evidence.

From the repository root, `make test-m1-e2e` additionally provisions isolated
PostgreSQL, seeds test-only identities through GORM, starts the real Go API and the
same-origin Next.js proxy, and runs required M1 journeys 1–3 in Chromium. The fixture
under `backend/internal/testsupport` is not a product registration or bootstrap path.

`make test-m3-artifact-e2e` additionally runs the real Docker/Distribution security
matrix, waits for push-event reconciliation, starts the production web application
against that control plane, and proves the pushed `smoke` Tag and immutable Digest
are discoverable in Chromium.
