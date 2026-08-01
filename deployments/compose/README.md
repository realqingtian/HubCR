# Local infrastructure

**English** | [简体中文](README.zh-CN.md)

This Compose stack starts PostgreSQL, Redis, MinIO, and CNCF Distribution for local
development. It intentionally does not start the API, worker, or web application so
each process can run with native hot reload.

From `deployments/compose`, start the stack with:

```bash
docker compose --env-file ../../.env.example up -d
```

The Registry host port defaults to `5000` and can be changed through
`HUBCR_REGISTRY_PORT`. On macOS, AirPlay Receiver may reserve port `5000` through the
`ControlCenter` process. Keep AirPlay running and select another local port when that
happens:

```bash
HUBCR_REGISTRY_PORT=5001 docker compose --env-file ../../.env.example up -d
```

The repository Make targets provide the repeatable workflow used by normal local
development. They default to `.env`; use `.env.example` only for the documented local
defaults:

```bash
HUBCR_ENV_FILE=.env.example make infra-config
HUBCR_ENV_FILE=.env.example make infra-up
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
HUBCR_ENV_FILE=.env.example make infra-down
```

When overriding the Registry port, pass the same value to `infra-up` and
`infra-smoke`, for example `HUBCR_REGISTRY_PORT=5001`.

The local endpoints are:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO S3 API: `http://localhost:9000`
- MinIO console: `http://localhost:9001`
- OCI Distribution: `http://localhost:5000`

Use the configured `HUBCR_REGISTRY_PORT` instead of `5000` when it is overridden.

Authentication and event notifications are not enabled yet. They will be connected
after the Registry Token authorization flow is specified.

## Smoke checks

Run these checks from the repository root after the stack starts:

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml ps --all
docker compose --env-file .env.example -f deployments/compose/compose.yaml exec -T postgres pg_isready -U hubcr -d hubcr
docker compose --env-file .env.example -f deployments/compose/compose.yaml exec -T redis redis-cli ping
curl --fail http://localhost:9000/minio/health/live
curl --fail http://localhost:5000/v2/
```

The expected results are a healthy PostgreSQL container, `accepting connections`,
Redis `PONG`, successful MinIO health, a completed `minio-init` container, and a
Registry `200 OK` response with `{}`. Replace `5000` with the configured
`HUBCR_REGISTRY_PORT` when necessary.

The current unauthenticated development Registry can be tested with a small image:

```bash
docker pull alpine:3.22
docker tag alpine:3.22 localhost:5000/hubcr/m0-smoke:local
docker push localhost:5000/hubcr/m0-smoke:local
docker image rm localhost:5000/hubcr/m0-smoke:local
docker pull localhost:5000/hubcr/m0-smoke:local
```

The `docker image rm` command above removes only the local test tag so that the next
command proves the image can be pulled back from Distribution. Use the configured
Registry port in all five commands when it is overridden.

## Stop and local data

The normal stop command removes project containers and the network but preserves the
named PostgreSQL, Redis, and MinIO volumes:

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml down
```

The following command is destructive and deletes all local HubCR infrastructure data.
Run it only when a clean local database, cache, and object store are explicitly
required:

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml down --volumes
```

## Verified environment

The full smoke was verified on 2026-08-01 with Docker Engine `29.6.2`, Docker Compose
`v5.3.1`, and a `linux/arm64` Docker server on Apple Silicon. PostgreSQL became
healthy, Redis returned `PONG`, MinIO created `hubcr-registry`, and Distribution
returned `200 OK`. An `alpine:3.22` image was pushed, its local test tag was removed,
and it was pulled back with Registry digest
`sha256:2c9d26f410d032d5b1525aa8a873e238b05b90c4ae8618743d4311f0cc827e37`.
After a normal `down`, all three named volumes remained and the Registry catalog still
contained the test repository after restart.

On the verified macOS host, `ControlCenter` reserved port `5000`, so the Registry
portion of the smoke used `HUBCR_REGISTRY_PORT=5001`. This is a host-port conflict,
not an OCI or Apple Silicon image-compatibility failure.
