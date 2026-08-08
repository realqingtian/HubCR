# Local infrastructure and single-host deployment

**English** | [简体中文](README.zh-CN.md)

This Compose stack starts PostgreSQL, Redis, MinIO, CNCF Distribution, and the local
gateway. Distribution requires scoped Bearer tokens: the gateway routes `/v2/` to
Distribution and `/token` to the separately running Go control plane. In the default
development workflow, the API, worker, and Web application stay outside Compose so
they can use native hot reload. The production override adds those services.

The Registry host port defaults to `5000` and can be changed through
`HUBCR_REGISTRY_PORT`. On macOS, AirPlay Receiver may reserve port `5000` through the
`ControlCenter` process. Keep AirPlay running and select another local port when that
happens:

```bash
HUBCR_REGISTRY_PORT=5001 HUBCR_ENV_FILE=.env.example make infra-up
```

From the repository root, use the Make workflow. `infra-up` generates or validates an
ignored local RSA private key, JWKS, and random event token before mounting only the
trust material into Distribution. All published development ports bind to
`127.0.0.1`. The targets default to `.env`; use `.env.example` only for the documented
local defaults:

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
`HUBCR_REGISTRY_EVENT_TOKEN`; the documented Make workflow reads the generated value
from ignored `.data/registry-auth/event-token`. Shared deployments must inject and
rotate an independent secret.

When overriding the Registry port, pass the same value to `infra-up`, `dev-api`, and
`infra-smoke`, for example `HUBCR_REGISTRY_PORT=5001`. Distribution's localhost-only
debug listener separately defaults to `HUBCR_REGISTRY_DEBUG_PORT=5002`.

The local endpoints are:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO S3 API: `http://localhost:9000`
- MinIO console: `http://localhost:9001`
- OCI gateway (`/v2/` and `/token`): `http://localhost:5000`
- Distribution operations (`/metrics` and `/debug/vars`): `http://127.0.0.1:5002`
- Go control plane: `http://localhost:8080`

Use the configured `HUBCR_REGISTRY_PORT` instead of `5000` when it is overridden.

## Supported single-host Compose deployment

The accepted MVP target combines this base file with
`compose.production.yaml`. It builds the Go API/worker/migration image and the
standalone Next.js image, removes every infrastructure host port, and publishes only
the gateway on `127.0.0.1` by default. A trusted operator-managed HTTPS reverse proxy
must forward the public origin to that listener.

Copy `.env.production.example` to ignored `.env.production`, replace every required
blank, provide the separately protected Registry keys and event secret, then run:

```bash
make prod-config
make prod-build
make prod-up
make prod-status
```

The worker uses the same required `HUBCR_DATABASE_URL` as the API and starts only
after PostgreSQL is healthy. Its lease must exceed its per-attempt timeout; polling,
lease, timeout, shutdown, retry, and concurrency bounds are explicit environment
settings in the example file. The production worker includes digest-pinned Trivy
0.72.0 and Cosign 3.0.6, uses dedicated non-authoritative cache/scratch paths, mounts
the Registry signing directory read-only to mint short-lived exact-repository Pull
tokens, and periodically repairs missing Artifact security workflows. Production startup still requires `make prod-migrate`
through `make prod-up` before either application process uses a new schema.

Production startup never creates signing keys or secrets. Images retain readable
tags but are pinned to immutable Manifest Digests. The complete deployment,
maintenance-window backup, destructive restore, migration, upgrade, and recovery
acceptance workflow is in the [MVP operator guide](../../docs/operator-guide.md).

Registry token authentication and authenticated Distribution push-event delivery are
enabled in the local Make workflow. Manifest and Index events reconcile Artifact and
current Tag metadata in PostgreSQL. Pull, delete, and mount events are filtered, and
Distribution deletion remains disabled until lifecycle policy is approved.

The gateway emits secret-safe JSON access logs and marks Registry `401` responses
with `registry_challenge=true`. Distribution uses JSON application logs and exposes
Prometheus plus notification queue state only through the loopback debug port. With
Registry auth enabled, the direct Go listener exposes bounded token and notification
counters at `GET /internal/metrics`; that endpoint is intentionally not routed by the
gateway. See [Registry operational observability](../../docs/registry-observability.md).

## Smoke checks

Run these checks from the repository root after the stack starts:

```bash
HUBCR_ENV_FILE=.env.example make infra-status
HUBCR_ENV_FILE=.env.example make infra-smoke
```

The expected results are healthy infrastructure, PostgreSQL `accepting connections`,
Redis `PONG`, MinIO `200`, Registry `401` with an exact scoped Bearer challenge whose
realm is `http://localhost:5000/token`, and reachable localhost-only Distribution
metrics and notification variables. A `401` capability response is correct; an
unauthenticated `200` would mean Registry authorization was bypassed.

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
It also verifies correlated token/notification logs, policy action counters, a
structured challenge log, Distribution metrics and queue visibility, and the absence
of tested credentials and Bearer tokens from gateway and API logs.

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
The same isolated runtime now also passes challenge/token/notification telemetry,
Distribution queue visibility, bounded-metric, and secret-safe log assertions; the
Distribution debug listener used host port `55002`.

On 2026-08-09, `make test-m4-security-e2e` built the same production topology, pushed
vulnerable and clean fixtures, persisted four jobs with the worker stopped, exercised
a Registry-outage retry, and verified scan/SBOM evidence, trusted, untrusted, invalid,
attested, and unsigned signatures, two policy versions, authorized API output, and
Trivy/Cosign versions. Distribution runs at warning log level so its startup output
cannot echo the configured notification Authorization header.

On 2026-08-08, `make test-m3-backup-restore-e2e` additionally built the complete
single-host production topology, pushed private and public images, stopped write
services, created a PostgreSQL plus Registry-object backup, deleted only isolated
test volumes, rotated separately protected Registry signing material, restored and
migrated the data, and verified login, private Pull, Artifact/Tag state, and an
unchanged Digest.

On macOS, `ControlCenter` may reserve port `5000`; use a consistent alternate
`HUBCR_REGISTRY_PORT` for `infra-up`, `dev-api`, and `infra-smoke`. This is a host-port
conflict, not an OCI or Apple Silicon compatibility failure.
