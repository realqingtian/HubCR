package securitystore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/jobstore"
	"hubcr.io/hubcr/migrations"
)

func TestStoreEnsuresRepairsAndPersistsDigestBoundEvidence(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 16,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)
	fixture := createFixture(t, ctx, pool, now)
	defer fixture.cleanup(t, ctx, pool)
	store := New(pool.ORM())
	target, err := security.NewTarget(
		fixture.repositoryID, fixture.namespace, fixture.repository, fixture.firstDigest,
	)
	if err != nil {
		t.Fatalf("security.NewTarget() error = %v", err)
	}

	var created atomic.Int32
	workflows := make(chan security.Workflow, 8)
	errorsChannel := make(chan error, 8)
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			workflow, wasCreated, err := store.EnsureWorkflow(ctx, target, now)
			if wasCreated {
				created.Add(1)
			}
			workflows <- workflow
			errorsChannel <- err
		}()
	}
	group.Wait()
	close(workflows)
	close(errorsChannel)
	var workflow security.Workflow
	for candidate := range workflows {
		if workflow.ID == "" {
			workflow = candidate
		} else if candidate.ID != workflow.ID || candidate.ScanJobID != workflow.ScanJobID ||
			candidate.SBOMJobID != workflow.SBOMJobID {
			t.Fatalf("concurrent workflows differ: %#v / %#v", workflow, candidate)
		}
	}
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("EnsureWorkflow() error = %v", err)
		}
	}
	if created.Load() != 1 {
		t.Fatalf("created workflows = %d, want 1", created.Load())
	}
	var jobCount int64
	if err := pool.ORM().WithContext(ctx).Table("jobs").
		Where("id IN ?", []string{workflow.ScanJobID, workflow.SBOMJobID}).Count(&jobCount).Error; err != nil || jobCount != 2 {
		t.Fatalf("workflow job count = %d, %v", jobCount, err)
	}

	repaired, err := store.RepairMissingWorkflows(ctx, 100, now.Add(time.Second))
	if err != nil || repaired < 1 {
		t.Fatalf("RepairMissingWorkflows() = %d, %v; want at least fixture gap", repaired, err)
	}
	secondTarget, _ := security.NewTarget(
		fixture.repositoryID, fixture.namespace, fixture.repository, fixture.secondDigest,
	)
	secondWorkflow, secondCreated, err := store.EnsureWorkflow(ctx, secondTarget, now.Add(time.Second))
	if err != nil || secondCreated {
		t.Fatalf("repaired EnsureWorkflow(second) = %#v, created=%t, %v", secondWorkflow, secondCreated, err)
	}
	repaired, err = store.RepairMissingWorkflows(ctx, 100, now.Add(2*time.Second))
	if err != nil || repaired != 0 {
		t.Fatalf("second RepairMissingWorkflows() = %d, %v; want 0", repaired, err)
	}

	scanJob, err := jobstore.New(pool.ORM()).ByID(ctx, workflow.ScanJobID)
	if err != nil {
		t.Fatalf("jobstore.ByID() error = %v", err)
	}
	resolvedWorkflow, resolvedTarget, err := store.ResolveJob(ctx, scanJob)
	if err != nil || resolvedWorkflow.ID != workflow.ID || resolvedTarget != target {
		t.Fatalf("ResolveJob() = %#v, %#v, %v", resolvedWorkflow, resolvedTarget, err)
	}

	version := security.ToolVersion{
		ScannerVersion: "0.72.0", DatabaseSchemaVersion: 2,
		DatabaseUpdatedAt: now.Add(-2 * time.Hour), DatabaseDownloadedAt: now.Add(-time.Hour),
		ObservedAt: now,
	}
	result := security.ScanResult{
		Target: target, ToolVersion: version, CompletedAt: now.Add(3 * time.Second),
		Findings: []security.Finding{{
			Target: "alpine", Class: "os-pkgs", Type: "alpine",
			VulnerabilityID: "CVE-2022-37434", PackageName: "zlib",
			InstalledVersion: "1.2.12-r0", FixedVersion: "1.2.12-r2",
			Status: "fixed", Severity: "CRITICAL", SeveritySource: "nvd",
		}},
	}
	if err := store.SaveScanResult(ctx, workflow, result); err != nil {
		t.Fatalf("SaveScanResult() error = %v", err)
	}
	if err := store.SaveSBOMResult(ctx, workflow, security.SBOMResult{
		Target: target, GeneratorVersion: "0.72.0", Format: security.CycloneDXFormat,
		Document:    json.RawMessage(`{"bomFormat":"CycloneDX","components":[]}`),
		CompletedAt: now.Add(4 * time.Second),
	}); err != nil {
		t.Fatalf("SaveSBOMResult() error = %v", err)
	}
	completeJob(t, ctx, pool, workflow.ScanJobID, now.Add(5*time.Second))
	completeJob(t, ctx, pool, workflow.SBOMJobID, now.Add(5*time.Second))
	detail, err := store.Detail(ctx, fixture.repositoryID, fixture.firstDigest)
	if err != nil {
		t.Fatalf("Detail() error = %v", err)
	}
	if detail.Scan.State != security.ResultCompleted || detail.SBOM.State != security.ResultCompleted ||
		detail.FindingCount != 1 || detail.SeverityCounts["CRITICAL"] != 1 ||
		detail.ToolVersion == nil || detail.ToolVersion.ScannerVersion != "0.72.0" {
		t.Fatalf("Detail() = %#v", detail)
	}

	newerVersion := version
	newerVersion.DatabaseUpdatedAt = now.Add(-30 * time.Minute)
	newerVersion.DatabaseDownloadedAt = now.Add(-10 * time.Minute)
	newerVersion.ObservedAt = now.Add(7 * time.Second)
	if err := store.SaveScanResult(ctx, secondWorkflow, security.ScanResult{
		Target: secondTarget, ToolVersion: newerVersion, Findings: []security.Finding{},
		CompletedAt: now.Add(7 * time.Second),
	}); err != nil {
		t.Fatalf("SaveScanResult(second) error = %v", err)
	}
	detail, err = store.Detail(ctx, fixture.repositoryID, fixture.firstDigest)
	if err != nil || detail.Scan.State != security.ResultStale {
		t.Fatalf("stale Detail() = %#v, %v", detail, err)
	}
}

