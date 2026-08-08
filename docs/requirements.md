# HubCR product requirements

**English** | [简体中文](requirements.zh-CN.md)

- Status: working baseline
- Last reviewed: 2026-08-09
- Applies to: HubCR MVP and the explicitly deferred roadmap

This document turns the original project discussion into a requirements baseline that
matches the repository as it exists today. It describes intended product behavior,
not a claim that every requirement has already been implemented. Current implementation
truth remains visible in [README.md](../README.md), and delivery sequencing lives in
the [development plan](development-plan.md).

## 1. Source and decision precedence

The baseline combines the project discussion recorded on 2026-08-01 with the current
code, configuration, architecture, and development standards. When sources disagree:

1. current code and configuration define what is implemented;
2. accepted decisions and repository architecture define what may be built;
3. this document defines product intent and acceptance boundaries;
4. roadmap ideas remain deferred until their decision gates are approved.

An open question is not permission for an implementer or AI agent to choose a policy.
Decisions listed in section 10 must be approved and recorded before dependent public
schemas, APIs, or user flows are finalized.

## 2. Product definition

HubCR is an open-source OCI container image hub for individuals and organizations. It
provides a Docker Hub-style product experience while retaining control of identity,
namespaces, repository visibility, authorization, metadata, and software supply-chain
policy. CNCF Distribution remains responsible for OCI protocol and content transfer.

The canonical image path is:

```text
hubcr.io/{namespace}/{repository}:{tag}
```

Primary users are:

- individual developers publishing and consuming images in a personal namespace;
- organization members collaborating through organization-owned repositories;
- repository administrators controlling visibility and access;
- security reviewers assessing vulnerabilities, signatures, and trust state;
- operators deploying and maintaining a HubCR instance.

## 3. Goals and non-goals

### 3.1 MVP goals

- Provide authenticated user sessions and an automatically associated personal
  namespace.
- Provide organizations, membership, and an approved baseline role model.
- Create and manage explicitly public or private repositories.
- Let OCI clients authenticate and receive short-lived, repository-scoped Registry
  tokens.
- Support image push and pull through CNCF Distribution without reimplementing the
  Distribution API.
- Present repository, tag, manifest, artifact digest, and basic ownership metadata in
  the control plane and web application.
- Run a reproducible local development environment with PostgreSQL, Redis, MinIO, and
  Distribution.
- Preserve authorization and immutable-digest invariants from the first usable MVP.

### 3.2 Current and post-MVP security goals

- Asynchronous Trivy scanning, SBOM generation, Cosign signature/attestation
  verification, versioned trust evaluation, and truthful authorized Web evidence are
  current M4 capabilities.
- Robot accounts, personal access tokens, audit logs, quotas, webhooks, retention,
  garbage collection, replication, and proxy caching.
- Public-service controls such as email verification, password recovery, MFA, abuse
  prevention, billing, high availability, and multi-region delivery.

### 3.3 Non-goals

- Reimplementing `/v2/`, manifests, blobs, uploads, downloads, or storage drivers.
- Starting the MVP as a collection of independently deployed microservices.
- Treating a mutable tag as the identity of an artifact or security result.
- Treating signature discovery as proof of cryptographic validity or policy trust.
- Committing to public SaaS, billing, Kubernetes, or pull-blocking behavior before the
  corresponding decision is approved.

## 4. Current implementation baseline

As of 2026-08-09, the repository contains:

| Area | Implemented now | Not implemented yet |
| --- | --- | --- |
| Go API | Process composition, PostgreSQL lifecycle, health, local session, organization/member, policy-protected repository and Artifact/Tag APIs, Registry token issuance, authenticated Distribution push-event ingestion, Registry operational metrics/logs, and an authorized digest-bound scan/SBOM status API | Account bootstrap/invitation redemption |
| Go worker | PostgreSQL-backed unique intents, atomic leased claims, bounded concurrency, retry/backoff, dead-letter state, cancellation, graceful shutdown, crash-safe reclaim, Artifact workflow repair, pinned Trivy scan/CycloneDX SBOM handlers, and Cosign verification with versioned trust evaluation | Later approved security enforcement |
| Web | Authenticated shell with overview, namespace, repository, policy-backed Registry quick-start, Artifact/Tag lists, immutable Digest detail routes, and runtime-validated scan/SBOM/signature/trust evidence | Account bootstrap/invitation redemption and public discovery |
| OCI data plane | Token-protected local Distribution gateway backed by MinIO with authorized Docker/OCI checks and push-event delivery to the control plane | Delete-event reconciliation and approved lifecycle behavior |
| Infrastructure | Local infrastructure plus a digest-pinned complete single-host Compose topology, explicit migration command, manual PostgreSQL/Registry-object backup, checksum-verified restore, worker PostgreSQL composition, non-authoritative Trivy cache, private Cosign scratch space, and versioned GORM migrations through `000009_signature_trust` | Redis application state, Kubernetes and high availability |
| Quality | Go and Web unit checks, isolated PostgreSQL persistence/HTTP/cross-tenant tests, deterministic Playwright state tests, real browser journeys, the complete M2 Docker/OCI authorization matrix, Push-to-Web Artifact discovery, clean-volume recovery rehearsal, and real Trivy/CycloneDX/Cosign trust-state acceptance with worker restart and retry | Wider host/client compatibility and capacity evidence |

