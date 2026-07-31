# HubCR architecture

**English** | [简体中文](architecture.zh-CN.md)

HubCR separates the business control plane from the OCI data plane.

```text
Browser --------> Next.js web application
Docker CLI -----> gateway ---- /api/* --> Go control plane
                         |----- /token --> scoped token endpoint
                         `----- /v2/* ----> CNCF Distribution --> S3 / MinIO

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
- `backend/migrations`: PostgreSQL schema migrations
- `frontend/app`: Next.js routes and layouts
- `frontend/features`: product feature code
- `frontend/lib`: shared typed API client and utilities
- `frontend/providers`: application-wide client providers
- `deployments/compose`: local infrastructure

## Module rules

1. Modules own their domain behavior and persistence contracts.
2. HTTP handlers and database adapters depend on modules, not the reverse.
3. Cross-module behavior goes through explicit application services.
4. Distribution remains the source of blob and manifest transport behavior.
5. HubCR remains the source of users, namespaces, visibility, authorization, and
   security policy.
6. Scan and signature results are keyed by immutable artifact digest.

## Deferred product decisions

The scaffold deliberately does not choose registration policy, organization roles,
repository-level grant inheritance, pull-blocking policy, signature trust roots, or
the final deployment target. These decisions affect schema and API contracts and
must be confirmed before their modules are implemented.
