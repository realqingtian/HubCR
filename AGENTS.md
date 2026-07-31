# HubCR repository instructions

**English** | [简体中文](AGENTS.zh-CN.md)

This is the primary instruction entry point for the HubCR repository. It contains
only repository-wide rules shared by every area of the project.

## Instruction scope and inheritance

- This file applies to the entire repository.
- `backend/AGENTS.md` adds rules only for files under `backend/`.
- `frontend/AGENTS.md` adds rules only for files under `frontend/`.
- A child instruction file supplements this file and takes precedence for its own
  directory when a rule is more specific.
- Do not apply backend-only rules to frontend work or frontend-only rules to backend
  work.
- For a task spanning both applications, read and follow both child files, but keep
  each rule within its declared scope.
- Files outside `backend/` and `frontend/` follow this root file and any nearer
  instruction file only.

## Required first steps

- Read `README.md` for current capabilities and limitations.
- Read `docs/architecture.md` and `docs/development.md` before structural or behavioral
  changes.
- Read `docs/requirements.md` and `docs/development-plan.md` before planning or
  implementing product capabilities. Respect their decision gates and do not start a
  blocked work package by inventing the missing policy.
- Read the `AGENTS.md` nearest to every file you will modify.
- Inspect `git status` and the relevant implementation before editing. Preserve
  unrelated user changes and never discard work you did not create.
- Treat current code and configuration as authoritative. Do not present roadmap items
  as implemented features.

## Repository-wide architecture

- HubCR is a modular monolith during the MVP phase.
- Go provides the business control plane and asynchronous worker.
- CNCF Distribution provides the OCI data plane and owns `/v2/`, manifest, blob,
  upload, download, and storage-driver behavior.
- PostgreSQL stores durable business state and initially backs asynchronous jobs.
- Redis is reserved for cache, rate-limit state, and short-lived coordination.
- OCI content is stored through Distribution in S3-compatible storage; local
  development uses MinIO.
- Long-running scanning and verification work belongs outside synchronous request and
  push paths.
- The web application is a separate consumer of the control-plane API; it does not
  own backend authorization or persistence rules.
- Changing these architectural choices requires a documented decision and maintainer
  approval before implementation.

## Shared product and security truth

- Namespace paths follow `hubcr.io/{namespace}/{repository}:{tag}`.
- Repository visibility is explicitly `PUBLIC` or `PRIVATE`; missing or unavailable
  data must never silently become public.
- Every repository access requires authorization even when content is physically
  deduplicated by digest.
- Web sessions are not long-lived Registry credentials.
- Registry tokens are short-lived and scoped to an exact repository and allowed
  actions such as `pull`, `push`, or `delete`.
- Artifact identity is the immutable digest; tags are mutable references.
- Scan reports, SBOMs, signatures, and verification results bind to artifact digests.
- Signature presence, cryptographic validity, and policy trust are separate states in
  APIs, storage, and UI.
- Scanning and verification remain asynchronous unless an accepted policy explicitly
  requires blocking.
- Never expose, log, or commit real credentials, tokens, authorization headers, or
  sensitive personal data.

## Product decisions that remain open

Do not silently choose registration mode, organization role names, repository grant
inheritance, scan-based pull blocking, signature trust roots, production deployment
target, billing, quotas, retention, or content governance. Record and obtain approval
for a decision before implementing schema, API, or UI contracts that depend on it.

## Shared implementation workflow

- Keep changes scoped to the requested behavior; avoid unrelated refactors.
- Identify the owning area and module before adding code.
- For planned product work, reference the relevant requirement and work-package IDs.
  Update plan state only when its acceptance evidence changes.
- Add or update focused tests for behavior changes and regressions.
- Prefer existing dependencies. Explain a new production dependency before adding it.
- Update architecture and status documentation when commands, boundaries, or available
  behavior change.
- Run targeted checks during development and the repository gate before completion.
- Report exactly what was verified and clearly state any untested runtime or external
  service path.

## Documentation and localization

- English is the canonical project-documentation language.
- Every user-facing Markdown document has a synchronized `.zh-CN.md` counterpart and
  a language switch at the top.
- Update both languages in the same change. Keep code identifiers, paths, commands,
  variables, API examples, and protocol names unchanged in translations.
- Do not document planned behavior as available.
- AI adapter files import or point to their nearest `AGENTS.md`; do not duplicate the
  canonical rules into separately maintained copies.

## Repository-wide validation

Before claiming completion, run:

```bash
make check
```

For documentation changes, also validate relative Markdown links, both language
directions, trailing whitespace, and final newlines. For behavior changes, run a
runtime smoke test when practical. If Docker or another dependency is unavailable,
state the limitation instead of claiming that path was tested.

## Git and external actions

- Do not commit, push, force-push, tag, publish, open a pull request, or modify external
  systems unless the user explicitly requests it.
- After completing and verifying any development or modification task, proactively ask
  whether the user wants the changes committed and pushed to the configured remote
  repository. Asking does not grant authorization; wait for an explicit confirmation
  before staging, committing, or pushing.
- Never use destructive Git commands to remove changes you did not create.
- Before an authorized commit, inspect the staged diff, run
  `git diff --cached --check`, and scan staged files for credentials.

## Repository-wide code review rules

- Flag changes that violate the control-plane and data-plane boundary.
- Flag authorization paths that can bypass namespace or repository checks.
- Flag scan, signature, or trust results keyed only by tag instead of digest.
- Flag synchronous security work added to the push path without an accepted policy.
- Flag planned functionality documented or displayed as implemented.
- Flag English documentation changed without its Simplified Chinese counterpart, and
  vice versa.
