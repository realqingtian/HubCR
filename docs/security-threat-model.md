# HubCR Registry MVP threat model

**English** | [简体中文](security-threat-model.zh-CN.md)

- Review package: M3-06
- Review date: 2026-08-08
- Scope: all 226 files in Git snapshot `f691ff0899fb63a9371d29fd5991f5a90ae686b9`, followed by remediation in the active M3 worktree
- Method: repository-wide static review, source-to-sink validation, attack-path calibration, focused regression tests, and the real Registry/Web acceptance runner

This document records implemented security boundaries. It does not treat the open
production deployment, backup, retention, scanning, or trust-policy decisions as
active controls.

## Assets and trust boundaries

| Asset | Boundary that must hold |
| --- | --- |
| Passwords and web sessions | Every password check passes one bounded admission control; browser sessions remain revocable and separate from Registry tokens. |
| Registry tokens | `/token` authenticates independently and issues a short-lived JWT for the exact service, repository, and allowed action subset. |
| Private metadata | Namespace and repository authorization runs before Artifact/Tag access, and one browser principal cannot inherit another principal's query cache. |
| OCI manifests and blobs | Distribution owns `/v2/` transport; physical digest deduplication never grants repository access. |
| Registry event reconciliation | Distribution events require an independent generated secret and remain untrusted, bounded, idempotent inputs. |
| PostgreSQL, Redis, and MinIO development data | Published development ports remain on `127.0.0.1`; Compose-network access stays separate from host exposure. |
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

## Reviewed threats and controls

| Threat | Before remediation | Implemented control | Residual limitation |
| --- | --- | --- | --- |
| Password guessing and Argon2 resource exhaustion | Web login used an allow-all limiter and Registry authentication bypassed it. | Both paths call `AuthenticatePasswordAttempt`. The process admits at most 10 attempts per normalized account and 60 per direct client per minute, stores at most 10,000 counters, fails closed when full, and permits at most four concurrent password verifications. Registry returns `429 TOOMANYREQUESTS`; Web returns `rate_limited`. | State is process-local. A multi-replica or shared deployment requires a Redis-backed limiter and an explicitly trusted proxy/client-address policy. |
| Cross-session browser cache disclosure | A session-expiry login replaced only `auth/me`, retaining identity-independent private queries. | Every newly authenticated principal removes all non-session queries and cached mutations before installing the new current-user record, while preserving the active session observer. | Query-key identity namespacing remains optional defense in depth. |
| Slow request exhaustion | The Go server bounded only headers; Registry streaming proxy settings applied to `/token` and `/api/`. | The API now has read-header, full-read, write, idle, and header-size limits. Long unbuffered 900-second behavior is scoped only to Distribution `/v2/`; API and token routes use bounded buffered defaults. | Practical capacity thresholds still require deployment-specific load testing. |
| Reachable development data services | Compose published PostgreSQL, Redis, MinIO, and the gateway on all interfaces with development credentials. | Every development host port is bound to `127.0.0.1`; the Go API also defaults to `127.0.0.1:8080`. | These settings are not a production deployment design. G-04 remains open. |
| Forged Registry events using a known token | A committed development token was accepted by both Distribution and the API. | `registry-keygen` generates a random ignored `event-token` with mode `0600`; Make injects it into both processes. The example environment has no token value and Compose fails if none is supplied. | Shared deployments must inject and rotate their own independent secret. |
| Mutable CI dependencies | Workflow actions used movable major-version tags. | Checkout, Go setup, and Bun setup are pinned to full official commit SHAs; `make check-workflows` prevents regression. | Pin updates still require human review of the referenced upstream change. |

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

No production topology or backup/restore claim is made. M3-07 remains blocked until
the G-04 subset approves the supported MVP deployment and backup contract.
