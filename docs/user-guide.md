# HubCR Registry MVP user guide

**English** | [简体中文](user-guide.zh-CN.md)

This guide describes only workflows implemented in the current Registry MVP release
candidate. See the [release limitations](release-limitations.md) for unavailable
features and the [API conventions](api.md) for programmatic behavior.

## Sign in and navigate

Open the HTTPS HubCR origin supplied by the operator and sign in with an existing
local username and password. The Web session is revocable and is not a Docker or OCI
credential. The overview shows the authenticated user's personal namespace and any
organizations they belong to.

Administrator invitation redemption and account bootstrap are not yet available as
a supported user workflow. An operator cannot use test seed commands as production
account provisioning.

## Organizations and repositories

An authenticated user can create an organization. Organization `OWNER` and `ADMIN`
members can manage the approved role matrix; the final `OWNER` cannot be removed or
demoted. Repository creation is available in personal namespaces and to authorized
organization members.

Every repository is explicitly `PUBLIC` or `PRIVATE`. Missing or unavailable
visibility never becomes public. Users without discovery access receive the same
not-found response for a private repository as for a repository that does not exist.

## Pull and Push with Docker

Repository Detail displays Quick-start commands derived from current visibility and
the caller's server-computed `can_pull` and `can_push` capabilities. Do not infer
permissions from an organization role shown elsewhere.

- An anonymous client may Pull an explicitly public repository.
- A private Pull requires `docker login` with the user's local username and password.
- Push always requires authentication and the approved Push capability.
- A Web browser session cookie cannot be used as a Registry credential.
- Registry tokens are short-lived and limited to the exact repository and allowed
  action subset.

Use the exact origin and commands shown by Repository Detail because the operator may
use a non-default DNS name or port.

## Tags and Artifacts

Repository Detail lists mutable current Tags separately from immutable Artifacts.
Selecting a Tag opens the exact Digest detail. A Digest remains the Artifact identity
even if a Tag later moves.

Media type, size, creation time, Index descriptors, and Platform fields appear only
when known. Unknown descriptor data is not shown as an empty successful result.
Loading, empty, unavailable, denied, failed, not-found, and successful states remain
distinct.

The backend asynchronously records digest-bound vulnerability scans, CycloneDX SBOMs,
Cosign signature verification, and versioned trust evaluation. The Artifact security
panel shows their authorized state without making a client-side trust decision. Scan
and SBOM states remain queued, running, completed, failed, unavailable, or stale;
signatures separately distinguish unsigned, cryptographically invalid, unverified,
valid but untrusted, and valid trusted evidence. These results are informational and
do not block Pull. Missing evidence is not a successful security result.

## Current safety boundaries

Repository deletion, Tag history, retention, garbage collection, robot accounts,
personal access tokens, password recovery, MFA, audit export, quotas, and public
discovery are unavailable. Contact the operator for maintenance and recovery; do not
manipulate Distribution storage or PostgreSQL directly.
