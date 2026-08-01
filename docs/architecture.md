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
- `backend/internal/platform/httpapi/artifacthandler`: authorized Artifact/Tag read adapter
- `backend/internal/platform/httpapi/registryeventhandler`: authenticated internal event adapter
- `backend/internal/platform/observability`: bounded process metrics and internal exposition adapters
- `backend/internal/platform/postgres/artifactstore`: GORM/PostgreSQL Artifact adapter
- `backend/migrations`: PostgreSQL schema migrations
- `frontend/app`: Next.js routes and layouts
- `frontend/features`: product feature code
- `frontend/lib`: shared typed API client and utilities
- `frontend/providers`: application-wide client providers
- `deployments/compose`: local infrastructure

The current web route boundary uses a shared authenticated shell around `/`,
`/namespaces/[namespace]`, and
`/namespaces/[namespace]/repositories/[repository]`. Route files validate dynamic OCI
name components and delegate API-backed presentation to the `namespaces` and
`repositories` features; they do not own authorization decisions.

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

## Deferred product decisions

The scaffold deliberately does not choose registration policy, organization roles,
repository-level grant inheritance, pull-blocking policy, signature trust roots, or
the final deployment target. These decisions affect schema and API contracts and
must be confirmed before their modules are implemented.
