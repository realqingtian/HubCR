# HubCR Registry MVP release limitations

**English** | [简体中文](release-limitations.zh-CN.md)

HubCR is an evidence-backed Registry MVP release candidate, not a general-purpose
production service. These limitations are part of the release contract.

## Supported evidence

- One API replica on a single Docker Compose host behind an operator-managed HTTPS
  reverse proxy.
- Docker Engine `29.6.2`, Compose `v5.3.1`, `linux/arm64`, and Apple Silicon host
  evidence using `alpine:3.22` Push and Pull.
- Local username/password sessions; organizations and the accepted four-role matrix;
  public/private repositories; exact-scope short-lived Registry tokens.
- Distribution-owned Push/Pull, authenticated push-event reconciliation,
  repository-scoped Artifact/Tag reads, Quick-start, and immutable Digest detail.
- Manual maintenance-window PostgreSQL plus Registry-object backup, checksum-verified
  destructive restore, current migration application, login, private Pull, and Digest
  consistency rehearsal.
- Asynchronous digest-bound Trivy 0.72.0 vulnerability scans and CycloneDX SBOMs,
  including an authorized status API and real vulnerable/clean fixture evidence.
- Asynchronous Cosign 3.0.6 signature discovery, cryptographic verification, and
  versioned namespace trust evaluation, including authorized API and Web presentation
  of unsigned, invalid, unverified, trusted, untrusted, unavailable, and stale states.

## Operator-supplied requirements

- HTTPS reverse proxy, public certificate, DNS, host firewall, operating-system
  hardening, monitoring integration, and capacity planning.
- Strong PostgreSQL and MinIO credentials plus separately protected Registry signing
  keys, JWKS rotation material, event secret, environment, and TLS keys.
- Encryption, off-host transfer, access control, retention outside HubCR, and regular
  testing of real backup copies.

## Not supported or not proven

- Public SaaS operation, public signup, supported administrator-invitation redemption,
  password recovery, email verification, MFA, ownership transfer, or billing.
- Kubernetes, multiple API replicas, shared authentication limiter state, high
  availability, zero-downtime upgrades, multi-region delivery, or numerical capacity
  guarantees.
- Automated backup schedules, a fixed RPO/RTO, cross-region disaster recovery, or an
  automatic database downgrade.
- Repository deletion, retention, Tag history, Distribution garbage collection,
  quotas, audit export, robot accounts, access tokens, webhooks, replication, or proxy
  caching.
- Product trust-policy management API/UI, SBOM download, or scan/trust-based Pull
  blocking.
- Host/client compatibility beyond the recorded Apple Silicon and Docker versions;
  Linux and Windows deployment hosts require separate evidence.

The worker has durable Trivy scan/SBOM and Cosign/trust handlers, but their results are
informational and do not affect Push or Pull. Trust-policy seeding remains an isolated
acceptance helper, not a supported product management endpoint. Redis does not hold
authoritative business state. The authentication limiter is process-local. The
full security model and residual risks are recorded in the
[Registry MVP threat model](security-threat-model.md).
