# HubCR architecture

**English** | [简体中文](architecture.zh-CN.md)

HubCR separates the business control plane from the OCI data plane.

```text
Browser --------> Next.js web application
Docker CLI -----> gateway ---- /api/* --> Go control plane
                         |----- /token --> scoped token endpoint
                         `----- /v2/* ----> CNCF Distribution --> S3 / MinIO
                                               `-- push events --> Go control plane

Go control plane --> PostgreSQL
                 --> Redis
                 --> PostgreSQL job table --> Go worker --> Trivy / Cosign
```

## Repository boundaries

- `backend/cmd/api`: control-plane process entry point
- `backend/cmd/worker`: asynchronous worker process entry point
- `backend/internal/app`: application composition only
- `backend/internal/modules`: business capabilities with explicit ownership
- `backend/internal/platform`: configuration and infrastructure adapters
- `backend/internal/modules/registry`: Registry token and Distribution-event business contracts
- `backend/internal/modules/artifacts`: Artifact/Tag validation and persistence contracts
- `backend/internal/modules/jobs`: durable intent, lease, retry, and handler contracts
- `backend/internal/modules/security`: digest-bound scan/SBOM workflow, result, and tool-version contracts
- `backend/internal/platform/httpapi/artifacthandler`: authorized Artifact/Tag read adapter
- `backend/internal/platform/httpapi/securityhandler`: authorized Artifact security-state adapter
- `backend/internal/platform/httpapi/registryeventhandler`: authenticated internal event adapter
- `backend/internal/platform/observability`: bounded process metrics and internal exposition adapters
- `backend/internal/platform/postgres/artifactstore`: GORM/PostgreSQL Artifact adapter
- `backend/internal/platform/postgres/jobstore`: atomic PostgreSQL job adapter
- `backend/internal/platform/postgres/securitystore`: transactional workflow and scan/SBOM evidence adapter
- `backend/internal/platform/scanner/trivy`: bounded pinned-Trivy process adapter
- `backend/migrations`: PostgreSQL schema migrations
- `frontend/app`: Next.js routes and layouts
- `frontend/features`: product feature code
- `frontend/lib`: shared typed API client and utilities
- `frontend/providers`: application-wide client providers
- `deployments/compose`: local infrastructure and the accepted single-host deployment

The current web route boundary uses a shared authenticated shell around `/`,
`/namespaces/[namespace]`, and
`/namespaces/[namespace]/repositories/[repository]`, with immutable Artifact details
at `/artifacts/[digest]` beneath the repository route. Route files validate dynamic
OCI name and Digest components and delegate API-backed presentation to the
`namespaces`, `repositories`, and `artifacts` features; they do not own authorization
decisions. Artifact clients validate every response with Zod and TanStack Query owns
loading, error, retry, and successful server state. Repository Detail includes only
the centralized policy result (`can_pull` and `can_push`), allowing Quick-start to
select commands without copying the role matrix into the browser.

## Module rules

1. Modules own their domain behavior and persistence contracts.
2. HTTP handlers and database adapters depend on modules, not the reverse.
3. Cross-module behavior goes through explicit application services.
4. Distribution remains the source of blob and manifest transport and storage
   behavior.
5. HubCR owns repository-scoped Artifact/Tag business metadata. Authenticated
   Distribution push events reconcile that metadata; authorized read APIs expose it
   without taking over OCI transport.
6. HubCR remains the source of users, namespaces, visibility, authorization, and
   security policy.
7. Scan and signature results are keyed by immutable artifact digest.
8. Registry operational signals follow the same ownership boundary: the gateway and
   Distribution observe `/v2/` challenges and delivery queues; the Go control plane
   observes token decisions and notification reconciliation.
9. The authentication module owns password-attempt admission. Web login and Registry
   Basic authentication converge on the same bounded process-local limiter and
   concurrency gate before Argon2 work; a future multi-replica deployment must replace
   that state with the approved shared Redis adapter.
10. The jobs module owns unique intents and lifecycle transitions. PostgreSQL claims
    due or expired work atomically with `SKIP LOCKED`; handlers remain bounded,
    cancellable and idempotent, and a stopped worker leaves its lease for safe reclaim.
11. Artifact reconciliation ensures one scan and one SBOM intent for each immutable
    repository/Digest pair. A periodic repair pass closes the crash gap after Artifact
    persistence. The worker receives only a short-lived exact-repository Pull token;
    scanner output and cache are non-authoritative inputs to validated PostgreSQL
    evidence.

The local infrastructure boundary binds the Go listener and every published Compose
port to `127.0.0.1`. The production override adds API, worker, migration, and Web
containers, removes infrastructure host ports, and publishes only a loopback gateway
behind the required operator-managed HTTPS reverse proxy. PostgreSQL and MinIO are
the durable recovery boundary; Redis is non-authoritative in the MVP. Registry
streaming timeouts remain on `/v2/`; token and business API routes use bounded
buffered proxy behavior. See the
[Registry MVP threat model](security-threat-model.md) for reviewed attacker paths and
remaining deployment limitations.

## Deferred product decisions

Registration policy, organization roles, grant inheritance, public Pull, initial
informational security enforcement, signature trust, the first deployment target,
and the MVP backup subset now have accepted records. A later Pull-blocking policy,
deletion, retention, garbage collection, quotas, audit, numeric disaster-recovery
objectives, Kubernetes, and public-release licensing remain open and must be confirmed
before dependent implementation.
