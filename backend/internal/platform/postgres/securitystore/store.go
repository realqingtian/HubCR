package securitystore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/postgres/jobstore"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (s *Store) EnsureWorkflow(
	ctx context.Context,
	target security.Target,
	now time.Time,
) (security.Workflow, bool, error) {
	if _, err := security.NewTarget(
		target.RepositoryID, target.Namespace, target.Repository, target.Digest.String(),
	); err != nil || now.IsZero() {
		return security.Workflow{}, false, security.ErrInvalid
	}
	now = now.UTC().Round(time.Microsecond)
	var workflow security.Workflow
	created := false
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		persisted, err := targetByDigest(transaction, target.RepositoryID, target.Digest.String())
		if err != nil {
			return err
		}
		if persisted.Namespace != target.Namespace || persisted.Repository != target.Repository {
			return security.ErrConflict
		}
		payload, err := security.MarshalJobPayload(target)
		if err != nil {
			return err
		}
		queue := jobstore.New(transaction)
		scanJob, _, err := enqueueSecurityJob(
			ctx, queue, security.ScanJobKind, target, payload, now,
		)
		if err != nil {
			return err
		}
		sbomJob, _, err := enqueueSecurityJob(
			ctx, queue, security.SBOMJobKind, target, payload, now,
		)
		if err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		record := workflowRecord{
			ID: id, RepositoryID: target.RepositoryID, Digest: target.Digest.String(),
			ScanJobID: scanJob.ID, SBOMJobID: sbomJob.ID, CreatedAt: now, UpdatedAt: now,
		}
		result := transaction.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "repository_id"}, {Name: "digest"}},
			DoNothing: true,
		}).Create(&record)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		current, err := workflowByTarget(transaction, target.RepositoryID, target.Digest.String())
		if err != nil {
			return err
		}
		if current.ScanJobID != scanJob.ID || current.SBOMJobID != sbomJob.ID {
			return security.ErrConflict
		}
		workflow, err = workflowFromRecord(current)
		return err
	})
	if err != nil {
		return security.Workflow{}, false, classify("ensure security workflow", err)
	}
	return workflow, created, nil
}

func (s *Store) RepairMissingWorkflows(
	ctx context.Context,
	limit int,
	now time.Time,
) (int, error) {
	if limit < 1 || limit > security.MaxRepairBatch || now.IsZero() {
		return 0, security.ErrInvalid
	}
	var targets []workflowReadRecord
	if err := s.database.WithContext(ctx).Raw(
		`SELECT a.repository_id, a.digest, n.name AS namespace, r.name AS repository
		 FROM artifacts AS a
		 JOIN repositories AS r ON r.id = a.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 LEFT JOIN security_workflows AS workflow
		   ON workflow.repository_id = a.repository_id AND workflow.digest = a.digest
		 WHERE workflow.id IS NULL
		 ORDER BY a.discovered_at, a.repository_id, a.digest
		 LIMIT ?`,
		limit,
	).Scan(&targets).Error; err != nil {
		return 0, classify("list missing security workflows", err)
	}
	repaired := 0
	for _, record := range targets {
		target, err := security.NewTarget(
			record.RepositoryID, record.Namespace, record.Repository, record.Digest,
		)
		if err != nil {
			return repaired, classify("decode repair target", err)
		}
		_, created, err := s.EnsureWorkflow(ctx, target, now)
		if err != nil {
			return repaired, err
		}
		if created {
			repaired++
		}
	}
	return repaired, nil
}

