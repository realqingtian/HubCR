# D-006 Public pull

**English** | [简体中文](d-006-public-pull.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Decision owner: product owner
- Blocks: Registry token flow and public-repository acceptance cases

## Context

A public repository may allow anonymous pulls or require authentication. The choice
changes Registry challenge, token issuance, discovery, and rate-limit behavior.

## Decision

Allow anonymous pull for explicitly `PUBLIC` repositories. Distribution still uses
the token flow: an anonymous caller may receive a short-lived token scoped only to
`pull` for the exact public repository. Push always requires an authenticated caller
with the approved capability. Missing visibility or policy data fails closed.

## Alternatives

- Require authentication for public pull: improves caller attribution but is less
  compatible with normal public Registry expectations.
- Deployment-configurable behavior: adds two acceptance modes before a default is
  proven.

## Consequences

M2 must test anonymous challenge/token/pull, private denial, exact scope, expiry, and
cross-repository reuse.