func TestTrustStoreVersionsPoliciesRepairsJobsAndPreservesTrustEvidence(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 8,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	fixture := createFixture(t, ctx, pool, now)
	defer fixture.cleanup(t, ctx, pool)
	store := New(pool.ORM())
	oldKey := trustTestPublicKey(t, "old")
	currentKey := trustTestPublicKey(t, "current")
	oldPolicy, err := store.CreateTrustPolicy(
		ctx, fixture.namespaceID, fixture.userID, []security.PublicKeyTrust{oldKey}, nil, now,
	)
	if err != nil || oldPolicy.Version != 1 {
		t.Fatalf("CreateTrustPolicy(old) = %#v, %v", oldPolicy, err)
	}
	target, _ := security.NewTarget(
		fixture.repositoryID, fixture.namespace, fixture.repository, fixture.firstDigest,
	)
	oldWorkflow, created, err := store.EnsureCurrentVerification(ctx, target, now.Add(500*time.Millisecond))
	if err != nil || !created || oldWorkflow.PolicyID != oldPolicy.ID || oldWorkflow.PolicyVersion != 1 {
		t.Fatalf("EnsureCurrentVerification(old) = %#v, %t, %v", oldWorkflow, created, err)
	}
	oldEvidence, err := security.EvaluateTrust(oldPolicy, []security.CryptographicEvidence{
		trustTestEvidence(t, "a", security.SignerPublicKey, oldKey.Fingerprint, security.CryptoValid),
	})
	if err != nil {
		t.Fatalf("EvaluateTrust(old) error = %v", err)
	}
	if err := store.SaveVerificationResult(ctx, security.VerificationResult{
		Workflow: oldWorkflow, CosignVersion: "v3.0.6", Evidence: oldEvidence,
		CompletedAt: now.Add(750 * time.Millisecond),
	}); err != nil {
		t.Fatalf("SaveVerificationResult(old) error = %v", err)
	}
	completeJob(t, ctx, pool, oldWorkflow.JobID, now.Add(900*time.Millisecond))
	oldDetail, err := store.signatureDetail(ctx, fixture.repositoryID, fixture.firstDigest)
	if err != nil || oldDetail == nil || oldDetail.Status.State != security.ResultCompleted ||
		oldDetail.Workflow.PolicyVersion != 1 || len(oldDetail.Evidence) != 1 {
		t.Fatalf("signatureDetail(old) = %#v, %v", oldDetail, err)
	}
	identity, _ := security.NewKeylessIdentity("https://issuer.example", "release@example.com")
	currentPolicy, err := store.CreateTrustPolicy(
		ctx, fixture.namespaceID, fixture.userID,
		[]security.PublicKeyTrust{currentKey}, []security.KeylessIdentity{identity}, now.Add(time.Second),
	)
	if err != nil || currentPolicy.Version != 2 {
		t.Fatalf("CreateTrustPolicy(current) = %#v, %v", currentPolicy, err)
	}
	staleDetail, err := store.signatureDetail(ctx, fixture.repositoryID, fixture.firstDigest)
	if err != nil || staleDetail == nil || staleDetail.Status.State != security.ResultStale ||
		staleDetail.Workflow.PolicyVersion != 1 {
		t.Fatalf("signatureDetail(stale) = %#v, %v", staleDetail, err)
	}
	workflow, created, err := store.EnsureCurrentVerification(ctx, target, now.Add(2*time.Second))
	if err != nil || !created || workflow.PolicyID != currentPolicy.ID || workflow.PolicyVersion != 2 {
		t.Fatalf("EnsureCurrentVerification() = %#v, %t, %v", workflow, created, err)
	}
	repaired, err := store.RepairMissingVerificationWorkflows(ctx, 100, now.Add(3*time.Second))
	if err != nil || repaired != 1 {
		t.Fatalf("RepairMissingVerificationWorkflows() = %d, %v; want 1", repaired, err)
	}
	job, err := jobstore.New(pool.ORM()).ByID(ctx, workflow.JobID)
	if err != nil {
		t.Fatalf("jobstore.ByID() error = %v", err)
	}
	input, err := store.ResolveVerificationJob(ctx, job)
	if err != nil || input.Policy.ID != currentPolicy.ID || len(input.CandidateKeys) != 2 ||
		len(input.CandidateIdentities) != 1 {
		t.Fatalf("ResolveVerificationJob() = %#v, %v", input, err)
	}
	evidence, err := security.EvaluateTrust(currentPolicy, []security.CryptographicEvidence{
		trustTestEvidence(t, "c", security.SignerPublicKey, currentKey.Fingerprint, security.CryptoValid),
		trustTestEvidence(t, "d", security.SignerPublicKey, oldKey.Fingerprint, security.CryptoValid),
		trustTestEvidence(t, "e", security.SignerUnknown, "", security.CryptoUnverified),
	})
	if err != nil {
		t.Fatalf("EvaluateTrust() error = %v", err)
	}
	result := security.VerificationResult{
		Workflow: workflow, CosignVersion: "v3.0.6", Evidence: evidence,
		CompletedAt: now.Add(4 * time.Second),
	}
	if err := store.SaveVerificationResult(ctx, result); err != nil {
		t.Fatalf("SaveVerificationResult() error = %v", err)
	}
	completeJob(t, ctx, pool, workflow.JobID, now.Add(5*time.Second))
	currentDetail, err := store.signatureDetail(ctx, fixture.repositoryID, fixture.firstDigest)
	if err != nil || currentDetail == nil || currentDetail.Status.State != security.ResultCompleted ||
		currentDetail.Workflow.PolicyVersion != 2 || currentDetail.CosignVersion != "v3.0.6" ||
		len(currentDetail.Evidence) != 3 {
		t.Fatalf("signatureDetail(current) = %#v, %v", currentDetail, err)
	}
	var counts struct {
		Evidence  int64
		Trusted   int64
		Untrusted int64
	}
	if err := pool.ORM().WithContext(ctx).Table("signature_evidence").
		Where("workflow_id = ?", workflow.ID).Count(&counts.Evidence).Error; err != nil {
		t.Fatalf("count signature evidence: %v", err)
	}
	if err := pool.ORM().WithContext(ctx).Table("signature_evidence").
		Where("workflow_id = ? AND trust_state = 'TRUSTED'", workflow.ID).
		Count(&counts.Trusted).Error; err != nil {
		t.Fatalf("count trusted signature evidence: %v", err)
	}
	if err := pool.ORM().WithContext(ctx).Table("signature_evidence").
		Where("workflow_id = ? AND trust_state = 'UNTRUSTED'", workflow.ID).
		Count(&counts.Untrusted).Error; err != nil {
		t.Fatalf("count untrusted signature evidence: %v", err)
	}
	if counts.Evidence != 3 || counts.Trusted != 1 || counts.Untrusted != 1 {
		t.Fatalf("signature evidence counts = %#v", counts)
	}
	var historicalCount int64
	if err := pool.ORM().WithContext(ctx).Table("signature_evidence AS evidence").
		Joins("JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id").
		Where("workflow.repository_id = ? AND workflow.digest = ? AND workflow.policy_version = 1 AND evidence.trust_state = 'TRUSTED'", fixture.repositoryID, fixture.firstDigest).
		Count(&historicalCount).Error; err != nil {
		t.Fatalf("count immutable historical evidence: %v", err)
	}
	if historicalCount != 1 {
		t.Fatalf("historical evidence count = %d; want 1", historicalCount)
	}
}

