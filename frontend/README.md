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

The current control-plane workspace supports local-session login, the explicit
personal namespace, personal and organization repository metadata, organization
creation, and member addition. API responses are validated with Zod and server state
is owned by TanStack Query. The UI deliberately exposes loading, empty, validation,
denial, and unavailable states; backend authorization remains the security boundary.

Browser requests use the same-origin `/api` path. In local development and standalone
Next.js operation, `HUBCR_CONTROL_PLANE_URL` selects the server-side rewrite target and
defaults to `http://127.0.0.1:8080`. A deployment gateway may route `/api` directly.
`NEXT_PUBLIC_API_BASE_URL` remains an optional public override for an already
CORS-enabled endpoint and must never contain a secret.

```bash
bun run typecheck
bun run test
bun run lint
bun run build
bun run test:e2e
```

The Playwright suite builds and starts the production web server, then uses
browser-level control-plane mocks for deterministic workflow and failure-state tests.
Run the backend PostgreSQL integration suite separately for persistence and
authorization evidence.

From the repository root, `make test-m1-e2e` additionally provisions isolated
PostgreSQL, seeds test-only identities through GORM, starts the real Go API and the
same-origin Next.js proxy, and runs required M1 journeys 1–3 in Chromium. The fixture
under `backend/internal/testsupport` is not a product registration or bootstrap path.
