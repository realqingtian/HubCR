# HubCR Registry MVP threat model

**English** | [简体中文](security-threat-model.zh-CN.md)

- Review package: M3-06 through M4-05 security-workflow extensions
- Review date: 2026-08-09
- Scope: all 226 files in Git snapshot `f691ff0899fb63a9371d29fd5991f5a90ae686b9`, followed by remediation in the active M3 worktree
- Method: repository-wide static review, source-to-sink validation, attack-path calibration, focused regression tests, and the real Registry/Web acceptance runner

This document records implemented security boundaries. D-009 and the M3-07 subset of
D-010 selected the single-host deployment and manual recovery contract. D-007 governs
the implemented informational security flow, and D-008 governs versioned namespace
signature trust. Retention, deletion, Pull enforcement, and product trust-policy
management remain outside active controls.

## Assets and trust boundaries

| Asset | Boundary that must hold |
| --- | --- |
| Passwords and web sessions | Every password check passes one bounded admission control; browser sessions remain revocable and separate from Registry tokens. |
| Registry tokens | `/token` authenticates independently and issues a short-lived JWT for the exact service, repository, and allowed action subset. |
| Private metadata | Namespace and repository authorization runs before Artifact/Tag access, and one browser principal cannot inherit another principal's query cache. |
| OCI manifests and blobs | Distribution owns `/v2/` transport; physical digest deduplication never grants repository access. |
| Registry event reconciliation | Distribution events require an independent generated secret and remain untrusted, bounded, idempotent inputs. |
| Scan and SBOM evidence | One workflow is bound to an exact repository plus immutable Digest; validated findings, CycloneDX documents, and tool versions persist in PostgreSQL. |
| Signature and trust evidence | Cryptographic validity is verified independently from versioned namespace policy trust; exact public-key or keyless identity rules and immutable historical results bind to the repository plus Digest. |
| Worker Registry credentials | The worker mints short-lived tokens restricted to exact-repository `pull`; no browser session or broad Registry credential is reused. |
| PostgreSQL, Redis, and MinIO data | Published development ports remain on `127.0.0.1`; production publishes only a loopback gateway, while data services stay private to the Compose network. |
| CI execution | Third-party actions are identified by reviewed immutable commit SHAs and run with read-only repository permission. |

## Attacker capabilities

- Send unauthenticated, concurrent, malformed, or deliberately slow requests to the
  public login, token, and API routes.
- Control usernames, passwords, Registry scopes, request headers, and request timing.
- Operate a legitimate low-privilege account and attempt cross-tenant reads.
- Reach a developer workstation from the same network when the host exposes a port.
- Reuse values documented in the repository and use two accounts sequentially in one
  browser profile.
- Benefit from movement or compromise of a mutable third-party CI reference.
- Publish a deliberately complex or vulnerable OCI Artifact and influence scanner
  output size, duration, and parse inputs within Registry transport limits.

## Reviewed threats and controls

