# D-002 Registration mode

**English** | [简体中文](d-002-registration.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Decision owner: product owner
- Blocks: user lifecycle schema and registration UI

## Context

The MVP needs controlled user creation without pretending that email delivery,
verification, recovery, or public-service abuse controls already exist.

## Decision

Use administrator invitation for the MVP. An authorized administrator creates a
single-use, expiring invitation and delivers it out of band. Anonymous
self-registration and configurable registration modes are deferred.

## Alternatives

- Open self-registration: requires rate limiting, verification, recovery, and abuse
  controls before safe public exposure.
- Configurable modes: adds multiple lifecycle contracts and UI branches to M1.

## Consequences

M1 must model invitation issuance, expiry, redemption, and revocation. Invitation
secrets are shown only at creation and stored as hashes.
