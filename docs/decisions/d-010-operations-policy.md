# D-010 Operations policy

**English** | [简体中文](d-010-operations-policy.zh-CN.md)

- Status: `ACCEPTED` for the MVP backup and restore subset
- Approved: 2026-08-08
- Decision owner: product owner
- Blocks: M3-07 backup, restore, and migration rehearsal

## Context

The broad operations decision includes backup, restore, retention, deletion, garbage
collection, quotas, audit, and disaster recovery. M3-07 needs only a bounded recovery
contract. Selecting unrelated destructive lifecycle policy now would silently expand
the MVP and could create irreversible data behavior without product evidence.

## Decision

The accepted MVP subset is:

1. A data backup contains the PostgreSQL business database and the MinIO
   `hubcr-registry` bucket used by Distribution.
2. Redis is excluded because the selected MVP does not store authoritative business
   state there.
3. Registry signing private keys, JWKS rotation material, event tokens, deployment
   environment, and other secrets are separately protected operator inputs and are
   excluded from the ordinary backup bundle.
4. Backup is a manual maintenance-window operation. Operators stop API, worker, Web,
   gateway, and Registry writes before confirming the backup. The command refuses to
   run while those Compose services are still running.
5. The backup is integrity-manifested and created with owner-only permissions. It
   contains password hashes, session records, repository metadata, and OCI content,
   so operators must encrypt it and store it outside the deployment host.
6. Restore is intentionally destructive, requires an explicit confirmation value,
   verifies every recorded checksum, replaces PostgreSQL and Registry object data,
   and then applies all current database migrations.
7. Registry keys and secrets must be supplied separately before the restored
   application starts. Rotating the signing key during a rehearsal is valid and
   invalidates previously issued short-lived Registry tokens.
8. Recovery acceptance proves database migration, user login, private-repository
   authorization, an existing private image pull, Artifact/Tag availability, and an
   unchanged immutable Digest.

Automated scheduling, cross-region disaster recovery, fixed RPO/RTO values, backup
retention, quotas, repository deletion, and Distribution garbage collection remain
unapproved and deferred.

## Alternatives

- Back up Docker volumes byte-for-byte: rejected because portable logical PostgreSQL
  dumps and S3-level object copies provide clearer version and integrity boundaries.
- Include Registry keys and deployment secrets in the same archive: rejected because
  it couples data recovery to a single high-value secret bundle and increases theft
  impact.
- Allow online backup while pushes and business writes continue: rejected because a
  PostgreSQL snapshot and separately copied object store could represent different
  repository states.
- Define fixed RPO/RTO and retention now: deferred until measured deployment needs and
  storage costs exist.

## Consequences

- The MVP recovery workflow requires downtime and is not a high-availability design.
- A successful local rehearsal is release evidence, not proof of off-host disaster
  recovery; operators must separately test their encrypted storage and TLS/secret
  restoration procedures.
- Later deletion, retention, garbage collection, quota, and audit work still requires
  explicit approval and cannot infer policy from this record.
