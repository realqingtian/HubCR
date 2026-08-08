# HubCR MVP operator guide

**English** | [简体中文](operator-guide.zh-CN.md)

This guide operates the [accepted single-host Docker Compose deployment](decisions/d-009-production-deployment.md).
It is an MVP recovery contract, not a high-availability design. Read the
[release limitations](release-limitations.md) before exposing HubCR to users.

## Supported topology

The production Compose model runs one API replica, one PostgreSQL-backed worker, one Web
application, the gateway, CNCF Distribution, PostgreSQL, Redis, and MinIO on one
host. Only the gateway binds a host port, and it binds `127.0.0.1` by default.

A trusted HTTPS reverse proxy is required in front of that listener. It owns the
public certificate and DNS name and forwards the complete origin, including `/`,
`/api/`, `/token`, and `/v2/`. PostgreSQL, Redis, MinIO, API, Web, and Distribution
operations endpoints must remain private to the Compose network.

## Prepare configuration and secrets

Requirements are Docker Engine with Compose, sufficient durable disk space, an HTTPS
reverse proxy, and an absolute host directory for Registry signing material.

1. Copy `.env.production.example` to ignored `.env.production` and restrict it to the
   deployment account.
2. Replace every blank required value. `HUBCR_DATABASE_URL` uses the Compose hostname
   `postgres`; URL-encode reserved password characters.
3. Set `HUBCR_REGISTRY_EXTERNAL_URL` to the exact public HTTPS origin. It must not
   contain a path, query, fragment, or credentials.
4. Put `private.pem` and `jwks.json` in the absolute
   `HUBCR_REGISTRY_AUTH_DIR`. Supply an independent event callback secret of at least
   32 visible ASCII characters as `HUBCR_REGISTRY_EVENT_TOKEN` through the protected
   operator environment.
5. Keep `HUBCR_SESSION_COOKIE_SECURE=true`. The insecure override exists only for the
   isolated HTTP acceptance runner.

The production workflow never generates keys or secrets. The private key, JWKS
rotation set, event token, environment file, reverse-proxy configuration, and TLS
keys are excluded from ordinary HubCR data backups and require a separate protected
recovery procedure.

## Validate, build, and start

Run from the repository root:

```bash
make prod-config
make prod-build
make prod-up
make prod-status
```

`prod-up` starts the durable dependencies, applies migrations under the PostgreSQL
advisory lock, then waits for the API, Web, Registry, worker, and gateway. The
production images and infrastructure images are pinned by immutable image digest.

The worker image contains Trivy 0.72.0 and Cosign 3.0.6; production enables scan,
SBOM, signature-verification, and trust-evaluation handlers. Dedicated scratch and
cache paths remain non-authoritative. The worker mounts the Registry signing directory
read-only so it can mint short-lived tokens limited to `pull` on the exact Artifact
repository; protect that container and key mount as part of the Registry signing
boundary. Disabling a tool leaves its intents queued for a later enabled worker rather
than inventing successful evidence.

After the external reverse proxy is configured, verify the public HTTPS origin:

```bash
curl --fail https://registry.example.com/api/v1/health/live
curl --fail https://registry.example.com/api/v1/health/ready
curl --silent --dump-header - --output /dev/null https://registry.example.com/v2/
```

Liveness and readiness should return `200`. `/v2/` should return `401` with a Bearer
challenge whose Realm is the configured external origin plus `/token`.

## Stop and inspect

```bash
make prod-status
make prod-down
```

`prod-down` removes containers and the network while preserving named PostgreSQL,
Redis, and MinIO volumes. Never use `down --volumes` on a deployment unless data loss
is explicitly intended or an isolated recovery rehearsal is resetting its own test
project.

## Create a data backup

The accepted backup is an offline-consistent manual operation. Announce a maintenance
window and prevent all business and OCI writes before proceeding.

```bash
make prod-maintenance-stop
HUBCR_BACKUP_DIR=/secure/off-host/hubcr-2026-08-08 \
HUBCR_BACKUP_MAINTENANCE_CONFIRMED=true \
make prod-backup
make prod-up
```

The command refuses to run if API, worker, Web, gateway, or Registry containers are
still running, refuses to overwrite an existing destination, creates owner-only
files, writes a PostgreSQL custom dump, mirrors the Distribution bucket, and records
SHA-256 checksums.

The output contains password hashes, session records, private metadata, scan/SBOM
evidence, job state, and OCI content. Encrypt it, transfer it off the deployment host,
restrict access, and test the actual encrypted copy. Redis, the rebuildable Trivy
cache, Registry keys, event tokens, environment files, TLS keys, and reverse-proxy
configuration are not included.

## Restore and migrate

Restore replaces the target PostgreSQL schema and the complete Registry object
bucket. Use an empty recovery host or a maintenance window, and verify that the
separately protected keys, secrets, DNS, and TLS material are available.

Start only the durable dependencies:

```bash
scripts/production-compose.sh up --detach --wait postgres redis minio minio-init
```

Then restore, migrate, and start the application:

```bash
HUBCR_BACKUP_DIR=/secure/off-host/hubcr-2026-08-08 \
HUBCR_RESTORE_CONFIRM=restore \
make prod-restore
make prod-up
```

Restore rejects an unsupported bundle, a checksum mismatch, a symbolic-link bundle,
or running application/write services. It applies all current migrations after data
replacement. New Registry signing keys are allowed; previously issued short-lived
Registry tokens then become invalid and clients authenticate again.

## Recovery acceptance checklist

Do not declare recovery successful until all of these are verified:

- current migrations are present and readiness is `200`;
- an existing user can log in and receive the correct personal namespace;
- an authorized user can read a private repository while an outsider cannot;
- Docker can authenticate and pull an image that existed before the backup;
- the restored Tag and Artifact are visible through the API and Web application;
- the immutable Digest exactly matches the pre-backup value;
- Registry keys, event secret, TLS certificate, and environment were restored from
  their separate protected source rather than the data bundle.

Run the repository-owned isolated rehearsal with:

```bash
make test-m3-backup-restore-e2e
```

It builds the production images, Pushes a test image, backs up during a write outage,
deletes only its isolated volumes, rotates Registry signing material, restores,
migrates, logs in, Pulls the private image, and compares the Digest.

Validate the scanner path separately with:

```bash
make test-m4-security-e2e
```

This isolated runner Pushes vulnerable and clean fixtures, persists work while the
worker is stopped, exercises a Registry-outage retry, and verifies scan/SBOM evidence,
trusted, untrusted, invalid, attested, and unsigned signatures, two immutable trust
policy versions, authorized API results, and Trivy/Cosign version evidence.

## Upgrade and rollback boundary

Before upgrading, create and verify a maintenance-window backup. Build the reviewed
revision, stop write services, and run `make prod-up`; migrations are forward-only and
run before application traffic resumes. There is no automatic database downgrade.
Rollback after an incompatible migration means restoring the pre-upgrade data bundle
with the matching reviewed application revision.

Automated schedules, retention, fixed RPO/RTO, cross-region recovery, high
availability, Kubernetes, deletion, and garbage collection are outside the accepted
MVP policy.
