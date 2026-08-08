package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/jobs"
)

const (
	ScanJobKind      = "TRIVY_SCAN"
	SBOMJobKind      = "TRIVY_SBOM"
	WorkflowVersion  = "v1"
	DefaultAttempts  = 3
	MaxRepairBatch   = 500
	MaxFindingText   = 4096
	MaxSBOMBytes     = 16 * 1024 * 1024
	CycloneDXFormat  = "CYCLONEDX_JSON"
	ScannerNameTrivy = "TRIVY"
)

var (
	ErrInvalid       = errors.New("invalid security workflow")
	ErrNotFound      = errors.New("security workflow not found")
	ErrConflict      = errors.New("security workflow conflicts with persisted state")
	ErrUnavailable   = errors.New("security workflow persistence is unavailable")
	ErrToolFailure   = errors.New("security tool execution failed")
	ErrInvalidOutput = errors.New("security tool returned invalid output")

	namePattern    = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$`)
)

type Target struct {
	RepositoryID string
	Namespace    string
	Repository   string
	Digest       artifacts.Digest
}

func NewTarget(repositoryID, namespace, repository, rawDigest string) (Target, error) {
	digest, err := artifacts.ParseDigest(rawDigest)
	if err != nil || repositoryID == "" || !validName(namespace) || !validName(repository) {
		return Target{}, ErrInvalid
	}
	return Target{
		RepositoryID: repositoryID,
		Namespace:    namespace,
		Repository:   repository,
		Digest:       digest,
	}, nil
}

func (t Target) RepositoryPath() string { return t.Namespace + "/" + t.Repository }

func (t Target) ImageReference(registryOrigin string) (string, error) {
	if _, err := NewTarget(t.RepositoryID, t.Namespace, t.Repository, t.Digest.String()); err != nil {
		return "", err
	}
	origin := strings.TrimSuffix(registryOrigin, "/")
	if origin == "" || strings.ContainsAny(origin, "?#@") {
		return "", ErrInvalid
	}
	return origin + "/" + t.RepositoryPath() + "@" + t.Digest.String(), nil
}

type JobPayload struct {
	WorkflowVersion string `json:"workflow_version"`
	RepositoryID    string `json:"repository_id"`
	Namespace       string `json:"namespace"`
	Repository      string `json:"repository"`
	Digest          string `json:"digest"`
}

func NewJobPayload(target Target) (JobPayload, error) {
	validated, err := NewTarget(
		target.RepositoryID, target.Namespace, target.Repository, target.Digest.String(),
	)
	if err != nil {
		return JobPayload{}, err
	}
	return JobPayload{
		WorkflowVersion: WorkflowVersion,
		RepositoryID:    validated.RepositoryID,
		Namespace:       validated.Namespace,
		Repository:      validated.Repository,
		Digest:          validated.Digest.String(),
	}, nil
}

func ParseJobPayload(raw json.RawMessage) (Target, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload JobPayload
	if err := decoder.Decode(&payload); err != nil {
		return Target{}, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Target{}, ErrInvalid
	}
	if payload.WorkflowVersion != WorkflowVersion {
		return Target{}, ErrInvalid
	}
	return NewTarget(payload.RepositoryID, payload.Namespace, payload.Repository, payload.Digest)
}

func IntentKey(kind string, target Target) (string, error) {
	if kind != ScanJobKind && kind != SBOMJobKind {
		return "", ErrInvalid
	}
	if _, err := NewTarget(
		target.RepositoryID, target.Namespace, target.Repository, target.Digest.String(),
	); err != nil {
		return "", err
	}
	prefix := "security-scan"
	if kind == SBOMJobKind {
		prefix = "security-sbom"
	}
	return fmt.Sprintf("%s:%s:%s", prefix, target.RepositoryID, target.Digest), nil
}

type Workflow struct {
	ID        string
	Target    Target
	ScanJobID string
	SBOMJobID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (w Workflow) Validate() error {
	if w.ID == "" || w.ScanJobID == "" || w.SBOMJobID == "" ||
		w.ScanJobID == w.SBOMJobID || w.CreatedAt.IsZero() ||
		w.UpdatedAt.Before(w.CreatedAt) {
		return ErrInvalid
	}
	_, err := NewTarget(
		w.Target.RepositoryID, w.Target.Namespace, w.Target.Repository, w.Target.Digest.String(),
	)
	return err
}

type ToolVersion struct {
	ScannerVersion        string
	DatabaseSchemaVersion int
	DatabaseUpdatedAt     time.Time
	DatabaseDownloadedAt  time.Time
	ObservedAt            time.Time
}

func (v ToolVersion) Validate() error {
	if !versionPattern.MatchString(v.ScannerVersion) || v.DatabaseSchemaVersion < 1 ||
		v.DatabaseUpdatedAt.IsZero() || v.DatabaseDownloadedAt.IsZero() ||
		v.ObservedAt.IsZero() || v.DatabaseDownloadedAt.Before(v.DatabaseUpdatedAt) {
		return ErrInvalid
	}
	return nil
}

type Finding struct {
	Target           string
	Class            string
	Type             string
	VulnerabilityID  string
	PackageName      string
	InstalledVersion string
	FixedVersion     string
	Status           string
	Severity         string
	SeveritySource   string
	PrimaryURL       string
	DataSourceID     string
	Title            string
	PublishedAt      *time.Time
	ModifiedAt       *time.Time
}

func (f Finding) Validate() error {
	required := []string{f.Target, f.Class, f.Type, f.VulnerabilityID, f.PackageName, f.InstalledVersion, f.Severity}
	for _, value := range required {
		if value == "" || len(value) > MaxFindingText {
			return ErrInvalid
		}
	}
	optional := []string{f.FixedVersion, f.Status, f.SeveritySource, f.PrimaryURL, f.DataSourceID, f.Title}
	for _, value := range optional {
		if len(value) > MaxFindingText {
			return ErrInvalid
		}
	}
	switch f.Severity {
	case "UNKNOWN", "LOW", "MEDIUM", "HIGH", "CRITICAL":
	default:
		return ErrInvalid
	}
	if f.PublishedAt != nil && f.PublishedAt.IsZero() || f.ModifiedAt != nil && f.ModifiedAt.IsZero() {
		return ErrInvalid
	}
	return nil
}

type ScanResult struct {
	Target      Target
	ToolVersion ToolVersion
	Findings    []Finding
	CompletedAt time.Time
}

func (r ScanResult) Validate() error {
	if _, err := NewTarget(
		r.Target.RepositoryID, r.Target.Namespace, r.Target.Repository, r.Target.Digest.String(),
	); err != nil || r.ToolVersion.Validate() != nil || r.CompletedAt.IsZero() {
		return ErrInvalid
	}
	for _, finding := range r.Findings {
		if err := finding.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SBOMResult struct {
	Target           Target
	GeneratorVersion string
	Format           string
	Document         json.RawMessage
	CompletedAt      time.Time
}

func (r SBOMResult) Validate() error {
	if _, err := NewTarget(
		r.Target.RepositoryID, r.Target.Namespace, r.Target.Repository, r.Target.Digest.String(),
	); err != nil || !versionPattern.MatchString(r.GeneratorVersion) ||
		r.Format != CycloneDXFormat || r.CompletedAt.IsZero() ||
		len(r.Document) < 2 || len(r.Document) > MaxSBOMBytes || !json.Valid(r.Document) {
		return ErrInvalid
	}
	var object map[string]any
	if err := json.Unmarshal(r.Document, &object); err != nil || object["bomFormat"] != "CycloneDX" {
		return ErrInvalid
	}
	return nil
}

type ResultState string

const (
	ResultQueued    ResultState = "QUEUED"
	ResultRunning   ResultState = "RUNNING"
	ResultCompleted ResultState = "COMPLETED"
	ResultFailed    ResultState = "FAILED"
	ResultStale     ResultState = "STALE"
)

type ResultStatus struct {
	State     ResultState
	ErrorCode string
	Attempts  int
	UpdatedAt time.Time
}

func ResultStatusFromJob(job jobs.Job, resultPresent, stale bool) (ResultStatus, error) {
	if err := job.Validate(); err != nil {
		return ResultStatus{}, ErrInvalid
	}
	status := ResultStatus{Attempts: job.Attempts, UpdatedAt: job.UpdatedAt}
	switch job.State {
	case jobs.StateQueued:
		status.State = ResultQueued
	case jobs.StateRunning:
		status.State = ResultRunning
	case jobs.StateDead:
		status.State = ResultFailed
		if job.LastErrorCode != nil {
			status.ErrorCode = string(*job.LastErrorCode)
		}
	case jobs.StateSucceeded:
		if !resultPresent {
			status.State = ResultFailed
			status.ErrorCode = "RESULT_MISSING"
		} else if stale {
			status.State = ResultStale
		} else {
			status.State = ResultCompleted
		}
	default:
		return ResultStatus{}, ErrInvalid
	}
	return status, nil
}

type Detail struct {
	Workflow       Workflow
	Scan           ResultStatus
	SBOM           ResultStatus
	Signature      *VerificationDetail
	ToolVersion    *ToolVersion
	FindingCount   int
	SeverityCounts map[string]int
	SBOMFormat     string
	ScannedAt      *time.Time
	SBOMCreatedAt  *time.Time
}

func validName(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && namePattern.MatchString(value)
}
