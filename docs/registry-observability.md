# Registry operational observability

**English** | [简体中文](registry-observability.zh-CN.md)

- Status: `IMPLEMENTED`
- Implemented: 2026-08-01
- Work package: M2-09
- Requirements: FR-REG-006 and FR-OPS-002

This document records the implemented operational signals for the Registry challenge,
token decisions, Distribution notification delivery, and Artifact reconciliation.
The design preserves the control-plane/data-plane boundary: Distribution still owns
`/v2/` and its Bearer challenges, while HubCR observes its own token and notification
decisions without proxying OCI bytes.

## Signal ownership

| Signal | Owner | Implemented evidence |
| --- | --- | --- |
| `/v2/` Bearer challenge | Gateway and Distribution | Gateway JSON access log with `request_id`, `route="registry"`, response status, and `registry_challenge=true` for a `401` |
| Token request and action intersection | Go control plane | Structured decision logs and bounded Prometheus counters |
| Notification delivery queue and retries | Distribution | Localhost-only debug server at `/metrics` and `/debug/vars` |
| Notification acceptance and reconciliation failures | Go control plane | Structured request logs and bounded Prometheus counters |

The gateway log deliberately omits the request URI, query string, remote address,
user agent, cookies, and authorization headers. It therefore records challenge
behavior without copying token request parameters or credentials into access logs.
Distribution application logs use JSON formatting.

## Control-plane metrics

When Registry authentication is enabled, the Go API exposes Prometheus text format at
`GET /internal/metrics` on the direct control-plane listener. This internal operations
endpoint is not routed through the local public gateway and is not part of the
business REST or OpenAPI contract. Counters are process-local and reset when the API
restarts.

| Metric | Labels | Meaning |
| --- | --- | --- |
| `hubcr_registry_token_requests_total` | `outcome` | Token requests classified as `issued`, `invalid`, `unauthorized`, `unavailable`, or `error` |
| `hubcr_registry_token_actions_total` | `action`, `decision` | Per-scope `pull`, `push`, and `delete` actions classified as `granted` or `denied` after policy intersection |
| `hubcr_registry_notification_requests_total` | `outcome` | Notification HTTP requests classified as `accepted`, `unauthorized`, `invalid`, `conflict`, `unavailable`, or `error` |
| `hubcr_registry_notification_events_total` | `outcome` | Accepted envelope events classified as `processed` or `ignored` |
| `hubcr_registry_reconciliation_failures_total` | `class` | Processor failures classified as `invalid`, `conflict`, `unavailable`, or `unknown` |

All label values come from fixed code-defined sets. Repository names, Digests, user
identifiers, request IDs, and error strings never become metric labels, preventing
unbounded cardinality and sensitive-data leakage.

## Structured control-plane logs

Successful token decisions include `request_id`, `outcome`, `anonymous`, bounded
scope/action counts, signing `kid`, and HTTP status. Failures include `request_id`, a
bounded outcome and error class, and status. Logs do not contain the Basic username or
password, raw scope/repository, subject identifier, authorization header, cookie, or
signed token.

Accepted notifications include `request_id`, `outcome`, envelope event count,
processed/ignored counts, and status. Rejected notifications include the correlated
request ID, bounded outcome/error class, and status. Authorization tokens and event
payloads are never logged. A generated request ID is returned even when Distribution
does not supply one, so every delivery attempt remains independently traceable.

## Distribution debug visibility

The local Compose stack enables Distribution's debug listener inside the container on
port `5001` and binds it to host loopback at
`127.0.0.1:${HUBCR_REGISTRY_DEBUG_PORT:-5002}`. `/metrics` exposes Distribution's
Prometheus metrics, including notification statistics, while `/debug/vars` exposes
endpoint queue state needed to investigate retry backlog and failures.

Distribution documents that debug endpoints may contain sensitive operational data.
The checked-in Compose mapping is therefore loopback-only. A later production target
must keep both the Go and Distribution operations endpoints on a protected internal
network or add operator authentication; neither endpoint may be exposed through the
public OCI gateway by default.

## Acceptance evidence

Focused tests prove metric exposition, bounded labels, policy grant/denial counting,
request correlation, reconciliation-failure classification, and secret-safe logs.
The real Docker/OCI suite proves the same signals while performing successful and
denied push/pull flows through Distribution 3.1.1:

```bash
go -C backend test ./internal/platform/observability \
  ./internal/platform/httpapi/registryhandler \
  ./internal/platform/httpapi/registryeventhandler
HUBCR_ENV_FILE=.env.example make infra-config
make test-m2-registry-e2e
make check
git diff --check
```

The runtime suite also checks that the gateway challenge log, control-plane logs,
control-plane counters, Distribution Prometheus endpoint, and notification queue
variables are present, and that no tested password or Bearer token appears in the
gateway or API logs.
