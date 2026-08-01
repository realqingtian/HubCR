# HubCR control-plane API conventions

**English** | [简体中文](api.zh-CN.md)

These conventions apply to the JSON control-plane API under `/api/v1`. Registry
protocol endpoints `/token` and `/v2/` keep their protocol-specific contracts.

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
- `GET /api/v1/auth/me` returns the authenticated user or `401 authentication_failed`.
- `POST /api/v1/auth/logout` revokes the server-side session, clears the cookie, and is
  idempotent when the cookie is missing or unknown.
- The cookie is `HttpOnly`, `SameSite=Lax`, scoped to `/`, and has an explicit expiry.
  `Secure` defaults to `false` only for local HTTP development; HTTPS deployments must
  set `HUBCR_SESSION_COOKIE_SECURE=true`.
- Login and logout reject browser requests marked `Sec-Fetch-Site: cross-site`.
- Login calls an explicit rate-limit adapter before credential lookup. The current
  self-hosted foundation uses an allow-all adapter until Redis-backed limits are
  implemented; the public-service mode remains unavailable.

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

## OpenAPI ownership

[`openapi.yaml`](openapi.yaml) is the manually maintained, reviewed OpenAPI 3.1
contract. A public API change is incomplete until handlers, focused tests, this
document, and the OpenAPI contract agree. Generated API documentation may be added
later, but code annotations are not the source of truth during the MVP.
