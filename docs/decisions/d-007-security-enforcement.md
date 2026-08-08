# D-007 Security enforcement

**English** | [简体中文](d-007-security-enforcement.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-08
- Decision owner: product owner
- Blocks: G-03 and M4 scan-policy contracts

## Context

M4 introduces asynchronous vulnerability scanning and SBOM workflows. HubCR must
decide whether scan state changes Registry pull authorization before it defines job,
result, API, or UI contracts. Blocking pull too early would couple Registry
availability to scanner freshness and failure behavior before accuracy, exception,
and recovery evidence exists.

## Decision

The initial M4 policy is informational scanning only:

1. Push and pull authorization do not depend on scan availability, status, age,
   vulnerability severity, or failure.
2. Scanning remains asynchronous and never runs in the synchronous push or pull
   path.
3. APIs and UI represent `QUEUED`, `RUNNING`, `COMPLETED`, `FAILED`, and `STALE`
   truthfully. Missing, failed, or stale results must not be presented as safe.
4. Scan results and SBOMs bind to immutable artifact digests, not mutable tags.
5. Optional or mandatory pull blocking requires a later decision with measured
   scanner accuracy, freshness limits, scanner-outage behavior, administrator
   override and audit workflows, and Registry availability evidence.

This decision does not approve a severity threshold, fail-open or fail-closed
behavior, repository-specific enforcement, or a synchronous security gate.

## Alternatives

- Optional repository-level blocking: deferred because repository-specific security
  policy ownership and override behavior are not yet defined.
- Mandatory severity-based blocking: deferred because the initial scanner has no
  accepted freshness, false-positive, exception, or availability contract.
- Fail-closed on missing, stale, or failed results: rejected for initial M4 because a
  scanner or job outage would become an unproven Registry outage mechanism.
- Fail-open enforcement: rejected for initial M4 because calling a policy enforced
  while bypassing failed checks would be misleading.

## Consequences

- M4 can establish durable jobs, digest-bound results, retry, stale-state, API, and UI
  evidence without changing Registry availability.
- Product surfaces must describe results as observations, not permission decisions or
  guarantees that an artifact is safe.
- A later enforcement proposal must define policy ownership, exemptions, audit,
  freshness, failure semantics, and rollout/recovery before it can change pull
  authorization.
