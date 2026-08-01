# D-003 Initial identity

**English** | [简体中文](d-003-initial-identity.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Decision owner: product owner
- Blocks: credentials, sessions, login API, and login UI

## Context

Local credentials work without an external identity provider. OIDC reduces local
password handling but cannot bootstrap every self-hosted installation by itself.

## Decision

Use local username/password credentials as the MVP default. Store only a modern,
salted password hash, issue revocable server-side web sessions, and keep the
authentication boundary ready for a later OIDC provider.

## Alternatives

- OIDC only: reduces password scope but makes an external provider mandatory.
- Local credentials and OIDC together: broadens the first account-linking contract.

## Consequences

M1 needs password hashing, secret-safe login errors, session expiry/revocation, and
rate-limit-ready endpoints. Email verification, recovery, MFA, and OIDC remain
unavailable.
