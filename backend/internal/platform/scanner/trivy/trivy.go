package trivy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hubcr.io/hubcr/internal/modules/security"
)

const (
	maxScanOutputBytes = 32 * 1024 * 1024
	maxStderrBytes     = 64 * 1024
)

type Options struct {
	Binary   string
	CacheDir string
	Insecure bool
	Clock    func() time.Time
}

type Runner interface {
	Run(context.Context, string, []string, []string, int) ([]byte, error)
}

type Scanner struct {
	options Options
	runner  Runner
	mutex   sync.Mutex
}

func New(options Options) (*Scanner, error) {
	return NewWithRunner(options, commandRunner{})
}

func NewWithRunner(options Options, runner Runner) (*Scanner, error) {
	if runner == nil || options.Clock == nil || options.Binary == "" ||
		strings.TrimSpace(options.Binary) != options.Binary ||
		options.CacheDir == "" || !filepath.IsAbs(options.CacheDir) {
		return nil, errors.New("Trivy dependencies and options must be configured")
	}
	return &Scanner{options: options, runner: runner}, nil
}

func (s *Scanner) Scan(
	ctx context.Context,
	reference string,
	token string,
) (security.ToolVersion, []security.Finding, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !validReference(reference) || token == "" {
		return security.ToolVersion{}, nil, security.ErrInvalid
	}
	if err := s.updateDatabase(ctx); err != nil {
		return security.ToolVersion{}, nil, err
	}
	args := s.imageArguments("json", reference)
	output, err := s.runner.Run(
		ctx, s.options.Binary, args, registryEnvironment(token), maxScanOutputBytes,
	)
	if err != nil {
		return security.ToolVersion{}, nil, fmt.Errorf("%w: scan image", security.ErrToolFailure)
	}
	findings, err := parseScan(output)
	if err != nil {
		return security.ToolVersion{}, nil, err
	}
	version, err := s.version(ctx)
	if err != nil {
		return security.ToolVersion{}, nil, err
	}
	return version, findings, nil
}

func (s *Scanner) GenerateSBOM(
	ctx context.Context,
	reference string,
	token string,
) (string, json.RawMessage, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !validReference(reference) || token == "" {
		return "", nil, security.ErrInvalid
	}
	output, err := s.runner.Run(
		ctx, s.options.Binary, s.imageArguments("cyclonedx", reference),
		registryEnvironment(token), security.MaxSBOMBytes,
	)
	if err != nil {
		return "", nil, fmt.Errorf("%w: generate SBOM", security.ErrToolFailure)
	}
	version, document, err := parseSBOM(output)
	if err != nil {
		return "", nil, err
	}
	return version, document, nil
}

func (s *Scanner) updateDatabase(ctx context.Context) error {
	_, err := s.runner.Run(
		ctx, s.options.Binary,
		[]string{"image", "--quiet", "--cache-dir", s.options.CacheDir, "--download-db-only"},
		nil, maxStderrBytes,
	)
	if err != nil {
		return fmt.Errorf("%w: update vulnerability database", security.ErrToolFailure)
	}
	return nil
}

func (s *Scanner) imageArguments(format, reference string) []string {
	args := []string{
		"image", "--quiet", "--cache-dir", s.options.CacheDir,
		"--image-src", "remote", "--format", format,
	}
	if format == "json" {
		args = append(args, "--scanners", "vuln", "--skip-db-update")
	}
	if s.options.Insecure {
		args = append(args, "--insecure")
	}
	return append(args, reference)
}

func (s *Scanner) version(ctx context.Context) (security.ToolVersion, error) {
	output, err := s.runner.Run(
		ctx, s.options.Binary,
		[]string{"version", "--cache-dir", s.options.CacheDir, "--format", "json"},
		nil, maxStderrBytes,
	)
	if err != nil {
		return security.ToolVersion{}, fmt.Errorf("%w: read scanner version", security.ErrToolFailure)
	}
	var value struct {
		Version         string `json:"Version"`
		VulnerabilityDB *struct {
			Version      int       `json:"Version"`
			UpdatedAt    time.Time `json:"UpdatedAt"`
			DownloadedAt time.Time `json:"DownloadedAt"`
		} `json:"VulnerabilityDB"`
	}
	if err := decodeOne(output, &value); err != nil || value.VulnerabilityDB == nil {
		return security.ToolVersion{}, security.ErrInvalidOutput
	}
	result := security.ToolVersion{
		ScannerVersion: value.Version, DatabaseSchemaVersion: value.VulnerabilityDB.Version,
		DatabaseUpdatedAt:    value.VulnerabilityDB.UpdatedAt.UTC(),
		DatabaseDownloadedAt: value.VulnerabilityDB.DownloadedAt.UTC(),
		ObservedAt:           s.options.Clock().UTC().Round(time.Microsecond),
	}
	if err := result.Validate(); err != nil {
		return security.ToolVersion{}, security.ErrInvalidOutput
	}
	return result, nil
}

