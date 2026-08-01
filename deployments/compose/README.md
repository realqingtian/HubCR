# Local infrastructure

**English** | [简体中文](README.zh-CN.md)

This Compose stack starts PostgreSQL, Redis, MinIO, CNCF Distribution, and the local
gateway. Distribution requires scoped Bearer tokens: the gateway routes `/v2/` to
Distribution and `/token` to the separately running Go control plane. The API,
worker, and web application stay outside Compose so they can use native hot reload.

The Registry host port defaults to `5000` and can be changed through
`HUBCR_REGISTRY_PORT`. On macOS, AirPlay Receiver may reserve port `5000` through the
`ControlCenter` process. Keep AirPlay running and select another local port when that
happens:

```bash
HUBCR_REGISTRY_PORT=5001 HUBCR_ENV_FILE=.env.example make infra-up
```

From the repository root, use the Make workflow. `infra-up` generates or validates an
ignored local RSA private key and JWKS before mounting only the trust material into
Distribution. The targets default to `.env`; use `.env.example` only for the
documented local defaults:

```bash
HUBCR_ENV_FILE=.env.example make infra-config
HUBCR_ENV_FILE=.env.example make infra-up
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
HUBCR_ENV_FILE=.env.example make infra-down
```

Start `make dev-api` in a separate terminal before requesting tokens. It enables the
local-only HTTP token endpoint and reuses the same ignored signing material. Direct
API startup remains fail-closed unless all Registry auth settings are supplied.
The API and Distribution notification endpoint must also receive the same
`HUBCR_REGISTRY_EVENT_TOKEN`; the documented Make workflow supplies the explicit
development-only default from `.env.example`.

When overriding the Registry port, pass the same value to `infra-up`, `dev-api`, and
`infra-smoke`, for example `HUBCR_REGISTRY_PORT=5001`.

The local endpoints are:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO S3 API: `http://localhost:9000`
- MinIO console: `http://localhost:9001`
- OCI gateway (`/v2/` and `/token`): `http://localhost:5000`
- Go control plane: `http://localhost:8080`

Use the configured `HUBCR_REGISTRY_PORT` instead of `5000` when it is overridden.

Registry token authentication and authenticated Distribution push-event delivery are
enabled in the local Make workflow. Manifest and Index events reconcile Artifact and
current Tag metadata in PostgreSQL. Pull, delete, and mount events are filtered, and
Distribution deletion remains disabled until lifecycle policy is approved.

## Smoke checks

Run these checks from the repository root after the stack starts:

```bash
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
```

The expected results are healthy infrastructure, PostgreSQL `accepting connections`,
Redis `PONG`, MinIO `200`, and Registry `401` with an exact scoped Bearer challenge
whose realm is `http://localhost:5000/token`. A `401` capability response is correct;
an unauthenticated `200` would mean Registry authorization was bypassed.

The isolated end-to-end target creates only test users and repositories through GORM,
starts a real API, pushes and pulls a small image through Docker, and removes its
containers, volumes, image tags, and temporary Docker credential file afterward:

```bash
make test-m2-registry-e2e
```

It verifies owner push, anonymous public pull, private denial, reader pull without
push, wrong-organization denial, invalid credentials, cross-repository and
cross-action token isolation, runtime rejection of expired and invalid-signature
tokens, event-derived Artifact/Tag metadata, and authorized Artifact API readback. The
metadata assertions cover repository-scoped identity, Manifest/Index descriptors, Tag
state, denied-push non-persistence, private `404` non-disclosure, and authenticated
public access. The target uses dedicated ports and a separate Compose project, and
never reads or writes the user's Docker credential store or macOS Keychain.

## Stop and local data

The normal stop command removes project containers and the network but preserves the
named PostgreSQL, Redis, and MinIO volumes:

```bash
HUBCR_ENV_FILE=.env.example make infra-down
```

The following command is destructive and deletes all local HubCR infrastructure data.
Run it only when a clean local database, cache, and object store are explicitly
required:

```bash
docker compose --env-file .env.example -f deployments/compose/compose.yaml down --volumes
```

## Verified environment

The authenticated Registry path was verified on 2026-08-01 with Docker Engine
`29.6.2`, Docker Compose `v5.3.1`, a `linux/arm64` Docker server, PostgreSQL 17,
Registry 3, and Nginx 1.29 on Apple Silicon. The automated matrix used isolated host
port `55001` and `alpine:3.22`; authorization, cross-repository and cross-action scope
isolation, runtime token expiry and signature rejection, event-driven Artifact/Tag,
and Artifact API checks passed.

On macOS, `ControlCenter` may reserve port `5000`; use a consistent alternate
`HUBCR_REGISTRY_PORT` for `infra-up`, `dev-api`, and `infra-smoke`. This is a host-port
conflict, not an OCI or Apple Silicon compatibility failure.
