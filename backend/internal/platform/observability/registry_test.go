package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryMetricsPrometheusContract(t *testing.T) {
	metrics := NewRegistryMetrics()
	metrics.ObserveToken(
		TokenIssued,
		ActionCounts{Pull: 2, Push: 1},
		ActionCounts{Push: 3, Delete: 1},
	)
	metrics.ObserveToken(TokenUnauthorized, ActionCounts{}, ActionCounts{})
	metrics.ObserveNotification(NotificationAccepted, 4, 2)
	metrics.ObserveNotification(NotificationUnavailable, 0, 0)
	metrics.ObserveReconciliationFailure(ReconciliationUnavailable)

	request := httptest.NewRequest(http.MethodGet, RegistryMetricsPath, nil)
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	for _, line := range []string{
		`hubcr_registry_token_requests_total{outcome="issued"} 1`,
		`hubcr_registry_token_requests_total{outcome="unauthorized"} 1`,
		`hubcr_registry_token_actions_total{action="pull",decision="granted"} 2`,
		`hubcr_registry_token_actions_total{action="push",decision="denied"} 3`,
		`hubcr_registry_notification_requests_total{outcome="accepted"} 1`,
		`hubcr_registry_notification_events_total{outcome="processed"} 4`,
		`hubcr_registry_notification_events_total{outcome="ignored"} 2`,
		`hubcr_registry_reconciliation_failures_total{class="unavailable"} 1`,
	} {
		if !strings.Contains(recorder.Body.String(), line+"\n") {
			t.Fatalf("metrics omitted %q:\n%s", line, recorder.Body.String())
		}
	}
}

func TestRegistryMetricsIgnoreInvalidEnumValues(t *testing.T) {
	metrics := NewRegistryMetrics()
	metrics.ObserveToken(TokenOutcome(255), ActionCounts{Pull: 1}, ActionCounts{})
	metrics.ObserveNotification(NotificationOutcome(255), 1, 1)
	metrics.ObserveReconciliationFailure(ReconciliationFailure(255))

	request := httptest.NewRequest(http.MethodGet, RegistryMetricsPath, nil)
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, request)
	for _, unexpected := range []string{
		`hubcr_registry_token_actions_total{action="pull",decision="granted"} 1`,
		`hubcr_registry_notification_events_total{outcome="processed"} 1`,
		`hubcr_registry_reconciliation_failures_total{class="unknown"} 1`,
	} {
		if strings.Contains(recorder.Body.String(), unexpected) {
			t.Fatalf("invalid observation changed metrics: %s", recorder.Body.String())
		}
	}
}
