# D-005 Grant inheritance

**English** | [简体中文](d-005-grant-inheritance.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Decision owner: product owner
- Blocks: central authorization-policy contract

## Context

Repository-specific grants add precedence, revocation, discovery, and cross-tenant
test cases to the MVP.

## Decision

Use organization-role-only authorization for organization repositories in the MVP. A
role applies consistently to every repository in that organization. Personal namespace
repositories are controlled by their owning user. Repository-specific user or team
grants are deferred.

## Alternatives

- Add repository-specific grants: supports finer access but requires a complete
  inheritance and conflict-resolution policy.
- Make grant behavior configurable: multiplies the authorization and token matrices.

## Consequences

The first authorization service remains small and deny-by-default. Teams needing
different access boundaries must use separate organizations until repository grants
exist.
