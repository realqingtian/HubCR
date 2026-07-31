# HubCR

**English** | [简体中文](README.zh-CN.md)

**HubCR — Open-source OCI registry for teams and organizations.**

HubCR is a container image hub for individual developers and organizations. It
provides the business control plane around an OCI registry: identities, namespaces,
repositories, access control, metadata, and software supply-chain security. Mature
OCI upload and download behavior is delegated to CNCF Distribution instead of being
reimplemented.

> [!IMPORTANT]
> HubCR is in early development. The repository contains a runnable project scaffold,
> health endpoints, and local infrastructure definitions. Authentication, repository
> workflows, registry token issuance, and security features are not implemented yet.
> Do not use the current version as a production registry.

## What HubCR is for

HubCR is designed to provide a Docker Hub-style experience while keeping the product
model and security policy under the project's control:

- personal and organization namespaces;
- public and private repositories;
- OCI image push, pull, tag, and deletion through CNCF Distribution;
- repository-scoped authorization for Docker and other OCI clients;
- organization membership and repository-level access control;
- artifact, manifest, tag, and digest metadata;
- asynchronous vulnerability scanning with Trivy;
- signature and attestation discovery and verification with Cosign;
- future robot accounts, access tokens, quotas, audit logs, webhooks, replication,
  proxy caching, and garbage collection.

Image names use a unified namespace model:

```text
hubcr.io/{namespace}/{repository}:{tag}
```

Examples:

```bash
docker pull hubcr.io/sunny/example:latest
docker push hubcr.io/my-organization/backend:v1.0.0
```

## Project status

| Area | Current status |
| --- | --- |
| Go control plane | Runnable scaffold with configuration, graceful shutdown, and health endpoints |
| Asynchronous worker | Runnable polling scaffold; job persistence is not connected |
| Web application | Next.js scaffold with typed API utilities and query provider |
| OCI data plane | Local CNCF Distribution configuration backed by MinIO |
| PostgreSQL and Redis | Local Compose services defined; application adapters are not connected |
| Users, organizations, and repositories | Module boundaries reserved; domain behavior is pending |
| Registry token service | Architecture reserved; token issuance is pending |
| Trivy and Cosign | Worker boundaries reserved; integrations are pending |

## Architecture

HubCR separates the business control plane from the OCI data plane and keeps
long-running security work outside the request path.

```mermaid
flowchart LR
    Client["Browser / Docker CLI"] --> Gateway["Gateway"]
    Gateway -->|"/"| Web["Next.js web app"]
    Gateway -->|"/api/*"| API["Go control plane"]
    Gateway -->|"/token"| Token["Scoped token endpoint"]
    Gateway -->|"/v2/*"| Registry["CNCF Distribution"]

    API --> PostgreSQL["PostgreSQL"]
    API --> Redis["Redis"]
    API --> Jobs["PostgreSQL job table"]
    Registry --> Storage["S3 / MinIO"]
    Registry --> Jobs
    Jobs --> Worker["Go worker"]
    Worker --> Trivy["Trivy"]
    Worker --> Cosign["Cosign"]
```

### Component responsibilities

- **Next.js web application:** public pages, authenticated product UI, and typed API
  consumption.
- **Go control plane:** users, sessions, organizations, namespaces, repository
  metadata, authorization decisions, and REST APIs.
- **Registry token endpoint:** short-lived tokens containing an exact repository and
  action scope such as `pull`, `push`, or `delete`.
- **CNCF Distribution:** OCI `/v2/` protocol, manifests, blobs, uploads, downloads,
  and storage-driver integration.
- **Worker:** asynchronous scan, SBOM, signature, verification, and maintenance jobs.
- **PostgreSQL:** durable business records and the initial job queue.
- **Redis:** cache, rate-limit state, and short-lived coordination data.
- **S3 / MinIO:** OCI blob and manifest storage managed through Distribution.

See the [architecture document](docs/architecture.md) for repository boundaries and
module rules.

## Repository structure

```text
HubCR/
├── backend/
│   ├── AGENTS.md                Backend-only AI instructions
│   ├── cmd/api/                 Control-plane process
│   ├── cmd/worker/              Asynchronous worker process
│   ├── internal/app/            Application composition
│   ├── internal/modules/        Business capability boundaries
│   ├── internal/platform/       Configuration and infrastructure adapters
│   └── migrations/              PostgreSQL migrations
├── frontend/
│   ├── AGENTS.md                Frontend-only AI instructions
│   ├── app/                     Next.js routes and layouts
│   ├── features/                Product feature modules
│   ├── lib/api/                 Typed API client and Zod schemas
│   └── providers/               Application-wide client providers
├── deployments/compose/         Local development infrastructure
├── docs/                        Architecture and development documentation
├── AGENTS.md                    Primary repository-wide AI instructions
├── .env.example                 Local configuration template
└── Makefile                     Common development commands
```

## Technology stack

| Layer | Technology |
| --- | --- |
| Control plane and worker | Go 1.26, standard library HTTP server |
| Web application | Next.js 16, React 19, TypeScript |
| UI and client data | Tailwind CSS, TanStack Query, Zod |
| Primary database | PostgreSQL |
| Cache and coordination | Redis |
| OCI registry | CNCF Distribution |
| Object storage | S3-compatible storage; MinIO for local development |
| Vulnerability scanning | Trivy, planned asynchronous integration |
| Signatures and attestations | Cosign, planned asynchronous integration |
| Local orchestration | Docker Compose |
| Tests and validation | Go test, Go vet, Vitest, ESLint, TypeScript, Next.js build |