type securityFixture struct {
	userID, namespaceID, repositoryID string
	namespace, repository             string
	firstDigest, secondDigest         string
}

func createFixture(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	now time.Time,
) securityFixture {
	t.Helper()
	suffix := randomHex(t, 6)
	fixture := securityFixture{
		userID: uuid(t), namespaceID: uuid(t), repositoryID: uuid(t),
		namespace: "security-" + suffix, repository: "image-" + suffix,
		firstDigest: "sha256:" + repeatHex("a", 64), secondDigest: "sha256:" + repeatHex("b", 64),
	}
	database := pool.ORM().WithContext(ctx)
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO users (id, username, created_at, updated_at) VALUES (?, ?, ?, ?)", []any{fixture.userID, fixture.namespace, now, now}},
		{"INSERT INTO namespaces (id, name, owner_user_id, owner_kind, created_at) VALUES (?, ?, ?, 'PERSONAL', ?)", []any{fixture.namespaceID, fixture.namespace, fixture.userID, now}},
		{"INSERT INTO repositories (id, namespace_id, name, visibility, description, created_by_user_id, visibility_updated_by_user_id, visibility_updated_at, created_at, updated_at) VALUES (?, ?, ?, 'PRIVATE', '', ?, ?, ?, ?, ?)", []any{fixture.repositoryID, fixture.namespaceID, fixture.repository, fixture.userID, fixture.userID, now, now, now}},
	}
	for _, digest := range []string{fixture.firstDigest, fixture.secondDigest} {
		statements = append(statements, struct {
			query string
			args  []any
		}{"INSERT INTO artifacts (id, repository_id, digest, kind, descriptors_complete, discovered_at, updated_at) VALUES (?, ?, ?, 'MANIFEST', false, ?, ?)", []any{uuid(t), fixture.repositoryID, digest, now, now}})
	}
	for _, statement := range statements {
		if err := database.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("create security fixture: %v", err)
		}
	}
	return fixture
}

