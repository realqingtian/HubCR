# HubCR Registry authentication protocol

**English** | [简体中文](registry-authentication.zh-CN.md)

- Status: `ACCEPTED`
- Approved: 2026-08-01
- Work package: M2-01
- Requirements: FR-REG-001, FR-REG-002
- Decisions: D-003, D-004, D-005, D-006
- Implementation: M2-02 through M2-04 completed and accepted on 2026-08-01

This document defines the authentication contract between OCI clients, the HubCR
token service, the gateway, and CNCF Distribution. M2-02 through M2-04 now implement
this accepted contract; event-driven artifact reconciliation remains outside it.

## 1. Ownership and request flow

HubCR keeps the control plane and OCI data plane separate:

```text
Docker / OCI client
    |
    | 1. unauthenticated /v2/* request
    v
gateway ------------------------------> CNCF Distribution
    ^                                      |
    | 4. retry /v2/* with Bearer token     | 2. 401 Bearer challenge
    |                                      v
    +---------------- client --------> gateway /token
                                           |
                                           | 3. authenticate, authorize,
                                           |    sign short-lived token
                                           v
                                      Go control plane
```

- The gateway routes `/v2/*` to Distribution. The Go control plane must not
  implement manifests, blobs, uploads, downloads, or OCI error behavior.
- The gateway routes `/token` to the Go control plane.
- Distribution creates Registry challenges and validates Bearer tokens.
- HubCR authenticates Registry callers, evaluates repository policy, and signs
  tokens.
- Browser `/api/v1` sessions and Registry credentials are separate. The token
  endpoint ignores session cookies and never converts a web session into a
  Registry credential.

The external gateway implementation and production deployment target remain outside
M2-01. M2-04 may select a local gateway implementation without deciding D-009.

## 2. Protocol identifiers and configuration

The following values are explicit trusted configuration. They must not be derived
from an untrusted `Host` or forwarded header.

| Concept | M2 contract | Constraint |
| --- | --- | --- |
| External Registry origin | operator-configured URL | HTTPS outside explicitly documented local development |
| Token realm | external Registry origin plus `/token` | absolute URL exposed in the Distribution challenge |
| Service | `hubcr-registry` by default | exact, case-sensitive match |
| JWT audience | same value as Service | exact match enforced by Distribution |
| JWT issuer | `hubcr-token-service` by default | exact, case-sensitive match |
| Token TTL | 300 seconds by default | configurable from 60 through 900 seconds |
| Clock-skew allowance | 30 seconds | applied only to `nbf`; it does not extend `exp` |

Deployments may override Service and Issuer to avoid collisions, but the token
service and Distribution configuration must use identical values. Startup fails if
the realm is not absolute, the service or issuer is empty, the TTL is outside its
allowed range, or signing configuration is invalid.

Distribution's token-auth configuration will use the same `realm`, `service`, and
`issuer`, plus the trusted public-key bundle. `autoredirect` stays disabled so
proxy headers cannot silently change the realm.

## 3. Distribution challenge contract

Distribution owns every `/v2/*` response. When a request needs authorization, it
returns `401 Unauthorized` with a challenge equivalent to:

```http
WWW-Authenticate: Bearer realm="https://hubcr.example/token",service="hubcr-registry",scope="repository:team/image:pull,push"
```

Rules:

1. `realm` is the configured public token URL.
2. `service` is the configured service and therefore the required JWT audience.
3. `scope` is omitted when no repository action is needed, such as the initial
   `/v2/` capability check.
4. Distribution may include one or more `scope` values for an OCI operation.
5. HubCR does not rewrite or proxy the challenge body. The gateway preserves
   `WWW-Authenticate` and `Docker-Distribution-Api-Version`.

## 4. Token request contract

M2 supports:

```http
GET /token?service=hubcr-registry&scope=repository:team/image:pull,push&client_id=docker
```

| Input | Behavior |
| --- | --- |
| `service` | Required exactly once and must equal configured Service |
| `scope` | Optional and repeatable; each value is parsed independently |
| `client_id` | Optional, syntax-validated, retained only as secret-safe audit context |
| `offline_token` | Optional Boolean compatibility hint; accepted, but M2 never issues a refresh token |
| HTTP Basic credentials | Optional; if present, authenticate through the Registry credential boundary |
| Cookies | Ignored |

Only `GET` is required for M2. Unsupported methods return `405 Method Not Allowed`
with `Allow: GET`.

### 4.1 Caller authentication

- No `Authorization` header means an anonymous caller.
- `Authorization: Basic ...` uses the same local username/password identity store as
  D-003, through a Registry-specific application boundary. It does not create a web
  session.
- Valid credentials produce a stable opaque user ID for `sub`; usernames and email
  addresses are not embedded in tokens.
- Malformed, unsupported, or invalid presented credentials return
  `401 Unauthorized`. They never fall back to anonymous access.
- The response may include `WWW-Authenticate: Basic realm="HubCR Registry"` so a
  client can identify the failed credential exchange.
- Bearer credentials and web-session cookies are not accepted as credentials for
  minting a new Registry token in M2.