func (s *Store) ResolveJob(
	ctx context.Context,
	job jobs.Job,
) (security.Workflow, security.Target, error) {
	if err := job.Validate(); err != nil ||
		(job.Kind != jobs.Kind(security.ScanJobKind) && job.Kind != jobs.Kind(security.SBOMJobKind)) {
		return security.Workflow{}, security.Target{}, security.ErrInvalid
	}
	payloadTarget, err := security.ParseJobPayload(job.Payload)
	if err != nil {
		return security.Workflow{}, security.Target{}, err
	}
	current, err := workflowByTarget(
		s.database.WithContext(ctx), payloadTarget.RepositoryID, payloadTarget.Digest.String(),
	)
	if err != nil {
		return security.Workflow{}, security.Target{}, classify("resolve security job", err)
	}
	workflow, err := workflowFromRecord(current)
	if err != nil {
		return security.Workflow{}, security.Target{}, classify("decode security workflow", err)
	}
	if workflow.Target != payloadTarget ||
		(job.Kind == jobs.Kind(security.ScanJobKind) && workflow.ScanJobID != job.ID) ||
		(job.Kind == jobs.Kind(security.SBOMJobKind) && workflow.SBOMJobID != job.ID) {
		return security.Workflow{}, security.Target{}, security.ErrConflict
	}
	return workflow, payloadTarget, nil
}

func (s *Store) SaveScanResult(
	ctx context.Context,
	workflow security.Workflow,
	result security.ScanResult,
) error {
	if workflow.Validate() != nil || result.Validate() != nil || workflow.Target != result.Target {
		return security.ErrInvalid
	}
	return classify("save security scan", s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := lockWorkflow(transaction, workflow); err != nil {
			return err
		}
		report := scanReportRecord{
			WorkflowID: workflow.ID, ScannerVersion: result.ToolVersion.ScannerVersion,
			DatabaseSchema:       result.ToolVersion.DatabaseSchemaVersion,
			DatabaseUpdatedAt:    result.ToolVersion.DatabaseUpdatedAt.UTC(),
			DatabaseDownloadedAt: result.ToolVersion.DatabaseDownloadedAt.UTC(),
			CompletedAt:          result.CompletedAt.UTC(), UpdatedAt: result.CompletedAt.UTC(),
		}
		if err := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workflow_id"}}, UpdateAll: true,
		}).Create(&report).Error; err != nil {
			return err
		}
		if err := transaction.Where("workflow_id = ?", workflow.ID).Delete(&findingRecord{}).Error; err != nil {
			return err
		}
		findings := make([]findingRecord, 0, len(result.Findings))
		for _, finding := range result.Findings {
			id, err := newID()
			if err != nil {
				return err
			}
			findings = append(findings, findingToRecord(id, workflow.ID, finding))
		}
		if len(findings) > 0 {
			if err := transaction.CreateInBatches(findings, 100).Error; err != nil {
				return err
			}
		}
		tool := toolStateRecord{
			ScannerName:          security.ScannerNameTrivy,
			ScannerVersion:       result.ToolVersion.ScannerVersion,
			DatabaseSchema:       result.ToolVersion.DatabaseSchemaVersion,
			DatabaseUpdatedAt:    result.ToolVersion.DatabaseUpdatedAt.UTC(),
			DatabaseDownloadedAt: result.ToolVersion.DatabaseDownloadedAt.UTC(),
			ObservedAt:           result.ToolVersion.ObservedAt.UTC(),
		}
		return transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "scanner_name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"scanner_version":        tool.ScannerVersion,
				"database_schema":        tool.DatabaseSchema,
				"database_updated_at":    tool.DatabaseUpdatedAt,
				"database_downloaded_at": tool.DatabaseDownloadedAt,
				"observed_at":            tool.ObservedAt,
			}),
			Where: clause.Where{Exprs: []clause.Expression{
				clause.Expr{SQL: "excluded.database_updated_at > security_tool_state.database_updated_at OR (excluded.database_updated_at = security_tool_state.database_updated_at AND excluded.observed_at > security_tool_state.observed_at)"},
			}},
		}).Create(&tool).Error
	}))
}