The Registry MVP is a release candidate with one bounded deployment and recovery
contract. It is not a general-purpose production service; account bootstrap,
operator-supplied TLS, secret recovery, capacity, compatibility, trust-policy
administration UI/API, and scan-based Pull blocking remain explicit release limitations.

## 5. Functional requirements

Priority meanings: **MUST** is required by the current or a completed accepted
milestone, **SHOULD** is expected when it does not block that milestone, and
**DEFERRED** belongs to a later or unapproved milestone.

### 5.1 Identity and sessions

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-ID-001 | MUST | A user can authenticate through the approved initial identity method and receive a revocable web session. |
| FR-ID-002 | MUST | The control plane identifies the current user on every protected API request and rejects invalid, expired, or revoked sessions. |
| FR-ID-003 | MUST | Each user has one stable personal namespace with a unique, normalized name. |
| FR-ID-004 | MUST | Passwords are never stored in plaintext, and every local password-verification endpoint uses bounded attempt and concurrency admission. |
| FR-ID-005 | DEFERRED | Email verification, password recovery, MFA, and additional OIDC providers follow the public-service decision. |

[D-002](decisions/d-002-registration.md) and
[D-003](decisions/d-003-initial-identity.md) select administrator invitations and
local username/password credentials with revocable server-side sessions for the MVP.

### 5.2 Organizations, namespaces, and authorization

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-ORG-001 | MUST | An authorized user can create an organization with a globally unique namespace. |
| FR-ORG-002 | MUST | Organization membership has an explicit approved role; capability checks do not rely on UI visibility alone. |
| FR-ORG-003 | MUST | Authorized organization members can list members and manage membership according to the approved role matrix. |
| FR-ORG-004 | MUST | Every namespace is owned by exactly one user or organization, and namespace ownership is auditable. |
| FR-AUTHZ-001 | MUST | Every protected control-plane and Registry access is authorized against the user, namespace, repository, and requested action. |
| FR-AUTHZ-002 | MUST | Denied or unavailable authorization data fails closed and never silently grants public access. |
| FR-AUTHZ-003 | SHOULD | Authorization decisions use a single backend policy boundary that can later emit audit records. |

[D-004](decisions/d-004-organization-roles.md) fixes the MVP role matrix, and
[D-005](decisions/d-005-grant-inheritance.md) selects organization-role-only access.

### 5.3 Repositories

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-REP-001 | MUST | An authorized namespace member can create a repository with a unique normalized name. |
| FR-REP-002 | MUST | Every repository stores explicit `PUBLIC` or `PRIVATE` visibility; there is no implicit public default caused by missing data. |
| FR-REP-003 | MUST | Users can list and view repositories they are authorized to discover. Public repository discovery rules must follow the approved product positioning. |
| FR-REP-004 | MUST | Authorized users can change visibility with a recorded timestamp and actor. |
| FR-REP-005 | SHOULD | Repository descriptions and basic metadata are editable without changing OCI identity. |
| FR-REP-006 | DEFERRED | Repository deletion, retention, quotas, transfer, and garbage collection require operations policies. |

### 5.4 Registry authentication and OCI data plane

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-REG-001 | MUST | `/v2/` is served by CNCF Distribution and issues a standards-compatible authentication challenge when credentials are required. |
| FR-REG-002 | MUST | `/token` authenticates the caller and issues a short-lived token limited to the exact repository and allowed `pull`, `push`, or `delete` actions. |
| FR-REG-003 | MUST | Public pull, private pull, authorized push, unauthorized access, expired token, and cross-repository token reuse all have automated acceptance coverage. |
| FR-REG-004 | MUST | Physical blob deduplication never bypasses repository-level authorization. |
| FR-REG-005 | MUST | Docker or another OCI client can push and pull a test image through the supported local gateway path. |
| FR-REG-006 | SHOULD | Token issuance and authorization failures emit structured, secret-safe operational logs. |

Web sessions and Registry tokens are separate credentials. Registry token lifetime,
signing-key management, and gateway topology must be recorded before FR-REG-002 is
implemented.

