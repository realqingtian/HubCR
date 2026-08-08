package trivy

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/security"
)

func TestScannerParsesVersionedVulnerabilitiesAndKeepsTokenOutOfArguments(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	runner := &scannerRunner{responses: [][]byte{
		nil,
		[]byte(`{"SchemaVersion":2,"ArtifactID":"sha256:image","ArtifactName":"registry/image","ArtifactType":"container_image","Results":[{"Target":"alpine (alpine 3.12)","Class":"os-pkgs","Type":"alpine","Vulnerabilities":[{"VulnerabilityID":"CVE-2022-37434","PkgName":"zlib","InstalledVersion":"1.2.12-r0","FixedVersion":"1.2.12-r2","Status":"fixed","Severity":"CRITICAL","SeveritySource":"nvd","PrimaryURL":"https://example.test/cve","DataSource":{"ID":"alpine"},"Title":"overflow"}]}]}`),
		[]byte(`{"Version":"0.72.0","VulnerabilityDB":{"Version":2,"UpdatedAt":"2026-08-08T10:00:00Z","DownloadedAt":"2026-08-08T11:00:00Z"}}`),
	}}
	scanner, err := NewWithRunner(Options{
		Binary: "/usr/local/bin/trivy", CacheDir: "/var/lib/hubcr/trivy",
		Insecure: true, Clock: func() time.Time { return now },
	}, runner)
	if err != nil {
		t.Fatalf("NewWithRunner() error = %v", err)
	}
	version, findings, err := scanner.Scan(
		context.Background(), "registry:5000/team/image@sha256:"+strings.Repeat("a", 64), "secret-token",
	)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if version.ScannerVersion != "0.72.0" || version.DatabaseSchemaVersion != 2 ||
		len(findings) != 1 || findings[0].VulnerabilityID != "CVE-2022-37434" {
		t.Fatalf("Scan() = %#v, %#v", version, findings)
	}
	if len(runner.calls) != 3 || runner.calls[0].environment != nil ||
		!slices.Contains(runner.calls[1].arguments, "--insecure") ||
		!slices.Contains(runner.calls[1].arguments, "--skip-db-update") {
		t.Fatalf("runner calls = %#v", runner.calls)
	}
	for _, argument := range runner.calls[1].arguments {
		if strings.Contains(argument, "secret-token") {
			t.Fatalf("token leaked into argument %q", argument)
		}
	}
	if !slices.Contains(runner.calls[1].environment, "TRIVY_REGISTRY_TOKEN=secret-token") {
		t.Fatalf("token environment missing: %#v", runner.calls[1].environment)
	}
}

func TestScannerGeneratesBoundedCycloneDX(t *testing.T) {
	runner := &scannerRunner{responses: [][]byte{[]byte(`{"bomFormat":"CycloneDX","specVersion":"1.7","metadata":{"tools":{"components":[{"name":"trivy","version":"0.72.0"}]}},"components":[]}`)}}
	scanner, _ := NewWithRunner(Options{
		Binary: "trivy", CacheDir: "/tmp/trivy", Clock: time.Now,
	}, runner)
	version, document, err := scanner.GenerateSBOM(
		context.Background(), "registry/team/image@sha256:"+strings.Repeat("a", 64), "token",
	)
	if err != nil || version != "0.72.0" || !json.Valid(document) {
		t.Fatalf("GenerateSBOM() = %q, %s, %v", version, document, err)
	}
	if slices.Contains(runner.calls[0].arguments, "--scanners") {
		t.Fatalf("SBOM arguments unexpectedly force vulnerability scanner: %#v", runner.calls[0].arguments)
	}
}

func TestScannerAcceptsEvidenceBackedCleanImage(t *testing.T) {
	now := time.Now().UTC()
	runner := &scannerRunner{responses: [][]byte{
		nil,
		[]byte(`{"SchemaVersion":2,"ArtifactID":"sha256:clean","ArtifactName":"registry/clean","ArtifactType":"container_image"}`),
		[]byte(`{"Version":"0.72.0","VulnerabilityDB":{"Version":2,"UpdatedAt":"2026-08-08T10:00:00Z","DownloadedAt":"2026-08-08T11:00:00Z"}}`),
	}}
	scanner, _ := NewWithRunner(Options{Binary: "trivy", CacheDir: "/tmp/trivy", Clock: func() time.Time { return now }}, runner)
	_, findings, err := scanner.Scan(
		context.Background(), "registry/team/clean@sha256:"+strings.Repeat("a", 64), "token",
	)
	if err != nil || len(findings) != 0 {
		t.Fatalf("Scan(clean) findings/error = %#v, %v", findings, err)
	}
}

func TestScannerRejectsUntrustedOutput(t *testing.T) {
	scanner, _ := NewWithRunner(Options{
		Binary: "trivy", CacheDir: "/tmp/trivy", Clock: time.Now,
	}, &scannerRunner{responses: [][]byte{nil, []byte(`{"SchemaVersion":1,"Results":[]}`)}})
	_, _, err := scanner.Scan(
		context.Background(), "registry/team/image@sha256:"+strings.Repeat("a", 64), "token",
	)
	if !errors.Is(err, security.ErrInvalidOutput) {
		t.Fatalf("Scan() error = %v, want ErrInvalidOutput", err)
	}
}

type scannerCall struct {
	arguments   []string
	environment []string
}

type scannerRunner struct {
	responses [][]byte
	calls     []scannerCall
}

func (r *scannerRunner) Run(
	_ context.Context,
	_ string,
	arguments []string,
	environment []string,
	_ int,
) ([]byte, error) {
	r.calls = append(r.calls, scannerCall{
		arguments: append([]string(nil), arguments...), environment: append([]string(nil), environment...),
	})
	if len(r.responses) == 0 {
		return nil, errors.New("missing response")
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}
