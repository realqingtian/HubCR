<!-- BEGIN:nextjs-agent-rules -->
# HubCR frontend instructions

**English** | [简体中文](AGENTS.zh-CN.md)

These instructions apply only to files under `frontend/`. They supplement the root
`AGENTS.md`. Do not use this file to impose frontend implementation rules on
`backend/` or other sibling directories.

## Installed Next.js version is authoritative

This may not be the Next.js version represented by an agent's training data. APIs,
conventions, and file structure can differ. Before writing framework code, read the
relevant guide under `node_modules/next/dist/docs/` and follow current deprecation
notices.
<!-- END:nextjs-agent-rules -->

## Frontend scope

The frontend is the Next.js web application for public registry pages and
authenticated HubCR workflows. It consumes control-plane APIs but does not implement
backend authorization, Registry token issuance, persistence, or worker behavior.

Before frontend work, inspect:

- `frontend/package.json` and `frontend/tsconfig.json`;
- the relevant route, feature, API schema, provider, and tests;
- the relevant installed Next.js documentation;
- `docs/architecture.md` and the frontend sections of `docs/development.md`.

For a frontend-only task, do not modify `backend/` unless the user explicitly requests
a coordinated backend change. If the required API does not exist, define or document
the frontend expectation and report the backend dependency instead of inventing a
client-side substitute.

## Frontend ownership

- `app/` owns routes, layouts, metadata, loading states, and error boundaries.
- `features/<capability>/` owns feature UI, feature hooks, and feature-local models.
- `lib/api/` owns low-level typed HTTP access and Zod response schemas.
- `providers/` owns application-wide client providers.
- `public/` contains static assets only.

Do not accumulate reusable product logic in route files. Do not put feature-specific
behavior into `lib`, and do not create a generic component or utility until at least
two real consumers share the same semantics.

## Next.js and React rules

- Use App Router conventions from the installed documentation.
- Pages and layouts are Server Components by default.
- Add `"use client"` only at the smallest component boundary that needs state, event
  handlers, effects, browser APIs, React context, or a client-only library.
- Do not move an entire route or layout to the client merely to support one interactive
  child.
- Keep client-component props serializable across the server/client boundary.
- Fetch on the server when data is needed for initial rendering and does not require a
  client-only session mechanism.
- Use route `loading`, `error`, and `not-found` conventions when the user experience
  needs those states.
- Avoid unnecessary effects and duplicated derived state.

## TypeScript and API data

- Keep TypeScript strict. Do not use `any`, unsafe casts, or non-null assertions to
  hide contract mismatches.
- Treat API responses as untrusted and validate them with Zod in `lib/api` before
  feature code consumes them.
- Keep server DTO types separate from presentation models when their meanings differ.
- Use TanStack Query for client-side server state, caching, invalidation, and mutation
  lifecycles. Do not duplicate its cache in component state.
- Query keys must be stable, serializable, and include every input that changes the
  result.
- Requests must expose loading, empty, unavailable, error, and success as distinct
  truthful states.
- Never infer unsupported security or registry status. Unknown and unavailable values
  remain unknown or unavailable, not successful zero values.
- Never expose secrets through `NEXT_PUBLIC_*` variables or client bundles.

## UI, styling, and accessibility

- Use Tailwind CSS and existing design tokens. Do not add a new component library,
  icon system, or styling framework without approval.
- Prefer semantic HTML before adding ARIA.
- All interactive controls require keyboard support, visible focus, and accessible
  names.
- Form inputs need labels and validation messages associated with the relevant field.
- Preserve readable color contrast and do not rely on color alone to communicate
  status.
- Layouts must work at mobile and desktop widths without hiding critical actions.
- Keep public and authenticated UI state truthful to backend capabilities; do not show
  roadmap features as available controls.

## Feature boundaries

Expected feature areas include `auth`, `organizations`, `namespaces`, `repositories`,
and `security`. Create a feature directory only when implementing a confirmed workflow.

- Features may depend on `lib` and shared UI, but must not import unrelated feature
  internals.
- Move code to a shared location only when the abstraction preserves behavior and has
  multiple real consumers.
- The frontend may display authorization outcomes but must not reproduce backend
  authorization decisions as a security boundary.
- Signature states must distinguish absent, discovered, cryptographically valid,
  trusted by policy, and invalid or expired.
- Artifact views use digest identity even when routes display a tag.

## Frontend tests

- Use Vitest for unit tests and schema tests.
- Colocate tests with the behavior they cover or use a nearby `__tests__` directory.
- Test visible behavior, validation, state transitions, and accessibility-relevant
  semantics instead of implementation details.
- Add a regression test for every frontend bug fix.
- Do not make unit tests depend on a live backend, network, timers, or order of
  execution.
- Add Playwright tests when complete user workflows are introduced; do not treat unit
  tests as proof of an end-to-end flow.

## Frontend documentation

When frontend behavior, commands, dependencies, routes, or structure changes, update
both `frontend/README.md` and `frontend/README.zh-CN.md`. Follow the root bilingual
documentation rules for any other affected document.

## Frontend validation

Use Bun for this application. During frontend development, run from `frontend/`:

```bash
bun run typecheck
bun run test
bun run lint
bun run build
```

Before declaring the overall task complete, run `make check` from the repository root.
Perform a real browser check when layout, interaction, routing, hydration, or responsive
behavior changes; automated build success alone is insufficient for those changes.

## Frontend code review rules

- Flag unnecessary Client Components and overly broad `"use client"` boundaries.
- Flag unvalidated API data or duplicated TanStack Query state.
- Flag UI that invents authorization, scan, signature, or trust outcomes.
- Flag inaccessible interactive controls or missing loading/error/empty states.
- Flag feature code placed in routes or unrelated shared modules.
- Flag frontend-only work that modifies backend implementation without explicit scope.