## Getting started

### Requirements

- Go 1.26 or newer;
- Bun 1.3 or newer;
- Docker with Compose support for the local infrastructure stack.

### 1. Clone and configure

```bash
git clone git@github.com:realqingtian/HubCR.git
cd HubCR
cp .env.example .env
```

The values in `.env.example` are development-only defaults. Never reuse them in a
shared or production environment.

### 2. Install web dependencies

```bash
cd frontend
bun install
cd ..
```

### 3. Start local infrastructure

```bash
docker compose --env-file .env -f deployments/compose/compose.yaml up -d
```

This starts PostgreSQL, Redis, MinIO, and CNCF Distribution. See the
[local infrastructure guide](deployments/compose/README.md) for ports and details.

### 4. Start the applications

Run each process in a separate terminal:

```bash
make dev-api
```

```bash
make dev-worker
```

```bash
make dev-web
```

Default local endpoints:

| Service | URL |
| --- | --- |
| Web application | `http://localhost:3000` |
| Go control plane | `http://localhost:8080` |
| OCI Distribution | `http://localhost:5000` |
| MinIO API | `http://localhost:9000` |
| MinIO console | `http://localhost:9001` |

### 5. Verify the control plane

```bash
curl --fail http://localhost:8080/api/v1/health/live
curl --fail http://localhost:8080/api/v1/health/ready
```

Both endpoints currently return:

```json
{"status":"ok"}
```

Stop the local infrastructure with:

```bash
docker compose --env-file .env -f deployments/compose/compose.yaml down
```

## Development commands

| Command | Purpose |
| --- | --- |
| `make dev-api` | Run the Go control plane |
| `make dev-worker` | Run the asynchronous worker |
| `make dev-web` | Run the Next.js development server |
| `make test` | Run Go and frontend unit tests |
| `make check` | Run formatting checks, vet, tests, type checking, lint, and production build |

Run `make check` before requesting review or committing a completed change.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `HUBCR_API_ADDRESS` | `:8080` | Control-plane listen address |
| `HUBCR_SHUTDOWN_TIMEOUT` | `10s` | Graceful HTTP shutdown timeout |
| `HUBCR_WORKER_POLL_INTERVAL` | `5s` | Worker polling interval |
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:8080` | Browser-visible control-plane base URL |
| `POSTGRES_DB` | `hubcr` | Local PostgreSQL database |
| `POSTGRES_USER` | `hubcr` | Local PostgreSQL user |
| `POSTGRES_PASSWORD` | development only | Local PostgreSQL password |
| `MINIO_ROOT_USER` | `hubcr` | Local MinIO administrator |
| `MINIO_ROOT_PASSWORD` | development only | Local MinIO password |

## Core security and data rules

- Repository access is authorized for every request, even when blobs are physically
  deduplicated by digest.
- Web sessions are not long-lived Registry credentials.
- Registry tokens must be short-lived and limited to an exact repository and action
  scope.
- Scan reports, SBOMs, and signature verification results are bound to immutable
  artifact digests, never only to mutable tags.
- Finding a signature is not the same as trusting it. Trust depends on successful
  verification against a versioned policy.
- Scanning and verification are asynchronous and must not block a successful image
  push unless an explicit repository policy requires it.
- Secrets and real credentials must never be committed. The repository only contains
  clearly marked local development defaults.

## Documentation

English is the primary documentation language. Every user-facing project document
must link to and remain synchronized with its Simplified Chinese counterpart.

| Document | English | 简体中文 |
| --- | --- | --- |
| Project overview | [README](README.md) | [README](README.zh-CN.md) |
| Product requirements | [Requirements](docs/requirements.md) | [产品需求](docs/requirements.zh-CN.md) |
| Executable development plan | [Development plan](docs/development-plan.md) | [开发计划](docs/development-plan.zh-CN.md) |
| Architecture | [Architecture](docs/architecture.md) | [架构](docs/architecture.zh-CN.md) |
| Development standards | [Development](docs/development.md) | [开发规范](docs/development.zh-CN.md) |
| AI instruction hierarchy | [Instructions](AGENTS.md) | [AI 指令](AGENTS.zh-CN.md) |
| Local infrastructure | [Compose](deployments/compose/README.md) | [本地基础设施](deployments/compose/README.zh-CN.md) |
| Web application | [Web](frontend/README.md) | [Web 应用](frontend/README.zh-CN.md) |

## Roadmap

1. **Registry MVP:** users, sessions, namespaces, organizations, repositories,
   visibility, scoped tokens, push/pull, tags, and artifact metadata.
2. **Supply-chain security:** Trivy scans, SBOMs, Cosign discovery and verification,
   trust policies, and security status pages.
3. **Operations:** robot accounts, access tokens, quotas, audit logs, webhooks,
   deletion, garbage collection, replication, and proxy caching.
4. **Public service readiness:** email verification, password recovery, MFA, abuse
   controls, billing, high availability, and multi-region delivery.

## Contributing

Read the [development standards](docs/development.md), the primary
[AI instructions](AGENTS.md), and the nearest directory-specific `AGENTS.md` before
making changes. Keep changes scoped, include tests for behavior, update both
documentation languages, and run `make check` before submitting work.

## License status

HubCR is intended to be an open-source project, but a license file has not been added
yet. A license must be selected before the first public release.