func (s *Store) SaveSBOMResult(
	ctx context.Context,
	workflow security.Workflow,
	result security.SBOMResult,
) error {
	if workflow.Validate() != nil || result.Validate() != nil || workflow.Target != result.Target {
		return security.ErrInvalid
	}
	return classify("save security SBOM", s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := lockWorkflow(transaction, workflow); err != nil {
			return err
		}
		record := sbomRecord{
			WorkflowID: workflow.ID, GeneratorVersion: result.GeneratorVersion,
			Format: result.Format, Document: append([]byte(nil), result.Document...),
			CompletedAt: result.CompletedAt.UTC(), UpdatedAt: result.CompletedAt.UTC(),
		}
		return transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workflow_id"}}, UpdateAll: true,
		}).Create(&record).Error
	}))
}

func (s *Store) Detail(ctx context.Context, repositoryID, digest string) (security.Detail, error) {
	if _, err := security.NewTarget(repositoryID, "placeholder", "placeholder", digest); err != nil {
		return security.Detail{}, security.ErrInvalid
	}
	current, err := workflowByTarget(s.database.WithContext(ctx), repositoryID, digest)
	if err != nil {
		return security.Detail{}, classify("find security detail", err)
	}
	workflow, err := workflowFromRecord(current)
	if err != nil {
		return security.Detail{}, classify("decode security detail", err)
	}
	queue := jobstore.New(s.database)
	scanJob, err := queue.ByID(ctx, workflow.ScanJobID)
	if err != nil {
		return security.Detail{}, classify("find security scan job", err)
	}
	sbomJob, err := queue.ByID(ctx, workflow.SBOMJobID)
	if err != nil {
		return security.Detail{}, classify("find security SBOM job", err)
	}

	var report scanReportRecord
	reportError := s.database.WithContext(ctx).Where("workflow_id = ?", workflow.ID).First(&report).Error
	if reportError != nil && !errors.Is(reportError, gorm.ErrRecordNotFound) {
		return security.Detail{}, classify("find security scan report", reportError)
	}
	var sbom sbomRecord
	sbomError := s.database.WithContext(ctx).Where("workflow_id = ?", workflow.ID).First(&sbom).Error
	if sbomError != nil && !errors.Is(sbomError, gorm.ErrRecordNotFound) {
		return security.Detail{}, classify("find security SBOM", sbomError)
	}
	var tool toolStateRecord
	toolError := s.database.WithContext(ctx).Where("scanner_name = ?", security.ScannerNameTrivy).First(&tool).Error
	if toolError != nil && !errors.Is(toolError, gorm.ErrRecordNotFound) {
		return security.Detail{}, classify("find security tool state", toolError)
	}
	reportPresent := reportError == nil
	sbomPresent := sbomError == nil
	scanStale := reportPresent && toolError == nil &&
		(report.ScannerVersion != tool.ScannerVersion || report.DatabaseSchema != tool.DatabaseSchema ||
			report.DatabaseUpdatedAt.Before(tool.DatabaseUpdatedAt))
	sbomStale := sbomPresent && toolError == nil && sbom.GeneratorVersion != tool.ScannerVersion
	scanStatus, err := security.ResultStatusFromJob(scanJob, reportPresent, scanStale)
	if err != nil {
		return security.Detail{}, err
	}
	sbomStatus, err := security.ResultStatusFromJob(sbomJob, sbomPresent, sbomStale)
	if err != nil {
		return security.Detail{}, err
	}
	detail := security.Detail{
		Workflow: workflow, Scan: scanStatus, SBOM: sbomStatus,
		SeverityCounts: map[string]int{},
	}
	if reportPresent {
		version := security.ToolVersion{
			ScannerVersion: report.ScannerVersion, DatabaseSchemaVersion: report.DatabaseSchema,
			DatabaseUpdatedAt:    report.DatabaseUpdatedAt.UTC(),
			DatabaseDownloadedAt: report.DatabaseDownloadedAt.UTC(),
			ObservedAt:           report.UpdatedAt.UTC(),
		}
		detail.ToolVersion = &version
		completed := report.CompletedAt.UTC()
		detail.ScannedAt = &completed
		type severityCount struct {
			Severity string
			Count    int
		}
		var counts []severityCount
		if err := s.database.WithContext(ctx).Model(&findingRecord{}).
			Select("severity, count(*) AS count").Where("workflow_id = ?", workflow.ID).
			Group("severity").Scan(&counts).Error; err != nil {
			return security.Detail{}, classify("count vulnerabilities", err)
		}
		for _, count := range counts {
			detail.SeverityCounts[count.Severity] = count.Count
			detail.FindingCount += count.Count
		}
	}
	if sbomPresent {
		detail.SBOMFormat = sbom.Format
		completed := sbom.CompletedAt.UTC()
		detail.SBOMCreatedAt = &completed
	}
	signature, err := s.signatureDetail(ctx, repositoryID, digest)
	if err != nil {
		return security.Detail{}, err
	}
	detail.Signature = signature
	return detail, nil
}