| Threat | Before remediation | Implemented control | Residual limitation |
| --- | --- | --- | --- |
| Password guessing and Argon2 resource exhaustion | Web login used an allow-all limiter and Registry authentication bypassed it. | Both paths call `AuthenticatePasswordAttempt`. The process admits at most 10 attempts per normalized account and 60 per direct client per minute, stores at most 10,000 counters, fails closed when full, and permits at most four concurrent password verifications. Registry returns `429 TOOMANYREQUESTS`; Web returns `rate_limited`. | State is process-local. A multi-replica or shared deployment requires a Redis-backed limiter and an explicitly trusted proxy/client-address policy. |
| Cross-session browser cache disclosure | A session-expiry login replaced only `auth/me`, retaining identity-independent private queries. | Every newly authenticated principal removes all non-session queries and cached mutations before installing the new current-user record, while preserving the active session observer. | Query-key identity namespacing remains optional defense in depth. |
| Slow request exhaustion | The Go server bounded only headers; Registry streaming proxy settings applied to `/token` and `/api/`. | The API now has read-header, full-read, write, idle, and header-size limits. Long unbuffered 900-second behavior is scoped only to Distribution `/v2/`; API and token routes use bounded buffered defaults. | Practical capacity thresholds still require deployment-specific load testing. |
| Reachable development data services | Compose published PostgreSQL, Redis, MinIO, and the gateway on all interfaces with development credentials. | Every development host port is bound to `127.0.0.1`; production resets all infrastructure ports and publishes only the loopback gateway for an external HTTPS reverse proxy. | Host hardening, TLS termination, firewall policy, and capacity remain operator responsibilities. |
| Forged Registry events using a known token | A committed development token was accepted by both Distribution and the API. | `registry-keygen` generates a random ignored `event-token` with mode `0600`; Make injects it into both processes. The example environment has no token value and Compose fails if none is supplied. | Shared deployments must inject and rotate their own independent secret. |
| Mutable CI dependencies | Workflow actions used movable major-version tags. | Checkout, Go setup, and Bun setup are pinned to full official commit SHAs; `make check-workflows` prevents regression. | Pin updates still require human review of the referenced upstream change. |
| Scanner command or output abuse | A private image and external scanner can consume resources or emit malformed/large output. | The pinned Trivy adapter uses fixed arguments, an environment-only exact-scope token, per-job cancellation, serialized cache access, bounded output, schema validation, normalized findings, and no raw stderr logging. Database updates run without the private token. | Artifact complexity and scanner/database availability still affect bounded worker capacity; operators must monitor jobs and size resources. |
| Signature tool or evidence abuse | An Artifact can carry malformed, oversized, misleading, or cryptographically invalid signature material, while an external verifier receives private Registry access. | The pinned Cosign adapter uses fixed arguments, an environment-only short-lived exact-scope token, bounded output, per-job cancellation, schema validation, and scratch cleanup. In-process public-key verification supports only reviewed algorithms; exact key/keyless policy matching remains separate from validity and every result records tool and policy versions. | Artifact complexity and Cosign/Registry availability still consume bounded worker capacity. Keyless identity verification depends on Cosign's certificate and transparency-log verification; trust-policy management is not yet a supported product API/UI. |
| Scanner worker signing-key exposure | Exact-scope Pull tokens require signing, widening the Registry private-key mount from API to worker. | Production mounts the signing directory read-only; application code only issues short-lived exact-repository `pull` scopes, and Distribution info logs are disabled to avoid printing notification authorization configuration. | A compromised worker process with the private key can mint broader valid tokens despite application-level scope checks. Single-host MVP operators must isolate the worker and rotate the Registry key after compromise; a later internal token broker should remove this mount. |
| Artifact-to-workflow crash gap | A process crash after Artifact persistence could leave security work absent. | Event handling ensures the unique workflow immediately, and the worker periodically repairs Artifacts missing a workflow. Database uniqueness yields one current scan and SBOM intent. | Detection is delayed by the configured repair interval when the immediate ensure step is interrupted. |

The authorization and Registry JWT review found no reportable bypass in the current
exact-repository, action-intersection, audience, expiry, signature, private-404, and
cross-tenant controls. This statement applies only to the reviewed source and tested
MVP journeys, not to future grants, robots, retention, or security-policy features.

## Verification contract

M3-06 is complete only when all of the following pass in the same worktree:

- focused Go authentication, Registry protocol, configuration, HTTP-server, and key-generation tests;
- the frontend principal-transition regression plus full frontend tests and build;
- rendered Compose validation and the real `make test-m3-artifact-e2e` journey;
- `make check`, `make check-docs`, `make check-security-config`,
  `make check-workflows`, and `git diff --check`.

M3-06 itself made no production or recovery claim. The later accepted G-04 subset is
implemented and separately verified by `make test-m3-backup-restore-e2e`: it builds
the complete single-host topology, confirms write-service shutdown, checksums the
PostgreSQL and Registry-object backup, excludes Registry secrets, restores into clean
isolated volumes with rotated signing material, applies current migrations, and
proves login, private Pull, Artifact/Tag state, and immutable Digest consistency.

The M4-02 through M4-05 extensions are separately verified by focused security,
Registry-token, Trivy, Cosign, trust-policy, configuration, frontend-contract, and HTTP
authorization tests; isolated PostgreSQL workflow, repair, concurrency, history, and
result-state tests; `make test-integration`; frontend unit/build and Playwright checks;
and the real `make test-m4-security-e2e` scan/SBOM, signature matrix, retry/restart,
policy re-evaluation, authorized API, and tool-version runner.
