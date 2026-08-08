# Distribution event reconciliation

**English** | [简体中文](distribution-event-reconciliation.zh-CN.md)

- Status: `IMPLEMENTED`
- Implemented: 2026-08-01
- Work package: M2-06
- Requirements: FR-ART-001 and FR-ART-004
- Dependency: [M2-05 Artifact metadata persistence](artifact-metadata-persistence.md)

This document records the implemented contract between CNCF Distribution and the
HubCR control plane. Distribution remains the OCI data plane; the Go API consumes
authenticated push notifications only to reconcile repository-scoped Artifact and
current Tag metadata.

The implementation follows the official [Distribution notification
contract](https://distribution.github.io/distribution/about/notifications/) and
[notification endpoint configuration](https://distribution.github.io/distribution/about/configuration/).
The accepted local runtime is Distribution 3.1.1.

## Scope and boundaries

- Distribution sends `push` events to `POST /internal/registry/events` with content
  type `application/vnd.docker.distribution.events.v2+json`.
- The endpoint processes OCI and Docker v2 Manifest and Index/Manifest List media
  types. Blob events and unsupported media types are accepted but ignored.
- Local Distribution configuration filters `pull`, `delete`, and `mount` actions.
  Deletion remains disabled until retention, deletion, and garbage-collection policy
  is approved.
- `events.includereferences` is enabled so Index events can carry their ordered child
  Manifest descriptors and Platform metadata.
- An omitted or empty `references` field remains an unknown descriptor set because
  the Distribution payload cannot distinguish an omitted empty set from an
  explicitly confirmed empty set.
- This work package does not expose Artifact/Tag HTTP APIs, retain Tag history, create
  deletion tombstones, or change OCI upload/download behavior.

## Authentication and request limits

Distribution and the API share `HUBCR_REGISTRY_EVENT_TOKEN`. The value is required
whenever Registry authentication is enabled, must contain 32–512 visible ASCII
characters, and is sent as exactly one `Authorization: Bearer <token>` header. The
handler compares a SHA-256 digest of the configured secret in constant time and never
logs the secret, authorization header, or notification payload.

No accepted token is checked in. `make registry-dev-keys` generates an ignored local
token with mode `0600`, and the Make workflow injects it into the API and Distribution.
Shared deployments must provide and rotate their own independent secret.

The handler accepts at most 1 MiB and 100 events per request. It rejects malformed
JSON, trailing JSON values, missing or duplicate authorization headers, unsupported
methods, and the wrong media type. Unknown JSON fields are accepted for protocol
forward compatibility.

## Reconciliation and retry behavior

Each relevant event resolves the exact `namespace/repository`, validates the Digest,
Tag, media type, size, timestamp, and descriptors, then calls the M2-05 atomic GORM
reconciliation service. Repository identity remains part of Artifact identity even
when Distribution deduplicates the physical content by Digest.

Distribution notification delivery is at-least-once and ordering is not guaranteed.
Exact event replay is therefore idempotent. A newer Tag observation may move the
current Tag, while an older or equal-timestamp event cannot move it away from a newer
persisted mapping. Untagged former Artifacts remain stored.

Responses intentionally drive Distribution retry behavior:

| Result | Status | Retry meaning |
| --- | --- | --- |
| Accepted or intentionally ignored | `202` | Delivery is complete |
| Invalid request or event | `400` | Permanent payload failure |
| Contradictory immutable metadata | `409` | Permanent reconciliation conflict |
| Repository lookup or persistence unavailable | `503` | Retryable dependency failure |
| Missing or invalid event token | `401` | Configuration/authentication failure |
| Wrong media type / oversized body | `415` / `413` | Permanent request failure |

Distribution uses a bounded local endpoint queue with a two-second timeout,
five-failure threshold, and one-second backoff. M2-09 now exposes the queue and retry
state together with correlated control-plane counters and logs as documented in
[Registry operational observability](registry-observability.md).

## Acceptance evidence

Focused tests cover event mapping, authentication, request limits, error
classification, duplicate delivery, Tag movement, stale-event protection, Index
references, repository isolation, and dependency failure. The real Docker/OCI suite
pushes public and private images through token-protected Distribution and verifies the
resulting PostgreSQL Artifact/Tag state, including repository-scoped identity and
denied-push non-persistence.

```bash
go -C backend test ./internal/modules/registry ./internal/platform/httpapi/registryeventhandler
make test-integration
make test-m2-registry-e2e
HUBCR_ENV_FILE=.env.example make infra-config
make check
git diff --check
```
