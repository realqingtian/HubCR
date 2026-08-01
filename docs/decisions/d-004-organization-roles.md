# D-004 Organization roles

**English** | [简体中文](d-004-organization-roles.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Decision owner: product owner
- Blocks: membership schema, APIs, and authorization matrix

## Context

Role names are insufficient unless every MVP capability has a deterministic result.
Deletion, ownership transfer, billing, and security-policy enforcement remain deferred.

## Decision

Use `OWNER`, `ADMIN`, `WRITER`, and `READER` organization roles:

| Capability | OWNER | ADMIN | WRITER | READER |
| --- | --- | --- | --- | --- |
| View organization and members | Yes | Yes | Yes | Yes |
| Change organization settings | Yes | No | No | No |
| Manage owners or admins | Yes | No | No | No |
| Invite/remove WRITER or READER | Yes | Yes | No | No |
| Create repositories | Yes | Yes | Yes | No |
| Change repository visibility | Yes | Yes | No | No |
| Edit repository description | Yes | Yes | Yes | No |
| Push organization repositories | Yes | Yes | Yes | No |
| Pull private organization repositories | Yes | Yes | Yes | Yes |

Every organization must retain at least one `OWNER`. Repository deletion and ownership
transfer are not introduced by this record.

## Alternatives

- `OWNER` and `MEMBER` only: simpler, but cannot represent read-only access or
  delegated administration.
- Custom roles: flexible, but adds policy schema and UI before the baseline is proven.

## Consequences

M1 tests must cover every capability and role, including denial and missing
membership.
