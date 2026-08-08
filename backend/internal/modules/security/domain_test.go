package security

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
)

const securityTestDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestTargetPayloadAndIntentAreDigestBound(t *testing.T) {
	target, err := NewTarget(
		"11111111-1111-4111-8111-111111111111", "team", "image", securityTestDigest,
	)
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	payload, err := MarshalJobPayload(target)
	if err != nil {
		t.Fatalf("MarshalJobPayload() error = %v", err)
	}
	decoded, err := ParseJobPayload(payload)
	if err != nil || decoded != target {
		t.Fatalf("ParseJobPayload() = %#v, %v", decoded, err)
	}
	scanKey, err := IntentKey(ScanJobKind, target)
	if err != nil {
		t.Fatalf("IntentKey(scan) error = %v", err)
	}
	sbomKey, err := IntentKey(SBOMJobKind, target)
	if err != nil || scanKey == sbomKey {
		t.Fatalf("IntentKey(SBOM) = %q, %v; scan %q", sbomKey, err, scanKey)
	}
	if _, err := ParseJobPayload(append(payload, []byte(` {}`)...)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ParseJobPayload(trailing) error = %v, want ErrInvalid", err)
	}
	unknown := json.RawMessage(`{"workflow_version":"v1","repository_id":"11111111-1111-4111-8111-111111111111","namespace":"team","repository":"image","digest":"` + securityTestDigest + `","secret":"no"}`)
	if _, err := ParseJobPayload(unknown); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ParseJobPayload(unknown) error = %v, want ErrInvalid", err)
	}
}

func TestResultStatusTruthfullyMapsJobLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	base := jobs.Job{
		ID: "job", Kind: jobs.Kind(ScanJobKind), IntentKey: "intent", Payload: json.RawMessage(`{}`),
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	tests := []struct {
		name          string
		job           jobs.Job
		result, stale bool
		want          ResultState
		code          string
	}{
		{name: "queued", job: withJobState(base, jobs.StateQueued, now), want: ResultQueued},
		{name: "running", job: withJobState(base, jobs.StateRunning, now), want: ResultRunning},
		{name: "completed", job: withJobState(base, jobs.StateSucceeded, now), result: true, want: ResultCompleted},
		{name: "stale", job: withJobState(base, jobs.StateSucceeded, now), result: true, stale: true, want: ResultStale},
		{name: "missing evidence", job: withJobState(base, jobs.StateSucceeded, now), want: ResultFailed, code: "RESULT_MISSING"},
		{name: "dead", job: withJobState(base, jobs.StateDead, now), want: ResultFailed, code: "SCANNER_UNAVAILABLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, err := ResultStatusFromJob(test.job, test.result, test.stale)
			if err != nil {
				t.Fatalf("ResultStatusFromJob() error = %v", err)
			}
			if status.State != test.want || status.ErrorCode != test.code {
				t.Fatalf("status = %#v, want state %s code %q", status, test.want, test.code)
			}
		})
	}
}

func withJobState(base jobs.Job, state jobs.State, now time.Time) jobs.Job {
	job := base
	job.State = state
	switch state {
	case jobs.StateRunning:
		job.Attempts = 1
		job.LeaseOwner = "worker"
		expires := now.Add(time.Minute)
		job.LeaseExpiresAt = &expires
		job.StartedAt = &now
	case jobs.StateSucceeded:
		job.Attempts = 1
		job.StartedAt = &now
		job.CompletedAt = &now
	case jobs.StateDead:
		job.Attempts = 3
		job.StartedAt = &now
		job.CompletedAt = &now
		code := jobs.ErrorCode("SCANNER_UNAVAILABLE")
		job.LastErrorCode = &code
	}
	return job
}

func TestScanAndSBOMEvidenceRejectMissingTruth(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	target, _ := NewTarget("repository", "team", "image", securityTestDigest)
	version := ToolVersion{
		ScannerVersion: "0.72.0", DatabaseSchemaVersion: 2,
		DatabaseUpdatedAt: now.Add(-time.Hour), DatabaseDownloadedAt: now.Add(-time.Minute),
		ObservedAt: now,
	}
	if err := (ScanResult{Target: target, ToolVersion: version, Findings: []Finding{}, CompletedAt: now}).Validate(); err != nil {
		t.Fatalf("clean ScanResult.Validate() error = %v", err)
	}
	validSBOM := SBOMResult{
		Target: target, GeneratorVersion: "0.72.0", Format: CycloneDXFormat,
		Document: json.RawMessage(`{"bomFormat":"CycloneDX"}`), CompletedAt: now,
	}
	if err := validSBOM.Validate(); err != nil {
		t.Fatalf("SBOMResult.Validate() error = %v", err)
	}
	validSBOM.Document = json.RawMessage(`{"components":[]}`)
	if err := validSBOM.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("SBOMResult.Validate(missing format) error = %v, want ErrInvalid", err)
	}
}