type scanDocument struct {
	SchemaVersion int    `json:"SchemaVersion"`
	ArtifactID    string `json:"ArtifactID"`
	ArtifactName  string `json:"ArtifactName"`
	ArtifactType  string `json:"ArtifactType"`
	Results       []struct {
		Target          string `json:"Target"`
		Class           string `json:"Class"`
		Type            string `json:"Type"`
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			Status           string `json:"Status"`
			Severity         string `json:"Severity"`
			SeveritySource   string `json:"SeveritySource"`
			PrimaryURL       string `json:"PrimaryURL"`
			DataSource       struct {
				ID string `json:"ID"`
			} `json:"DataSource"`
			Title            string     `json:"Title"`
			PublishedDate    *time.Time `json:"PublishedDate"`
			LastModifiedDate *time.Time `json:"LastModifiedDate"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func parseScan(raw []byte) ([]security.Finding, error) {
	var document scanDocument
	if err := decodeOne(raw, &document); err != nil || document.SchemaVersion != 2 ||
		document.ArtifactID == "" || document.ArtifactName == "" || document.ArtifactType != "container_image" {
		return nil, security.ErrInvalidOutput
	}
	findings := make([]security.Finding, 0)
	for _, result := range document.Results {
		for _, vulnerability := range result.Vulnerabilities {
			finding := security.Finding{
				Target: result.Target, Class: result.Class, Type: result.Type,
				VulnerabilityID: vulnerability.VulnerabilityID, PackageName: vulnerability.PkgName,
				InstalledVersion: vulnerability.InstalledVersion,
				FixedVersion:     vulnerability.FixedVersion, Status: vulnerability.Status,
				Severity: vulnerability.Severity, SeveritySource: vulnerability.SeveritySource,
				PrimaryURL: vulnerability.PrimaryURL, DataSourceID: vulnerability.DataSource.ID,
				Title: vulnerability.Title, PublishedAt: cloneTime(vulnerability.PublishedDate),
				ModifiedAt: cloneTime(vulnerability.LastModifiedDate),
			}
			if err := finding.Validate(); err != nil {
				return nil, security.ErrInvalidOutput
			}
			findings = append(findings, finding)
		}
	}
	return findings, nil
}

func parseSBOM(raw []byte) (string, json.RawMessage, error) {
	var document struct {
		BOMFormat string `json:"bomFormat"`
		Metadata  struct {
			Tools struct {
				Components []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"components"`
			} `json:"tools"`
		} `json:"metadata"`
	}
	if err := decodeOne(raw, &document); err != nil || document.BOMFormat != "CycloneDX" {
		return "", nil, security.ErrInvalidOutput
	}
	version := ""
	for _, component := range document.Metadata.Tools.Components {
		if strings.EqualFold(component.Name, "trivy") {
			version = component.Version
			break
		}
	}
	var compact bytes.Buffer
	if version == "" || json.Compact(&compact, raw) != nil {
		return "", nil, security.ErrInvalidOutput
	}
	if len(compact.Bytes()) > security.MaxSBOMBytes {
		return "", nil, security.ErrInvalidOutput
	}
	return version, json.RawMessage(bytes.Clone(compact.Bytes())), nil
}

func decodeOne(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func registryEnvironment(token string) []string {
	const prefix = "TRIVY_REGISTRY_TOKEN="
	environment := os.Environ()
	filtered := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		if !strings.HasPrefix(value, prefix) {
			filtered = append(filtered, value)
		}
	}
	return append(filtered, prefix+token)
}

func validReference(value string) bool {
	return value != "" && len(value) <= 512 && !strings.ContainsAny(value, " \t\r\n") &&
		strings.Contains(value, "@sha256:")
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

type commandRunner struct{}

func (commandRunner) Run(
	ctx context.Context,
	binary string,
	arguments []string,
	environment []string,
	maxOutput int,
) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, arguments...)
	if environment != nil {
		command.Env = environment
	}
	stdout := &limitedBuffer{maximum: maxOutput}
	stderr := &limitedBuffer{maximum: maxStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, errors.New("Trivy command failed")
	}
	return bytes.Clone(stdout.Bytes()), nil
}

type limitedBuffer struct {
	buffer  bytes.Buffer
	maximum int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	remaining := b.maximum - b.buffer.Len()
	if remaining <= 0 {
		return 0, errors.New("Trivy output limit exceeded")
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		return remaining, errors.New("Trivy output limit exceeded")
	}
	return b.buffer.Write(value)
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }

var _ security.Scanner = (*Scanner)(nil)
