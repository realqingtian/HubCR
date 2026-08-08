# HubCR control-plane API conventions

**English** | [简体中文](api.zh-CN.md)

These conventions apply to the JSON control-plane API under `/api/v1`. Registry
protocol endpoints `/token` and `/v2/` keep their protocol-specific contracts.

In the supported single-host Compose deployment, callers use one operator-supplied
HTTPS origin. The gateway routes `/api/` to the control plane, `/token` to the scoped
token endpoint, `/v2/` to Distribution, and all other paths to the Web application.
Only the gateway publishes a host port; direct API and data-service endpoints remain
private. See the [operator guide](operator-guide.md), [user guide](user-guide.md), and
[release limitations](release-limitations.md).

## Transport

- Request and response bodies use `application/json`.
- JSON request bodies are limited to 1 MiB, reject unknown fields, and must contain
  exactly one value.
- Successful response bodies are endpoint-specific and are not wrapped in a generic
  data envelope.
- Timestamps use UTC RFC 3339 with an explicit `Z` timezone; fractional seconds are
  preserved when present.

## Errors

Errors use a stable envelope:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request validation failed",
    "fields": [
      {"field": "name", "message": "must not be empty"}
    ]
  },
  "request_id": "7f35c89a99194586b19dfba975b5e11b"
}
```

The shared codes are `invalid_request`, `validation_failed`, `not_found`,
`method_not_allowed`, `authentication_failed`, `rate_limited`, `forbidden`, `conflict`,
and `internal_error`.
Authentication failures do not distinguish an unknown username, wrong password,
missing session, expired session, or revoked session. Error messages never expose
database errors, SQL, stack traces, paths, credentials, tokens, or authorization
headers.

## Request correlation

Clients may send `X-Request-ID` using 1–128 ASCII letters, digits, `.`, `_`, or `-`.
The API generates a new 128-bit hexadecimal value when the header is missing or
invalid. Every response echoes the accepted or generated ID in `X-Request-ID`, and
error envelopes include it as `request_id`.

## Pagination

List endpoints use bounded cursor pagination:

- `limit` defaults to `20` and must be between `1` and `100`;
- `cursor` is an opaque value up to 512 characters;
- repeated `limit` or `cursor` query parameters are invalid;
- responses expose `meta.limit` and omit `meta.next_cursor` when there is no next
  page.

## Status behavior

- malformed JSON and unsupported content types return `400 invalid_request`;
- domain validation returns `422 validation_failed` with optional field details;
- unknown routes return `404 not_found`;
- known paths with unsupported methods return `405 method_not_allowed` and `Allow`;
- unexpected failures and recovered panics return `500 internal_error` without the
  internal cause.

Liveness is process-oriented. Readiness returns `503` with
`{"status":"unavailable"}` while required PostgreSQL access is unavailable and
recovers automatically when the dependency returns.

## Web authentication

- `POST /api/v1/auth/login` accepts a local username and password and returns the user
  plus session expiry. The opaque session secret is returned only as the
  `hubcr_session` cookie, never in JSON.
- `GET /api/v1/auth/me` returns the authenticated user and explicit
  `personal_namespace`, or `401 authentication_failed`.
- `POST /api/v1/auth/logout` revokes the server-side session, clears the cookie, and is
  idempotent when the cookie is missing or unknown.
- The cookie is `HttpOnly`, `SameSite=Lax`, scoped to `/`, and has an explicit expiry.
  `Secure` defaults to `false` only for local HTTP development; HTTPS deployments must
  set `HUBCR_SESSION_COOKIE_SECURE=true`.
- Login and logout reject browser requests marked `Sec-Fetch-Site: cross-site`.
- Login and Registry Basic authentication share one bounded password-attempt boundary
  before credential lookup and Argon2 work. The single-process MVP limits attempts by
  normalized account and direct client, caps concurrent verification, and returns
  `429 rate_limited` when admission is denied. Multi-replica deployment requires the
  planned shared Redis implementation; public-service mode remains unavailable.

## Registry token protocol

The accepted [Registry authentication protocol](registry-authentication.md) owns the
non-`/api/v1` `GET /token` contract.

- Registry authentication is explicitly feature-gated and defaults to disabled until
  M2-04 connects Distribution and the local gateway.
- The endpoint accepts the exact configured `service`, repeatable canonical
  repository `scope` values, optional `client_id`, and optional HTTP Basic
  credentials.
- Basic credentials are verified without creating a web session. Cookies and Bearer
  credentials are ignored or rejected as specified by the protocol.
- Successful responses contain identical `token` and `access_token` JWTs,
  `expires_in`, and `issued_at`; every response is non-cacheable.
- The JWT contains only independently policy-intersected `pull` and `push` actions.
  `delete` is recognized but never granted in M2.
- Protocol errors use the Distribution `errors` array rather than the business API
  error envelope.
- Enabling the route requires an explicit external origin, Service/Audience, Issuer,
  60–900-second TTL, an absolute read-only RS256 private-key path, and a trusted
  public JWKS containing the active key.

## Organizations

All organization endpoints require a valid `hubcr_session` cookie.

- `POST /api/v1/organizations` creates an organization, its globally unique namespace,
  and the caller's initial `OWNER` membership atomically.
- `GET /api/v1/organizations` lists organizations containing the caller.
- `GET /api/v1/organizations/{organization_id}` and the corresponding `/members`
  endpoint require any organization membership.
- `POST /api/v1/organizations/{organization_id}/members` adds a member; `PATCH` and
  `DELETE` on `/members/{user_id}` change a role or remove a member.
- `OWNER` may manage all roles. `ADMIN` may manage only `WRITER` and `READER`.
  `WRITER` and `READER` cannot manage members, and the last `OWNER` cannot be demoted
  or removed.
- Organization and member lists use the shared `limit` plus opaque `cursor` contract.
  Member write requests marked `Sec-Fetch-Site: cross-site` are rejected.

## Repositories

All repository endpoints currently require a valid `hubcr_session` cookie and use the
canonical path `/api/v1/namespaces/{namespace}/repositories/{repository}`.

- `POST /api/v1/namespaces/{namespace}/repositories` creates a repository with an
  explicit `PUBLIC` or `PRIVATE` visibility. Personal namespace owners and
  organization `OWNER`, `ADMIN`, or `WRITER` members may create repositories.
- `GET /api/v1/namespaces/{namespace}/repositories` returns a bounded page. A caller
  outside the namespace sees only explicitly `PUBLIC` repositories; personal owners
  and all organization roles may also discover `PRIVATE` repositories.
- `GET .../{repository}` returns `404` for a missing repository and for a private
  repository the caller cannot discover, avoiding private-existence disclosure. A
  successful detail response includes caller-specific `capabilities.can_pull` and
  `capabilities.can_push` booleans computed by the centralized policy. It does not
  expose or require the client to reproduce organization roles.
- `PATCH .../{repository}` can change `description`, `visibility`, or both. Personal
  owners and organization `OWNER`/`ADMIN` may change visibility; organization
  `WRITER` may edit descriptions but cannot change visibility; `READER` cannot mutate
  repository metadata.
- Visibility changes update `visibility_updated_by_user_id`,
  `visibility_updated_at`, and `updated_at` atomically. Description-only changes leave
  the visibility evidence unchanged.
- Namespace and repository names are case-normalized lowercase OCI path components,
  at most 64 bytes. Repository names are unique within a namespace. Repository lists
  use the shared `limit` plus opaque `cursor` contract, and mutation requests marked
  `Sec-Fetch-Site: cross-site` are rejected.

## Artifacts and tags

All Artifact and Tag endpoints require a valid `hubcr_session` cookie and are scoped
under `/api/v1/namespaces/{namespace}/repositories/{repository}`.

- `GET .../artifacts` lists repository-scoped immutable Artifacts ordered by Digest.
- `GET .../artifacts/{digest}` returns Manifest or Index details. A confirmed Index
  descriptor set is returned as ordered `manifests`; the field is omitted when the
  set is unknown and is `[]` when it is confirmed empty.
- `GET .../tags` lists mutable current Tag references ordered by case-sensitive Tag
  name.
- `GET .../tags/{tag}` returns the current Tag reference and embeds the corresponding
  Artifact detail so clients can inspect media type, size, discovery/source creation
  time, and Index Platform metadata without treating the Tag as identity.
- Artifact and Tag lists use the shared `limit` and opaque `cursor` contract. Artifact
  cursors encode a Digest; Tag cursors encode a Tag name.
- Repository discovery authorization is evaluated before metadata access. A private
  repository that the caller cannot discover returns the same `404 not_found` as a
  missing repository, and no Artifact query runs.
- Optional media type, size, source creation time, and Platform fields are omitted
  when unavailable. The API does not invent zero values or missing Platform facts.
- These endpoints are read-only. Distribution continues to own `/v2/`, and deletion,
  retention, Tag history, and garbage collection remain outside M2-07.

## Artifact security

`GET .../artifacts/{digest}/security` requires a valid session and reuses repository
discovery authorization before reading security storage. A missing Artifact and an
inaccessible private repository both return `404 not_found` without disclosing private
existence.

The response keeps scan, SBOM, and signature verification state separate. Scan and
SBOM each contain `state`, `attempts`,
and `updated_at`; supported states are `QUEUED`, `RUNNING`, `COMPLETED`, `FAILED`, and
`STALE`. Failed work may include a machine-readable `error_code`. Completed or stale
scan evidence includes the Trivy and vulnerability-database versions, database
timestamps, finding count, and severity counts. A persisted SBOM includes
`completed_at` and `format: CYCLONEDX_JSON`. Queued, running, failed, or missing
evidence never receives invented completion fields or successful zero values.

`signature.state` is `ABSENT` when no trust-policy verification workflow exists.
Otherwise it reports the durable job state and fixed `policy_id`/`policy_version`.
Completed or stale verification includes the Cosign version, completion time, and
zero or more digest-identified signature/attestation evidence records. Each record
separates `cryptographic_state` (`VALID`, `INVALID`, `UNVERIFIED`, or `UNAVAILABLE`)
from `trust_state` (`TRUSTED`, `UNTRUSTED`, or `NOT_EVALUATED`) and includes the
backend reason plus exact public-key fingerprint or keyless issuer/subject evidence
when available. An empty completed evidence array truthfully means no signature or
attestation was discovered. `STALE` means the evidence belongs to an older immutable
policy version and is not a current trust conclusion.

This endpoint exposes informational state only; it does not block Push or Pull.

## OpenAPI ownership

[`openapi.yaml`](openapi.yaml) is the manually maintained, reviewed OpenAPI 3.1
contract. A public API change is incomplete until handlers, focused tests, this
document, and the OpenAPI contract agree. Generated API documentation may be added
later, but code annotations are not the source of truth during the MVP.