### 5.5 Artifacts, manifests, and tags

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-ART-001 | MUST | HubCR records repository artifacts by immutable digest and reconciles relevant Distribution events idempotently. |
| FR-ART-002 | MUST | Tags are mutable references to artifact digests; moving or deleting a tag does not rewrite historical security results. |
| FR-ART-003 | MUST | Authorized users can list tags and inspect digest, media type, size when available, creation/discovery time, and platform metadata. |
| FR-ART-004 | MUST | Duplicate or retried registry events do not create duplicate artifacts, tags, or jobs. |
| FR-ART-005 | SHOULD | Multi-platform indexes expose their child manifests without inventing missing platform metadata. |

### 5.6 Asynchronous jobs

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-JOB-001 | MUST | M4 migrations create a durable PostgreSQL job foundation from an empty database. |
| FR-JOB-002 | MUST | Workers claim jobs atomically with leases, bounded retries and backoff, terminal dead-letter state, and crash-safe lease recovery. |
| FR-JOB-003 | MUST | A deterministic unique intent prevents duplicate current work for the same job kind, repository, artifact digest, and policy version. |
| FR-JOB-004 | MUST | Worker concurrency is bounded, handlers are idempotent and cancellable, and graceful shutdown stops new claims without silently losing persisted work. |

### 5.7 Supply-chain security

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-SEC-001 | MUST | A successful manifest push enqueues an asynchronous Trivy scan keyed by artifact digest. |
| FR-SEC-002 | MUST | Scan records include status, findings, severity, fix availability, scanner version, vulnerability database version, and timestamps. |
| FR-SEC-003 | MUST | HubCR generates or associates an SBOM with the immutable artifact digest. |
| FR-SEC-004 | MUST | Cosign signatures and attestations are discovered and verified without conflating presence, validity, and trust. |
| FR-SEC-005 | MUST | Verification records the artifact digest, signature digest, identity/key evidence, policy version, result, and verification time. |
| FR-SEC-006 | MUST | Failed jobs can be retried safely and expose truthful queued, running, completed, failed, and stale states. |

Security work remains asynchronous unless a separately approved policy explicitly
requires a pull decision to block on it.

### 5.8 Operations and administration

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-OPS-001 | SHOULD | Services expose dependency-aware readiness and process-level liveness checks. |
| FR-OPS-002 | SHOULD | Logs are structured, correlate requests and jobs, and never contain credentials or authorization headers. |
| FR-OPS-003 | DEFERRED | Robot accounts and access tokens are scoped, revocable, and display secrets only at creation. |
| FR-OPS-004 | DEFERRED | Audit logs capture security-relevant actor, action, target, result, and timestamp data. |
| FR-OPS-005 | DEFERRED | Quotas, retention, webhook delivery, replication, caching, and garbage collection each require an approved operating policy. |
| FR-OPS-006 | MUST | The supported MVP deployment provides a maintenance-window PostgreSQL and Registry-object backup, checksum-verified destructive restore, current migration application, and recovery acceptance without bundling Registry secrets. |

## 6. Domain and data constraints

The durable model now includes `User`, local credential, revocable web session,
administrator invitation, `Organization`, `OrganizationMember`, `Namespace`,
`Repository`, `Artifact`, current `Tag`, and ordered Manifest descriptor records.
The durable model also includes job, scan, SBOM, signature, and versioned trust-policy
records. Later operations work may add robot, token, and audit records.

Mandatory data constraints are:

- use stable opaque identifiers internally and normalized unique names for paths;
- store timestamps in UTC and return explicit timezones through APIs;
- enforce namespace and repository uniqueness at the database boundary;
- use transactions for membership, ownership, visibility, and tag changes that must
  remain atomic;
- bind artifacts and security results to validated OCI digests;
- design registry-event and worker writes for idempotent retry;
- do not expose database records directly as public transport contracts.

## 7. Non-functional requirements

### Security and privacy

- Fail closed on unavailable identity, policy, repository, or visibility data.
- Validate all external input, including names, pagination, Registry scopes, digests,
  event payloads, and URLs.
- Rotate and protect signing keys and secrets outside source control.
- Apply least privilege to database, Redis, object-storage, and Distribution access.
- Produce a threat model for session authentication and Registry token exchange
  before the first external deployment.
- Keep authentication admission state bounded and apply the same control to Web login
  and Registry credential verification; multi-replica deployment requires shared
  limiter state.

### Reliability and consistency

- Graceful shutdown must stop accepting work and bound outstanding request or job
  completion.
- Event handling and worker execution must tolerate duplicate delivery.
- Readiness must only report success when dependencies required for real traffic are
  usable.
- The accepted MVP recovery objective is the manual, downtime-based D-010 subset;
  numeric RPO/RTO and cross-region disaster recovery remain unapproved.

### Performance and scalability

- Image bytes flow directly through Distribution and object storage, not through the
  Go control plane.
