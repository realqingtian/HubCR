# M5-01 Trust Policy management — design and implementation plan

**English** | [简体中文](m5-01-trust-policy-management.zh-CN.md)

- Status: `PROPOSED` design; product owner approved the M5-01 scope and boundaries on
  2026-08-09. This document captures the confirmed boundaries, the chosen re-verification
  trigger mechanism, and an ordered implementation plan.
- Decision basis: [D-008 Signature trust](decisions/d-008-signature-trust.md) (`ACCEPTED`)
  defines the versioned namespace trust model and trust evaluation. M5-01 adds the
  authorized management surface on top of the already-built, M4-accepted model. It does
  not change D-008 or D-007.
- Plan reference: [Development plan, section 10](development-plan.md#10-milestone-5--operations-and-public-service-readiness)
  (Milestone 5 — operations and public-service readiness).

## 1. Context and non-goals

### What already exists (do not redesign)

D-008 is `ACCEPTED` and the domain, storage, and verification engine are M4-accepted:

- Domain — `backend/internal/modules/security/trust.go`: `TrustPolicy` (versioned per
  namespace), `PublicKeyTrust` (sha256 fingerprint), `KeylessIdentity` (exact issuer +
  subject, no wildcards), `EvaluateTrust` pure function, `VerificationIntentKey` scoped by
  policy ID. Caps: `MaxTrustPolicySubjects=128`, `MaxPublicKeyBytes=16KiB`.
- Storage — `backend/internal/platform/postgres/securitystore/truststore.go`:
  `CreateTrustPolicy` locks the namespace row (`SELECT ... FOR UPDATE`) and computes
  `MAX(version)+1`. All foreign keys are `ON DELETE RESTRICT` (immutable history).
  "Current" is derived as `ORDER BY version DESC LIMIT 1` per namespace.
- Verification — worker handler `trust_handler.go` + cosign adapter. Cosign does
  discovery and cryptographic validity only; trust is evaluated separately via
  `EvaluateTrust`.
- Backfill — `combinedSecurityRepairer` (`backend/internal/app/worker/app.go`) runs
  periodically and already re-verifies artifacts when the current policy changes, because
  a new policy version has a new `policy_id` and therefore a new
  `(artifact, current_policy)` workflow row is absent until repaired.
- Read surface — read-only `GET .../artifacts/{digest}/security`
  (`platform/httpapi/securityhandler/handler.go`) and the read-only frontend panel
  `features/security/artifact-security-panel.tsx`. The detail reader already marks a
  result **stale** when `currentPolicy.ID != workflow.PolicyID`.

### What M5-01 adds (the gap)

- No HTTP management API (no create / list / read endpoint for policies).
- No `MANAGE_TRUST_POLICY` capability in `modules/authorization/policy.go`.
- No frontend trust-policy management UI.
- `TrustService.CreatePolicy` does no authorization today (by design — only the seed CLI
  `testsupport/m4trustseed` called it).
- Re-verification after a policy change only happens lazily on the next periodic repair
  tick; M5-01 adds an eager, namespace-scoped trigger so the delay is bounded and
  predictable.

### Non-goals (deferred, require separate approval)

- Editing or deleting historical policy versions — append-only immutable history stays.
- Repository-level or override-level trust policies — namespace scope only.
- Wildcard OIDC issuer or subject patterns — exact match only.
- Pull blocking based on trust state — D-007 keeps results informational; re-blocking
  requires a separate decision.
- Listing verification results across a namespace or policy — per-artifact reads only.
- A CLI for policy management — M5-01 ships API + web; a CLI can follow later.

## 2. Confirmed boundaries

These were approved by the product owner on 2026-08-09 and confirmed in the scoping
session:

| Boundary | Value |
| --- | --- |
| Who may manage | Namespace `OWNER` only. For a personal namespace, the personal owner; for an organization namespace, the organization `OWNER` role. |
| History model | Append-only immutable versions. No edit, no delete of historical versions. FKs stay `ON DELETE RESTRICT`. |
| Trust subjects | Exact public-key fingerprint; exact OIDC issuer + subject. No wildcards, no implicit trust. |
| Re-verification trigger | On policy creation, eagerly trigger a namespace-scoped repair, then let the periodic repair loop finish any remainder. |
| Pull behavior | Unchanged — results stay informational. No blocking (D-007 holds). |
| State separation | Signature presence, cryptographic validity, and policy trust remain separate states, keyed by digest. |

## 3. Re-verification trigger mechanism (confirmed)

The key open question was how re-verification fires after a new policy version is
created. The confirmed choice is **option C: create the policy, commit, then eagerly
trigger a namespace-scoped repair**, with the periodic repair loop as the backstop.

### Why this choice

- The mechanism already exists. `combinedSecurityRepairer.RepairMissingWorkflows` already
  re-verifies artifacts whose `(artifact, current_policy)` workflow is missing. A new
  policy version becomes "current" immediately on commit, so its workflow rows are
  missing until repaired.
- Eager trigger bounds the delay. Pure reliance on the periodic loop (option A) gives
  unbounded latency for large namespaces (many artifacts across many ticks). Eager
  trigger starts the work on policy creation instead of waiting for the next tick.
- The transaction stays clean. The policy writes commit first; the repair runs as a
  separate, independent operation. Enqueueing jobs inside `CreateTrustPolicy`'s
  transaction (option B) would cross module and table boundaries and is riskier.
- Load is bounded. Reuse the existing `cfg.Scanner.RepairBatch` limit per pass and let
  the periodic loop drain any remainder. No new concurrency parameter is introduced.

### How it composes with existing paths

1. `CreatePolicy` commits the new version (transaction closes).
2. The new version is now the namespace's current policy
   (`ORDER BY version DESC LIMIT 1`).
3. `RepairTrustVerificationForNamespace(namespaceID, batch)` runs once, scoped to that
   namespace: it selects `(artifact, current_policy)` pairs missing a workflow and calls
   the existing `EnsureCurrentVerification` for each.
4. Each `EnsureCurrentVerification` enqueues a `COSIGN_VERIFY` job with a policy-scoped
   `VerificationIntentKey` (`security-signature:<repo>:<digest>:policy:<newPolicyID>`)
   and inserts the workflow row, both in one transaction. Dedup is idempotent.
5. The worker claims and runs the jobs as it does today; results land keyed by digest +
   policy version. The read path marks earlier-version results `stale` until refreshed.
6. The periodic `combinedSecurityRepairer` continues to run and finishes any remainder
   beyond the first `RepairBatch`, and covers any artifacts pushed later.

### Failure and concurrency semantics

- The policy commit and the eager repair are decoupled. If the process dies between
  commit and the eager repair, the periodic loop still backfills — no work is lost.
- The eager repair is idempotent: `EnsureCurrentVerification` dedups on
  `(repo, digest, policy_id)` via `ON CONFLICT DO NOTHING`, and the job queue dedups on
  `intent_key`. Running the eager repair and the periodic loop concurrently is safe.
- A failed eager repair does not roll back the policy. The policy is committed and
  current; verification simply catches up asynchronously. This matches D-008, where
  changing a policy queues re-verification rather than blocking on it.

## 4. Backend design

### 4.1 Authorization capability

Add one capability to `modules/authorization/policy.go` and reuse an existing one:

- `ManageTrustPolicy Capability = "MANAGE_TRUST_POLICY"` (new) — gates POST (create new
  version).
- `organizationCapabilities[ManageTrustPolicy] = { RoleOwner: true }` — organization
  owners only.
- Extend `AllowsPersonalNamespace`'s owner switch to include `ManageTrustPolicy` so a
  personal namespace owner can manage their own policy.
- Reuse the existing `ViewOrganization` capability (already defined and granted to all org
  roles, but currently never enforced) to gate GET (read current policy). No change to its
  definition; this endpoint becomes its first enforcement site.

`ManageTrustPolicy` is `OWNER`-only (approved boundary). `ViewOrganization` grants read to
all org members plus the personal-namespace owner; non-members are denied. See section 9
for the rationale on read access.

### 4.2 Namespace-scoped repair

Add a namespace-scoped repair method alongside the existing global one. It mirrors
`RepairMissingVerificationWorkflows` but adds a `namespace_id` filter to the same CTE
query, and is exposed through `TrustStore`, `TrustService`, and the worker's
`combinedSecurityRepairer`-equivalent surface used by the control plane.

- `TrustStore.RepairMissingVerificationWorkflowsForNamespace(ctx, namespaceID, limit, now)`
  in `securitystore/truststore.go`.
- `TrustService.RepairTrustVerificationForNamespace(ctx, namespaceID, limit)` with the
  same `[1, MaxRepairBatch]` validation as the global repair.
- Reuse `cfg.Scanner.RepairBatch` as the per-pass limit (confirmed: no new parameter).

The eager trigger calls this once after `CreatePolicy` commits. It does not loop to
exhaustion; the periodic loop drains the remainder.

### 4.3 Authorized `CreatePolicy`

`TrustService.CreatePolicy` gains authorization:

- New parameter or a wrapper that takes the actor and resolves namespace access (the
  `repositories.Service.access` / `allows` pattern, or an equivalent `NamespaceAccess`
  lookup by namespace name/ID).
- Deny-by-default: if the actor lacks `ManageTrustPolicy`, return a forbidden error that
  the HTTP layer maps to `403`.
- Keep input validation as-is (subject count, key/identity validation).

### 4.4 HTTP management API (namespace level)

New routes on the existing `*httpapi.Router`, registered in
`app/controlplane/app.go`. Resource is namespace-level and singular (one current policy
per namespace), matching the domain model:

| Method | Path | Behavior | Auth |
| --- | --- | --- | --- |
| `GET` | `/api/v1/namespaces/{namespace}/trust-policy` | Return the current policy (highest version). `404` if none exists. | `ViewOrganization` (org Owner/Admin/Writer/Reader) or personal-namespace owner; non-members get `404`. |
| `POST` | `/api/v1/namespaces/{namespace}/trust-policy` | Create a new version. Append-only. Returns the new version with `201`. After commit, trigger the namespace-scoped repair. | `ManageTrustPolicy` (namespace `OWNER`). |

Notes:

- No `PUT`/`PATCH`/`DELETE` — append-only immutable history.
- The POST body carries public keys (with fingerprints) and/or keyless identities, with
  the same validation caps as the domain (`MaxTrustPolicySubjects`, `MaxPublicKeyBytes`).
- The response for GET/POST includes `id`, `version`, `public_keys`, `keyless_identities`,
  `created_by_user_id`, `created_at`.
- The POST response is minimal `201` + the new version; it does not include the
  eager-repair outcome (see section 9).
- Error mapping reuses the existing `mapError` pattern
  (`ErrForbidden` → `403`, `ErrNotFound` → `404`, `ErrInvalid`/`ErrConflict` → `400`/`409`).
  Read denial for a non-member resolves to `404` (not `403`), matching the existing
  repository discovery behavior that hides existence from unauthorized callers.

### 4.5 Wiring

In `app/controlplane/app.go`:

- `trustService` is already constructed (line 124) but only passed to the registry
  scheduler. Pass it (plus `authorization.Policy` and the namespace-access lookup) to a
  new trust-policy handler.
- Register the new routes alongside `securityhandler.RegisterRoutes`.

## 5. Frontend design

A namespace-level trust-policy management page, scoped to the M5-01 minimum:

- **View current policy**: show version, public-key fingerprints, OIDC issuer + subject
  pairs, created-by, created-at. Show an empty state when no policy exists.
- **Create new version form**: add public keys (with computed/entered fingerprint) and/or
  exact keyless identities; submit via `POST`. On success, refresh the view.
- Read-only history list is **out of scope** for M5-01 (deferred).

Conventions to follow (from existing `features/security/artifact-security-panel.tsx` and
the TanStack Query + Zod-validated API contract pattern):

- New feature folder under `frontend/features/` (e.g. `features/trust-policy/`).
- TanStack Query for the GET, a mutation for the POST, Zod schema for the API contract.
- Authorization is enforced by the backend; the UI hides/disables management controls
  when the current user is not the namespace owner (gated on the same access signal used
  elsewhere in the namespace view).
- Preserve the separation of presence / validity / trust in any reused rendering.

## 6. Security and policy review checklist

- [ ] `ManageTrustPolicy` is `OWNER`-only for both personal and organization namespaces;
  deny-by-default tests cover admin/writer/reader and anonymous callers.
- [ ] `ViewOrganization` GET read access: org Owner/Admin/Writer/Reader and the
  personal-namespace owner can read; a non-member (no membership relationship) gets `404`
  and the policy is never leaked.
- [ ] `CreatePolicy` performs authorization before any write.
- [ ] No new pull-blocking behavior is introduced (D-007 holds). Add a regression test
  that confirms a failed/untrusted verification does not affect pull tokens.
- [ ] Results stay keyed by digest + policy version; no tag-keyed trust state is added.
- [ ] Public-key material only (no private keys); fingerprints validated; size caps
  enforced.
- [ ] Authorization bypass test: a user from namespace A cannot manage namespace B's
  policy; cross-namespace isolation test in the integration suite.
- [ ] No real signatures are uploaded to the Sigstore transparency log in any test or
  acceptance script (keep the M4 incident guard in place).

## 7. Ordered implementation plan

Each step is independently testable and small enough to review as one unit.

### Step 1 — Authorization capability

- Add `ManageTrustPolicy` to the capability enum, `organizationCapabilities` map
  (`RoleOwner` only), and `AllowsPersonalNamespace` owner switch.
- Files: `backend/internal/modules/authorization/policy.go`.
- Tests: extend the policy table tests to cover personal owner, personal non-owner,
  organization owner, and each non-owner organization role (all deny except owners).

### Step 2 — Namespace-scoped repair (store + service)

- Add `RepairMissingVerificationWorkflowsForNamespace` to `TrustStore` and the postgres
  implementation (namespace-filtered CTE), and `RepairTrustVerificationForNamespace` to
  `TrustService` with `[1, MaxRepairBatch]` validation.
- Files: `backend/internal/platform/postgres/securitystore/truststore.go`,
  `backend/internal/modules/security/trust_service.go`, the `TrustStore` interface.
- Tests: integration test that creates a policy + artifacts, confirms the scoped repair
  enqueues workflows only for the target namespace and leaves other namespaces alone;
  idempotency test (running twice enqueues once).

### Step 3 — Authorized `CreatePolicy` + namespace access

- Add authorization to `CreatePolicy` using the namespace-access pattern, deny-by-default.
- Files: `backend/internal/modules/security/trust_service.go` (and the access lookup it
  needs).
- Tests: owner succeeds; non-owner and cross-namespace callers are forbidden; input
  validation unchanged.

### Step 4 — HTTP management API

- Implement the GET (current) and POST (create version) handlers at the namespace level,
  wire routes and dependencies in `app/controlplane/app.go`, and add the POST → eager
  namespace-scoped repair trigger after commit.
- Files: new `platform/httpapi/trustpolicyhandler/` (or extend `securityhandler`),
  `app/controlplane/app.go`, `docs/api.md` + `docs/openapi.yaml` + Chinese counterparts.
- Tests: handler unit tests (200/201/400/403/404/409), integration test of the full
  create-then-reverify flow against PostgreSQL, and a regression test that pull tokens
  are unaffected by trust state.

### Step 5 — Frontend management UI

- Add the view-current + create-new-version feature, TanStack Query + Zod contract, owner
  gating of management controls.
- Files: new `frontend/features/trust-policy/`, wiring in the namespace view, frontend
  API client + Zod schema.
- Tests: Vitest unit tests for the view and the create flow (success, validation error,
  forbidden); TypeScript, ESLint, production build.

### Step 6 — Documentation and acceptance

- Update `docs/api.md`, `docs/openapi.yaml`, `docs/user-guide.md`, and the release
  limitations, in both languages; record evidence in `docs/development-plan.md`.
- Acceptance: extend the M4 e2e script (or add an M5-01 script) that creates a policy via
  API, confirms artifacts re-verify under the new version, and confirms pull is
  unaffected — with transparency-log upload kept disabled.

## 8. Validation

- Per step: focused Go/frontend tests, then the full gate.
- Completion gate: `make check` (with the Go cache workaround
  `/private/tmp/hubcr-go-cache` in the sandbox), the relevant integration tests, and a
  runtime acceptance run.
- Report exactly what was verified; state any untested external path (e.g. real OIDC
  provider, real transparency log).

## 9. Resolved items

Confirmed in the scoping session on 2026-08-09, grounded in the existing code:

- **GET read access reuses `ViewOrganization`.** The `GET .../trust-policy` endpoint is
  readable by organization Owner/Admin/Writer/Reader and by the personal-namespace owner.
  Rationale: an org member can already read the same trust subjects indirectly through the
  per-artifact `GET .../artifacts/{digest}/security` endpoint (gated by repository
  discovery, which an org Reader passes for private repos). An owner-only GET would create
  an inconsistent asymmetry. `ViewOrganization` is already defined and granted to all org
  roles but is currently never enforced; this endpoint becomes its first real use, with no
  schema change. POST (create new version) stays owner-only via `ManageTrustPolicy`.
  Non-members (no membership relationship) cannot read the policy — they get `404`, never a
  leak.
- **POST response is minimal `201 Created` + the new policy version.** It does not include
  the eager-repair outcome (e.g. number of workflows enqueued). This matches every existing
  create endpoint (repository create, organization create), which returns `201` + the
  created resource only. No existing JSON response returns an async-work count; the only
  async-kicking endpoint (registry event webhook) returns `202` with an empty body and
  routes counts to logs/metrics. Frontend create mutations use invalidate-on-success and do
  not consume side-effect metadata. Eager-repair progress stays observable through
  per-artifact reads and the existing `stale` flag.
- **JSON field names.** `id`, `version`, `public_keys` (each with `fingerprint` and key
  metadata), `keyless_identities` (each with `issuer` and `subject`), `created_by_user_id`,
  `created_at`. These mirror the existing `TrustPolicy` domain and the read-only security
  response shapes.
