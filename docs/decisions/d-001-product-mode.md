# D-001 Product mode

**English** | [简体中文](d-001-product-mode.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Decision owner: product owner
- Blocks: identity and repository-discovery contracts

## Context

Public SaaS requires abuse controls, account recovery, content governance, and
operational policy that are deferred beyond the Registry MVP.

## Decision

Prioritize self-hosted/private deployment for the MVP. Instances may expose explicitly
public repositories, but operating HubCR as a public multi-tenant SaaS is deferred.

## Alternatives

- Public SaaS first: closest to Docker Hub, but pulls public-service controls into M1.
- Equal priority: increases configuration and acceptance scope before either mode is
  proven.

## Consequences

M1 can optimize bootstrap and administration for an instance operator. Registration,
billing, abuse prevention, and content governance for public service remain
unavailable.