func (s *Store) signatureDetail(
	ctx context.Context,
	repositoryID, digest string,
) (*security.VerificationDetail, error) {
	record, err := latestSignatureWorkflowByTarget(s.database.WithContext(ctx), repositoryID, digest)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, classify("find signature workflow", err)
	}
	workflow, err := signatureWorkflowFromRecord(record)
	if err != nil {
		return nil, classify("decode signature workflow", err)
	}
	job, err := jobstore.New(s.database).ByID(ctx, workflow.JobID)
	if err != nil {
		return nil, classify("find signature job", err)
	}
	var verification signatureVerificationRecord
	verificationError := s.database.WithContext(ctx).
		Where("workflow_id = ?", workflow.ID).First(&verification).Error
	if verificationError != nil && !errors.Is(verificationError, gorm.ErrRecordNotFound) {
		return nil, classify("find signature verification", verificationError)
	}
	currentPolicy, err := currentPolicyByNamespace(s.database.WithContext(ctx), record.NamespaceID)
	if err != nil {
		return nil, classify("find current trust policy", err)
	}
	verificationPresent := verificationError == nil
	status, err := security.ResultStatusFromJob(
		job, verificationPresent, currentPolicy.ID != workflow.PolicyID,
	)
	if err != nil {
		return nil, err
	}
	detail := &security.VerificationDetail{
		Workflow: workflow, Status: status, Evidence: []security.SignatureEvidence{},
	}
	if !verificationPresent {
		return detail, nil
	}
	detail.CosignVersion = verification.CosignVersion
	completed := verification.CompletedAt.UTC()
	detail.CompletedAt = &completed
	var records []signatureEvidenceRecord
	if err := s.database.WithContext(ctx).Where("workflow_id = ?", workflow.ID).
		Order("kind, signature_digest").Find(&records).Error; err != nil {
		return nil, classify("find signature evidence", err)
	}
	for _, evidenceRecord := range records {
		signatureDigest, err := artifacts.ParseDigest(evidenceRecord.SignatureDigest)
		if err != nil {
			return nil, security.ErrInvalid
		}
		evidence := security.SignatureEvidence{
			CryptographicEvidence: security.CryptographicEvidence{
				SignatureDigest: signatureDigest,
				Kind:            security.SignatureKind(evidenceRecord.Kind),
				SignerType:      security.SignerType(evidenceRecord.SignerType),
				KeyFingerprint:  evidenceRecord.KeyFingerprint,
				OIDCIssuer:      evidenceRecord.OIDCIssuer,
				Subject:         evidenceRecord.Subject,
				State:           security.CryptographicState(evidenceRecord.CryptographicState),
			},
			TrustState: security.PolicyTrustState(evidenceRecord.TrustState),
			Reason:     evidenceRecord.Reason,
		}
		if evidence.Validate() != nil {
			return nil, security.ErrInvalid
		}
		detail.Evidence = append(detail.Evidence, evidence)
	}
	return detail, nil
}

func enqueueSecurityJob(
	ctx context.Context,
	queue *jobstore.Store,
	kind string,
	target security.Target,
	payload json.RawMessage,
	now time.Time,
) (jobs.Job, bool, error) {
	key, err := security.IntentKey(kind, target)
	if err != nil {
		return jobs.Job{}, false, err
	}
	intent, err := jobs.NewIntent(kind, key, payload, security.DefaultAttempts, now)
	if err != nil {
		return jobs.Job{}, false, err
	}
	return queue.Enqueue(ctx, intent, now)
}

