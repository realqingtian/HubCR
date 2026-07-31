# HubCR

**HubCR — Open-source OCI registry for teams and organizations.**

面向个人与组织的开源可信 OCI 镜像中心。

HubCR uses a Go business control plane and reuses CNCF Distribution as its OCI data
plane. The repository currently contains the executable project scaffold; domain
features will be implemented after the MVP policies are confirmed.

## Project structure

```text
HubCR/
├── backend/              Go control plane and asynchronous worker
├── frontend/             Next.js web application
├── deployments/compose/  Local PostgreSQL, Redis, MinIO, and Distribution
├── docs/                 Architecture and future design decisions
├── .env.example          Local configuration defaults
└── Makefile              Common development commands
```

See [docs/architecture.md](docs/architecture.md) for module boundaries.

## Requirements

- Go 1.26 or newer
- Bun 1.3 or newer
- Docker with Compose support for local infrastructure

## Get started

Install the frontend dependencies if needed:

```bash
cd frontend
bun install
```

Start the control plane:

```bash
make dev-api
```

The health endpoints are:

```text
GET http://localhost:8080/api/v1/health/live
GET http://localhost:8080/api/v1/health/ready
```

In separate terminals, start the worker and web application:

```bash
make dev-worker
make dev-web
```

Run all checks:

```bash
make check
```

Local infrastructure instructions are in
[deployments/compose/README.md](deployments/compose/README.md).
