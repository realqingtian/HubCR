package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

const RegistryMetricsPath = "/internal/metrics"

type TokenOutcome uint8

const (
	TokenIssued TokenOutcome = iota
	TokenInvalid
	TokenUnauthorized
	TokenUnavailable
	TokenError
)

type NotificationOutcome uint8

const (
	NotificationAccepted NotificationOutcome = iota
	NotificationUnauthorized
	NotificationInvalid
	NotificationConflict
	NotificationUnavailable
	NotificationError
)

type ReconciliationFailure uint8

const (
	ReconciliationInvalid ReconciliationFailure = iota
	ReconciliationConflict
	ReconciliationUnavailable
	ReconciliationUnknown
)

type ActionCounts struct {
	Pull   uint64
	Push   uint64
	Delete uint64
}

type RegistryMetrics struct {
	tokenRequests          [5]atomic.Uint64
	tokenGrantedActions    [3]atomic.Uint64
	tokenDeniedActions     [3]atomic.Uint64
	notificationRequests   [6]atomic.Uint64
	notificationEvents     [2]atomic.Uint64
	reconciliationFailures [4]atomic.Uint64
}

func NewRegistryMetrics() *RegistryMetrics {
	return &RegistryMetrics{}
}

func (m *RegistryMetrics) ObserveToken(
	outcome TokenOutcome,
	granted ActionCounts,
	denied ActionCounts,
) {
	if m == nil || outcome > TokenError {
		return
	}
	m.tokenRequests[outcome].Add(1)
	m.tokenGrantedActions[0].Add(granted.Pull)
	m.tokenGrantedActions[1].Add(granted.Push)
	m.tokenGrantedActions[2].Add(granted.Delete)
	m.tokenDeniedActions[0].Add(denied.Pull)
	m.tokenDeniedActions[1].Add(denied.Push)
	m.tokenDeniedActions[2].Add(denied.Delete)
}

func (m *RegistryMetrics) ObserveNotification(
	outcome NotificationOutcome,
	processed uint64,
	ignored uint64,
) {
	if m == nil || outcome > NotificationError {
		return
	}
	m.notificationRequests[outcome].Add(1)
	m.notificationEvents[0].Add(processed)
	m.notificationEvents[1].Add(ignored)
}

func (m *RegistryMetrics) ObserveReconciliationFailure(class ReconciliationFailure) {
	if m == nil || class > ReconciliationUnknown {
		return
	}
	m.reconciliationFailures[class].Add(1)
}

func (m *RegistryMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		m.writePrometheus(w)
	})
}

func (m *RegistryMetrics) writePrometheus(w http.ResponseWriter) {
	_, _ = fmt.Fprintln(w, "# HELP hubcr_registry_token_requests_total Registry token requests by bounded outcome.")
	_, _ = fmt.Fprintln(w, "# TYPE hubcr_registry_token_requests_total counter")
	for index, outcome := range []string{"issued", "invalid", "unauthorized", "unavailable", "error"} {
		_, _ = fmt.Fprintf(w, "hubcr_registry_token_requests_total{outcome=%q} %d\n", outcome, m.tokenRequests[index].Load())
	}
	_, _ = fmt.Fprintln(w, "# HELP hubcr_registry_token_actions_total Registry token action decisions by action and decision.")
	_, _ = fmt.Fprintln(w, "# TYPE hubcr_registry_token_actions_total counter")
	for index, action := range []string{"pull", "push", "delete"} {
		_, _ = fmt.Fprintf(w, "hubcr_registry_token_actions_total{action=%q,decision=%q} %d\n", action, "granted", m.tokenGrantedActions[index].Load())
		_, _ = fmt.Fprintf(w, "hubcr_registry_token_actions_total{action=%q,decision=%q} %d\n", action, "denied", m.tokenDeniedActions[index].Load())
	}
	_, _ = fmt.Fprintln(w, "# HELP hubcr_registry_notification_requests_total Distribution notification requests by bounded outcome.")
	_, _ = fmt.Fprintln(w, "# TYPE hubcr_registry_notification_requests_total counter")
	for index, outcome := range []string{"accepted", "unauthorized", "invalid", "conflict", "unavailable", "error"} {
		_, _ = fmt.Fprintf(w, "hubcr_registry_notification_requests_total{outcome=%q} %d\n", outcome, m.notificationRequests[index].Load())
	}
	_, _ = fmt.Fprintln(w, "# HELP hubcr_registry_notification_events_total Distribution notification events by processing result.")
	_, _ = fmt.Fprintln(w, "# TYPE hubcr_registry_notification_events_total counter")
	for index, outcome := range []string{"processed", "ignored"} {
		_, _ = fmt.Fprintf(w, "hubcr_registry_notification_events_total{outcome=%q} %d\n", outcome, m.notificationEvents[index].Load())
	}
	_, _ = fmt.Fprintln(w, "# HELP hubcr_registry_reconciliation_failures_total Artifact reconciliation failures by bounded class.")
	_, _ = fmt.Fprintln(w, "# TYPE hubcr_registry_reconciliation_failures_total counter")
	for index, class := range []string{"invalid", "conflict", "unavailable", "unknown"} {
		_, _ = fmt.Fprintf(w, "hubcr_registry_reconciliation_failures_total{class=%q} %d\n", class, m.reconciliationFailures[index].Load())
	}
}