func (f securityFixture) cleanup(t *testing.T, ctx context.Context, pool *postgres.Pool) {
	t.Helper()
	database := pool.ORM().WithContext(ctx)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"DELETE FROM vulnerability_findings WHERE workflow_id IN (SELECT id FROM security_workflows WHERE repository_id = ?)", []any{f.repositoryID}},
		{"DELETE FROM signature_evidence WHERE workflow_id IN (SELECT id FROM signature_workflows WHERE repository_id = ?)", []any{f.repositoryID}},
		{"DELETE FROM signature_verifications WHERE workflow_id IN (SELECT id FROM signature_workflows WHERE repository_id = ?)", []any{f.repositoryID}},
		{"DELETE FROM signature_workflows WHERE repository_id = ?", []any{f.repositoryID}},
		{"DELETE FROM trust_policy_identities WHERE policy_id IN (SELECT id FROM trust_policies WHERE namespace_id = ?)", []any{f.namespaceID}},
		{"DELETE FROM trust_policy_public_keys WHERE policy_id IN (SELECT id FROM trust_policies WHERE namespace_id = ?)", []any{f.namespaceID}},
		{"DELETE FROM trust_policies WHERE namespace_id = ?", []any{f.namespaceID}},
		{"DELETE FROM cosign_tool_state WHERE name = 'COSIGN'", nil},
		{"DELETE FROM security_scan_reports WHERE workflow_id IN (SELECT id FROM security_workflows WHERE repository_id = ?)", []any{f.repositoryID}},
		{"DELETE FROM security_sboms WHERE workflow_id IN (SELECT id FROM security_workflows WHERE repository_id = ?)", []any{f.repositoryID}},
		{"DELETE FROM security_tool_state WHERE scanner_name = 'TRIVY'", nil},
		{"DELETE FROM security_workflows WHERE repository_id = ?", []any{f.repositoryID}},
		{"DELETE FROM jobs WHERE intent_key LIKE ?", []any{"%:" + f.repositoryID + ":%"}},
		{"DELETE FROM artifacts WHERE repository_id = ?", []any{f.repositoryID}},
		{"DELETE FROM repositories WHERE id = ?", []any{f.repositoryID}},
		{"DELETE FROM namespaces WHERE id = ?", []any{f.namespaceID}},
		{"DELETE FROM users WHERE id = ?", []any{f.userID}},
	} {
		if err := database.Exec(statement.query, statement.args...).Error; err != nil {
			t.Errorf("cleanup security fixture: %v", err)
		}
	}
}