func targetByDigest(database *gorm.DB, repositoryID, digest string) (workflowReadRecord, error) {
	var record workflowReadRecord
	err := database.Raw(
		`SELECT a.repository_id, a.digest, n.name AS namespace, r.name AS repository
		 FROM artifacts AS a
		 JOIN repositories AS r ON r.id = a.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 WHERE a.repository_id = ? AND a.digest = ?`,
		repositoryID, digest,
	).Scan(&record).Error
	if err != nil {
		return workflowReadRecord{}, err
	}
	if record.RepositoryID == "" {
		return workflowReadRecord{}, gorm.ErrRecordNotFound
	}
	return record, nil
}

func workflowByTarget(database *gorm.DB, repositoryID, digest string) (workflowReadRecord, error) {
	var record workflowReadRecord
	err := database.Raw(
		`SELECT workflow.*, n.name AS namespace, r.name AS repository
		 FROM security_workflows AS workflow
		 JOIN repositories AS r ON r.id = workflow.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 WHERE workflow.repository_id = ? AND workflow.digest = ?`,
		repositoryID, digest,
	).Scan(&record).Error
	if err != nil {
		return workflowReadRecord{}, err
	}
	if record.ID == "" {
		return workflowReadRecord{}, gorm.ErrRecordNotFound
	}
	return record, nil
}

func workflowFromRecord(record workflowReadRecord) (security.Workflow, error) {
	target, err := security.NewTarget(record.RepositoryID, record.Namespace, record.Repository, record.Digest)
	if err != nil {
		return security.Workflow{}, err
	}
	workflow := security.Workflow{
		ID: record.ID, Target: target, ScanJobID: record.ScanJobID, SBOMJobID: record.SBOMJobID,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
	if err := workflow.Validate(); err != nil {
		return security.Workflow{}, err
	}
	return workflow, nil
}

func lockWorkflow(database *gorm.DB, workflow security.Workflow) error {
	var record workflowRecord
	if err := database.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", workflow.ID).First(&record).Error; err != nil {
		return err
	}
	if record.RepositoryID != workflow.Target.RepositoryID || record.Digest != workflow.Target.Digest.String() ||
		record.ScanJobID != workflow.ScanJobID || record.SBOMJobID != workflow.SBOMJobID {
		return security.ErrConflict
	}
	return nil
}

func findingToRecord(id, workflowID string, finding security.Finding) findingRecord {
	return findingRecord{
		ID: id, WorkflowID: workflowID, Target: finding.Target, Class: finding.Class,
		Type: finding.Type, VulnerabilityID: finding.VulnerabilityID,
		PackageName: finding.PackageName, InstalledVersion: finding.InstalledVersion,
		FixedVersion: finding.FixedVersion, Status: finding.Status, Severity: finding.Severity,
		SeveritySource: finding.SeveritySource, PrimaryURL: finding.PrimaryURL,
		DataSourceID: finding.DataSourceID, Title: finding.Title,
		PublishedAt: cloneTime(finding.PublishedAt), ModifiedAt: cloneTime(finding.ModifiedAt),
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded[:]), nil
}

func classify(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, security.ErrInvalid) || errors.Is(err, security.ErrConflict) ||
		errors.Is(err, security.ErrNotFound) || errors.Is(err, security.ErrUnavailable) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, jobs.ErrInvalidJob) || errors.Is(err, jobs.ErrConflict) {
		return fmt.Errorf("%s: %w", operation, security.ErrConflict)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, jobs.ErrNoJob) {
		return fmt.Errorf("%s: %w", operation, security.ErrNotFound)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, security.ErrConflict)
		case "23502", "23503", "23514", "22P02":
			return fmt.Errorf("%s: %w", operation, security.ErrInvalid)
		}
	}
	return fmt.Errorf("%s: %w", operation, security.ErrUnavailable)
}

var _ security.Store = (*Store)(nil)