## 5. Repository scope

### 5.1 Grammar

HubCR accepts the Distribution resource-scope shape:

```text
repository:{namespace}/{repository}:{action[,action...]}
```

For example:

```text
repository:team/image:pull,push
```

HubCR's MVP constraints are stricter than the general Distribution grammar:

- Resource type must be exactly `repository`. Deprecated resource classes such as
  `repository(plugin)` are not emitted or accepted.
- Resource name has exactly two path components: namespace and repository.
- Each component is lowercase ASCII, at most 64 bytes, and matches
  `[a-z0-9]+(?:[._-][a-z0-9]+)*`.
- Host-prefixed resource names, ports, empty components, extra path components,
  Unicode, whitespace, and traversal syntax are rejected.
- The parser locates the first and last `:` delimiters rather than naively splitting
  into an arbitrary number of fields.
- Actions are case-sensitive. Recognized actions are `pull`, `push`, and
  `delete`.
- Duplicate actions and duplicate identical scopes are deduplicated. Claims use the
  deterministic action order `pull`, `push`, `delete`.
- Inputs are validated, not normalized into a different repository identity.

### 5.2 Multiple scopes

The token endpoint accepts repeated `scope` parameters because a standards-compatible
client may request more than one resource. Each exact repository is resolved and
authorized independently. Access entries are sorted by repository name so equivalent
requests produce deterministic claims.

Authorization for repository A never supplies an action for repository B. A token
containing only A remains unusable for B. A token may contain A and B only when the
client explicitly requested both and policy independently grants actions on both.
Implementations must apply bounded request size and scope-count limits before parsing;
the exact transport limits belong to M2-03.

An absent `scope` is valid and produces an empty `access` array, allowing the
client to complete a base `/v2/` capability check. An empty scope value or malformed
scope is a protocol error.

## 6. Authorization and action intersection

For every valid repository scope:

```text
token actions = requested actions ∩ policy-allowed actions
```

The policy source is the central authorization module, not HTTP handlers or
Distribution configuration.

| Repository and caller | Policy-allowed Registry actions |
| --- | --- |
| Explicitly `PUBLIC`, anonymous | `pull` |
| Explicitly `PUBLIC`, authenticated | `pull` plus any authenticated capability below |
| Personal namespace owner | `pull`, `push` |
| Personal namespace non-owner | no private access; public rule still applies |
| Organization `OWNER`, `ADMIN`, or `WRITER` | `pull`, `push` |
| Organization `READER` | `pull` |
| Missing membership or wrong organization | no private actions |
| Missing repository, visibility, namespace owner, role, or policy data | no actions |

`delete` is recognized so the protocol remains explicit, but M2 never grants it.
Repository deletion, retention, and garbage-collection policy are deferred, and
M2-04 must disable Distribution deletion before connecting token authentication.

A well-formed request with a valid caller but no allowed actions is not a token
endpoint authorization error. HubCR returns `200 OK` with the exact repository entry
and an empty `actions` array. Distribution then denies the original OCI request.
This preserves the standard action-intersection flow and avoids using token endpoint
status differences to reveal whether a private repository exists.

## 7. JWT contract

The returned Bearer token is a signed JWT that clients treat as opaque.

### 7.1 JOSE header

| Field | Value |
| --- | --- |
| `typ` | `JWT` |
| `alg` | `RS256` for the first implementation |
| `kid` | Distribution-compatible JWK thumbprint of the active public key |

Symmetric HMAC algorithms are forbidden because they would share signing authority
with the data plane. M2-02 may add another asymmetric algorithm only with signer,
Distribution, rotation, and negative-test evidence.

### 7.2 Claims

| Claim | Contract |
| --- | --- |
| `iss` | configured Issuer |
| `sub` | stable opaque user ID, or `""` for anonymous |
| `aud` | configured Service as a single JSON string |
| `exp` | `iat + TTL` |
| `nbf` | `iat - 30 seconds` |
| `iat` | UTC issuance instant |
| `jti` | cryptographically random, at least 128 bits |
| `access` | array of exact `type`, `name`, and intersected `actions` entries |

The token contains no password, session ID, email address, organization membership,
repository visibility, or complete policy snapshot.

### 7.3 Signing-key lifecycle

- M2-02 uses asymmetric keys. The token service receives private signing material;
  Distribution receives public verification material only.
- Production private keys come from a read-only secret-mounted file, are never stored
  in the database or source tree, and must not be supplied as a plain environment
  variable.
- Configuration identifies one active signing key. Every issued token uses its
  `kid`.
- The Distribution trust bundle may contain the active public key and retiring public
  keys. Issuance never uses a retiring key.
- Rotation is staged: add the new public key to Distribution and reload/restart it;
  switch the token service to the matching private key; retain the old public key for
  at least maximum TTL plus clock skew; then remove it in a later reload/restart.
- Startup fails for an unreadable key, public/private mismatch, duplicate `kid`,
  unsupported algorithm, or an active key absent from the configured trust set.
- Repository fixtures may include visibly unsafe local-only test keys. Production
  startup must reject those fixtures.

## 8. Token response and lifetime

