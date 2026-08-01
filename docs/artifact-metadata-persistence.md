# Artifact metadata persistence

**English** | [简体中文](artifact-metadata-persistence.zh-CN.md)

- Status: `APPROVED`
- Approved: 2026-08-01
- Work package: M2-05
- Requirements: FR-ART-001 through FR-ART-005
- Implementation: M2-05 completed and accepted on 2026-08-01

This document defines the persistence contract consumed by M2-06 event reconciliation
and M2-07 Artifact APIs. Both capabilities are implemented in separate adapters over
this shared domain boundary.

## Decisions and boundaries

- Artifact identity is `(repository_id, digest)`, even when Distribution physically
  deduplicates content.
- M2 accepts canonical SHA-256 Digests only:
  `sha256:<64 lowercase hexadecimal characters>`.
- A Tag stores only its current Artifact reference. Tag move/delete history is not
  stored in M2-05.
- Moving or removing a Tag never deletes its former Artifact. Untagged Artifacts stay
  stored until a separately approved retention and garbage-collection policy exists.
- An Index stores an ordered, immutable set of child Manifest descriptors. Missing
  platform metadata remains absent.
- M2-05 owns domain validation, GORM/Gormigrate Schema, atomic persistence, and
  repository-scoped reads. Distribution push-event ingestion is implemented by
  M2-06, while authorized HTTP reads are implemented by M2-07.

## Module ownership

`backend/internal/modules/artifacts` owns value validation, domain entities, stable
errors, reconciliation orchestration, and the Store interface.

`backend/internal/platform/postgres/artifactstore` owns GORM records, PostgreSQL
transactions and locking, persistence reads, and database-error classification. It
does not authorize requests or interpret Distribution events.

`backend/migrations` owns forward migration `000006_artifact_metadata`.

## Schema

### `artifacts`

Each row has an opaque UUID, Repository ID, Digest, immutable `MANIFEST` or `INDEX`
kind, nullable Media Type/Size/source-created time, descriptor-completion state, and
UTC discovery/update times.

Durable invariants include:

- unique `(repository_id, digest)`;
- canonical SHA-256 Digest Check;
- non-negative Size;
- `updated_at >= discovered_at`;
- only an Index may have a completed descriptor set;
- repository deletion and Artifact deletion use `RESTRICT` rather than implicit
  metadata cleanup.

### `tags`

The composite identity is `(repository_id, name)`. Names are case-sensitive, at most
128 bytes, and match `[A-Za-z0-9_][A-Za-z0-9._-]{0,127}`. A composite foreign key
ensures the current Artifact belongs to the same Repository.

### `manifest_descriptors`

Rows store Repository ID, Index Artifact ID, zero-based position, child Manifest
Artifact ID, and nullable OS/Architecture/Variant. Composite foreign keys enforce that
parent and child belong to the same Repository. Checks reject self-reference and
partial Platform values.

`descriptors_complete` distinguishes an unknown descriptor set from a confirmed empty
set. Once complete, a Digest's ordered descriptor set is immutable.

## Atomic reconciliation

`ReconcileArtifact` runs one transaction:

1. Insert-or-load and lock the repository-scoped parent Artifact.
2. Require immutable Kind to match.
3. Enrich nullable metadata; different non-null facts for the same Digest conflict.
4. Reconcile child Manifest rows when a complete Index descriptor set is supplied.
5. Insert the complete descriptor set once, or compare an existing set exactly.
6. Optionally create or move the current Tag in the same transaction.
7. Return the durable winning state after concurrent conflict handling.

An exact replay does not create rows or change timestamps. Any contradiction returns
`ErrConflict` and rolls back every change. `RemoveTag` is idempotent and never removes
Artifact or descriptor rows.

## Reads and errors

Reads always require Repository ID and support Artifact-by-Digest, Tag-by-name,
Digest-ordered Artifact pagination, name-ordered Tag pagination, and ordered Index
descriptors. Limits are 1 through 100 and cursors must be valid domain values.

Stable errors are `ErrInvalidDigest`, `ErrInvalidTag`, `ErrInvalidArtifact`,
`ErrConflict`, `ErrNotFound`, and `ErrUnavailable`. SQL and schema details never cross
the Store boundary.

## Acceptance evidence

M2-05 tests must cover validation, empty/upgrade/repeat migrations, database
constraints, exact replay, metadata enrichment and conflict rollback, Tag movement
and removal, untagged Artifact retention, descriptor immutability, cross-repository
foreign keys, bounded pagination, and concurrent idempotency with real isolated
PostgreSQL.

Completion requires:

```bash
go -C backend test ./internal/modules/artifacts
go -C backend test ./internal/platform/postgres/artifactstore
make test-integration
make check
git diff --check
```

The M2-05 work package itself excludes Distribution notification handling, HTTP APIs,
Tag audit history, Artifact deletion/retention/GC, security jobs/results, and frontend
views. M2-06 now provides notification handling and M2-07 provides read-only HTTP
APIs without changing the remaining boundaries.
