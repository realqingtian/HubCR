package security

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
)

func TestHandlersPersistScanAndSBOMWithoutPassingTokenInState(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	target, _ := NewTarget("repository-id", "team", "image", securityTestDigest)
	workflow := Workflow{
		ID: "workflow", Target: target, ScanJobID: "scan-job", SBOMJobID: "sbom-job",
		CreatedAt: now, UpdatedAt: now,
	}
	service := &handlerService{workflow: workflow, target: target}
	scanner := &handlerScanner{version: ToolVersion{
		ScannerVersion: "0.72.0", DatabaseSchemaVersion: 2,
		DatabaseUpdatedAt: now.Add(-time.Hour), DatabaseDownloadedAt: now.Add(-time.Minute),
		ObservedAt: now,
	}, sbom: json.RawMessage(`{"bomFormat":"CycloneDX"}`)}
	tokens := &handlerTokens{token: "short-lived-secret"}
	handlers, err := NewHandlers(
		service, scanner, tokens, HandlerOptions{RegistryHost: "registry:5000", Clock: func() time.Time { return now }},
	)
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	registered := handlers.JobHandlers()
	for _, test := range []struct {
		kind jobs.Kind
		id   string
	}{
		{jobs.Kind(ScanJobKind), workflow.ScanJobID},
		{jobs.Kind(SBOMJobKind), workflow.SBOMJobID},
	} {
		payload, _ := MarshalJobPayload(target)
		job := jobs.Job{ID: test.id, Kind: test.kind, Payload: payload}
		if err := registered[test.kind].Handle(context.Background(), job); err != nil {
			t.Fatalf("Handle(%s) error = %v", test.kind, err)
		}
	}
	if scanner.reference != "registry:5000/team/image@"+securityTestDigest ||
		scanner.token != "short-lived-secret" || tokens.repository != "team/image" {
		t.Fatalf("scanner reference/token = %q/%q; repository %q", scanner.reference, scanner.token, tokens.repository)
	}
	if service.scan.Target != target || service.sbom.Target != target ||
		string(service.sbom.Document) == "short-lived-secret" {
		t.Fatalf("persisted results = %#v / %#v", service.scan, service.sbom)
	}
}

func TestHandlerClassifiesInvalidScannerOutputPermanently(t *testing.T) {
	now := time.Now().UTC()
	target, _ := NewTarget("repository-id", "team", "image", securityTestDigest)
	workflow := Workflow{ID: "workflow", Target: target, ScanJobID: "scan", SBOMJobID: "sbom", CreatedAt: now, UpdatedAt: now}
	handlers, _ := NewHandlers(
		&handlerService{workflow: workflow, target: target},
		&handlerScanner{err: ErrInvalidOutput}, &handlerTokens{token: "token"},
		HandlerOptions{RegistryHost: "registry:5000", Clock: func() time.Time { return now }},
	)
	payload, _ := MarshalJobPayload(target)
	err := handlers.JobHandlers()[jobs.Kind(ScanJobKind)].Handle(
		context.Background(), jobs.Job{ID: "scan", Kind: jobs.Kind(ScanJobKind), Payload: payload},
	)
	code, terminal := jobs.ClassifyHandlerError(err)
	if code != "SCANNER_OUTPUT_INVALID" || !terminal {
		t.Fatalf("classified error = %s terminal=%t", code, terminal)
	}
}

type handlerService struct {
	workflow Workflow
	target   Target
	scan     ScanResult
	sbom     SBOMResult
	err      error
}

func (s *handlerService) ResolveJob(context.Context, jobs.Job) (Workflow, Target, error) {
	return s.workflow, s.target, s.err
}

func (s *handlerService) SaveScanResult(_ context.Context, _ Workflow, result ScanResult) error {
	s.scan = result
	return s.err
}

func (s *handlerService) SaveSBOMResult(_ context.Context, _ Workflow, result SBOMResult) error {
	s.sbom = result
	return s.err
}

type handlerScanner struct {
	version   ToolVersion
	findings  []Finding
	sbom      json.RawMessage
	reference string
	token     string
	err       error
}

func (s *handlerScanner) Scan(_ context.Context, reference, token string) (ToolVersion, []Finding, error) {
	s.reference, s.token = reference, token
	return s.version, s.findings, s.err
}

func (s *handlerScanner) GenerateSBOM(_ context.Context, reference, token string) (string, json.RawMessage, error) {
	s.reference, s.token = reference, token
	return "0.72.0", s.sbom, s.err
}

type handlerTokens struct {
	token      string
	repository string
	err        error
}

func (t *handlerTokens) IssuePull(_ context.Context, repository string) (string, error) {
	t.repository = repository
	return t.token, t.err
}