func trustTestPublicKey(t *testing.T, name string) security.PublicKeyTrust {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey() error = %v", err)
	}
	key, err := security.NewPublicKeyTrust(
		name, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
	)
	if err != nil {
		t.Fatalf("security.NewPublicKeyTrust() error = %v", err)
	}
	return key
}

func trustTestEvidence(
	t *testing.T,
	digit string,
	signer security.SignerType,
	fingerprint string,
	state security.CryptographicState,
) security.CryptographicEvidence {
	t.Helper()
	digest, err := artifacts.ParseDigest("sha256:" + repeatHex(digit, 64))
	if err != nil {
		t.Fatalf("artifacts.ParseDigest() error = %v", err)
	}
	return security.CryptographicEvidence{
		SignatureDigest: digest, Kind: security.SignatureKindSignature,
		SignerType: signer, KeyFingerprint: fingerprint, State: state,
	}
}

func completeJob(t *testing.T, ctx context.Context, pool *postgres.Pool, id string, at time.Time) {
	t.Helper()
	result := pool.ORM().WithContext(ctx).Exec(
		"UPDATE jobs SET state = ?, attempt_count = 1, started_at = ?, completed_at = ?, updated_at = ? WHERE id = ?",
		jobs.StateSucceeded, at, at, at, id,
	)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("complete job %s: rows=%d error=%v", id, result.RowsAffected, result.Error)
	}
}

func uuid(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func randomHex(t *testing.T, bytes int) string {
	t.Helper()
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(value)
}

func repeatHex(value string, count int) string {
	result := ""
	for range count {
		result += value
	}
	return result
}