- List APIs use bounded pagination and indexed access paths.
- Long-running scans and verification never occupy synchronous API or push handlers.
- Numerical latency, throughput, and capacity targets must be measured and approved
  before they are used as release gates.

### Compatibility and accessibility

- Registry workflows target standards-compatible Docker and OCI clients.
- The web application supports keyboard navigation, semantic controls, visible focus,
  and distinct loading, empty, unavailable, failed, and completed states.
- Local development is supported on Apple Silicon through Docker Desktop, Go, and
  Bun; additional host platforms require their own validation evidence.

## 8. Required user journeys

The Registry MVP must demonstrate these end-to-end journeys:

1. A user authenticates and views the correct personal namespace.
2. An authorized user creates an organization and manages a member under the approved
   role model.
3. An authorized user creates a private repository and later changes it to public.
4. A Docker client obtains a correctly scoped token and pushes an image.
5. An authorized client pulls a private image, while an unauthorized client is
   denied.
6. A public repository allows the approved anonymous or authenticated pull behavior.
7. The web UI lists the pushed tag and immutable digest after event reconciliation.
8. A token for one repository or action cannot be reused for another repository or
   action.

Security milestones add journeys for scan results, SBOMs, signature validity, trust
policy changes, job retry, and stale-result handling.

## 9. Registry MVP acceptance criteria

The MVP is complete only when all of the following are true:

- the decisions that affect its schema and public APIs are approved and recorded;
- migrations create the required identity, namespace, organization, repository,
  artifact, tag, and session foundations from an empty database;
- the required journeys in section 8 pass through supported local entry points;
- authorization tests cover public/private visibility, membership, allowed actions,
  denial, token expiry, and cross-repository isolation;
- Registry events reconcile artifacts and tags idempotently by digest;
- the web UI represents loading, empty, denial, unavailable, and successful states
  truthfully;
- `make check`, backend integration tests, frontend tests, and OCI end-to-end tests
  pass;
- no real secrets are committed and development defaults are clearly unsafe outside
  local use;
- setup, operation, API behavior, and limitations are documented in English and
  Simplified Chinese;
- a release candidate has explicit evidence for database migration, backup/restore,
  and supported Docker/OCI client compatibility.

## 10. Open decision register

| Decision | Needed before | Question |
| --- | --- | --- |
| [D-001 Product mode](decisions/d-001-product-mode.md) | Identity and discovery contracts | `ACCEPTED`: self-hosted/private deployment first; public SaaS deferred. |
| [D-002 Registration](decisions/d-002-registration.md) | User lifecycle schema and UI | `ACCEPTED`: administrator-issued, single-use expiring invitations. |
| [D-003 Initial identity](decisions/d-003-initial-identity.md) | Session implementation | `ACCEPTED`: local username/password credentials with revocable server-side sessions. |
| [D-004 Organization roles](decisions/d-004-organization-roles.md) | Membership migrations and APIs | `ACCEPTED`: `OWNER`, `ADMIN`, `WRITER`, and `READER` with the recorded capability matrix. |
| [D-005 Grant inheritance](decisions/d-005-grant-inheritance.md) | Authorization policy | `ACCEPTED`: organization-role-only access; repository-specific grants deferred. |
| [D-006 Public pull](decisions/d-006-public-pull.md) | Registry token flow | `ACCEPTED`: anonymous pull for explicit `PUBLIC` repositories through exact-scope short-lived tokens. |
| [D-007 Security enforcement](decisions/d-007-security-enforcement.md) | Pull policy | `ACCEPTED`: informational asynchronous scans first; no pull blocking in initial M4. |
| [D-008 Signature trust](decisions/d-008-signature-trust.md) | Verification schema | `ACCEPTED`: versioned namespace policy combining public keys and exact OIDC issuer plus subject identities. |
| [D-009 Production deployment](decisions/d-009-production-deployment.md) | Deployment contracts | `ACCEPTED`: single-host Docker Compose behind an operator-managed HTTPS reverse proxy; Kubernetes deferred. |
| [D-010 Operations policy](decisions/d-010-operations-policy.md) | Destructive/retention features | `ACCEPTED` for manual PostgreSQL plus Registry-object backup/restore and migration rehearsal; retention, deletion, GC, quotas, audit and numeric DR targets remain deferred. |
| D-011 License | First public release | Which open-source license governs HubCR? |

Each accepted decision should be written as a short architecture or product decision
record under `docs/decisions/`, linked from this table, and reflected in both language
versions.

## 11. Requirements maintenance

- Use requirement IDs in plans, issues, tests, and change descriptions when practical.
- Change `MUST` scope only with maintainer approval and a synchronized plan update.
- Update the current baseline table when functionality becomes usable.
- Record new uncertainty in the decision register instead of embedding an assumption
  in code.
- Review this document at every milestone exit and before planning the next milestone.