Successful response:

```json
{
  "token": "<signed-jwt>",
  "access_token": "<same-signed-jwt>",
  "expires_in": 300,
  "issued_at": "2026-08-01T00:00:00Z"
}
```

- `token` and `access_token` are both returned and are byte-for-byte identical.
- `expires_in` equals the configured TTL and is never less than 60.
- `issued_at` is RFC 3339 UTC and matches `iat` to whole-second precision.
- Every success and error response uses `Content-Type: application/json`,
  `Cache-Control: no-store`, and `Pragma: no-cache`.
- M2 does not enable cross-origin browser access to `/token`; OCI clients do not
  require CORS.
- The default TTL is five minutes. The maximum is fifteen minutes to limit replay
  exposure while allowing normal image transfers to start.
- No refresh token is issued in M2. Docker CLI sends `offline_token=true` during
  login, so HubCR accepts the hint while returning only the short-lived access token.
  Clients request another short-lived token after expiry.
- Tokens are not server-side sessions and are not individually revocable. Password or
  membership changes affect future issuance; already issued tokens remain valid until
  expiry unless the signing key is removed as an emergency response.

## 9. Error contract

`/token` is a Registry protocol endpoint, not an `/api/v1` business API. It uses
the Distribution JSON error shape:

```json
{
  "errors": [
    {
      "code": "UNAUTHORIZED",
      "message": "registry credentials are invalid"
    }
  ]
}
```

| Condition | Status | Code |
| --- | --- | --- |
| Missing, repeated, or mismatched `service` | `400` | `DENIED` |
| Malformed scope, unsupported type/action, invalid `client_id` | `400` | `DENIED` |
| Repeated or non-Boolean `offline_token` | `400` | `DENIED` |
| Malformed, unsupported, or invalid credentials | `401` | `UNAUTHORIZED` |
| Password-attempt admission denied | `429` | `TOOMANYREQUESTS` |
| Unsupported method | `405` | `UNSUPPORTED` |
| Policy lookup or signing dependency unavailable | `503` | `UNAVAILABLE` |
| Unexpected internal failure | `500` | `UNKNOWN` |

Error messages are stable and generic. They do not include repository existence,
visibility, membership, password details, key paths, SQL errors, token contents, or
stack traces. Dependency failure never becomes an empty successful policy decision;
it fails closed with `503`.

## 10. Logging and security controls

M2-03 and M2-09 must provide structured, secret-safe events for token requests and
failures. Allowed fields include request ID, configured service, authenticated versus
anonymous state, opaque actor ID, exact canonical repository, requested actions,
granted actions, decision reason class, signing `kid`, status, and duration.

Never log:

- `Authorization`, `Cookie`, raw Basic credentials, passwords, JWTs, private keys,
  or full request URLs containing credentials;
- arbitrary parser input before validation;
- database errors containing sensitive values.

Rate limiting is not an authorization policy, but every password-verification path
uses bounded attempt and concurrency admission in the single-process MVP. Request
body/query size and scope count are also bounded. Multi-replica deployment requires
shared limiter state without changing authorization truth.

## 11. Acceptance evidence required after implementation

M2-02 through M2-04 are not accepted by unit tests alone. The implementation must
cover:

| Case | Required evidence |
| --- | --- |
| Base `/v2/` | Distribution challenge then valid empty-access token |
| Anonymous public pull | exact repository `pull` succeeds |
| Anonymous private pull | empty actions; Distribution denies without existence leak |
| Authenticated role matrix | `OWNER`/`ADMIN`/`WRITER` push; `READER` pull only |
| Personal namespace | owner push/pull; non-owner denied unless public pull |
| Action intersection | request `pull,push`, grant only policy subset |
| Cross-repository isolation | token for A cannot access B |
| Multiple explicit scopes | each repository independently intersected |
| Invalid token | expired, future-`nbf`, wrong issuer, audience, signature, or `kid` denied |
| Key rotation | active and retiring keys verify during overlap; retired key fails after removal |
| Secret safety | logs and errors contain no credential or token material |
| Real client | Docker or supported OCI client pushes and pulls through the local gateway |

## 12. Normative references

- [CNCF Distribution Token Authentication Specification](https://distribution.github.io/distribution/spec/auth/token/)
- [CNCF Distribution Token Scope Documentation](https://distribution.github.io/distribution/spec/auth/scope/)
- [CNCF Distribution JWT Authentication Implementation](https://distribution.github.io/distribution/spec/auth/jwt/)
- [CNCF Distribution Registry Configuration](https://distribution.github.io/distribution/about/configuration/#token)
- [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md)

## 13. Review gate

Approval of this document closed M2-01 and unblocked M2-02. The approved contract
confirms:

1. the fixed Service/Audience and Issuer contract;
2. the 300-second default and 60–900-second TTL range;
3. RS256 and the staged public-key overlap rotation model;
4. independent authorization of repeated repository scopes;
5. empty-action `200 OK` for well-formed but unauthorized scopes;
6. Basic Registry authentication remaining separate from web sessions;
7. the same-origin gateway path contract without selecting a production target.
