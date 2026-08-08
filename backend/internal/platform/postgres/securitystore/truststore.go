package securitystore

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/postgres/jobstore"
)

func (s *Store) CreateTrustPolicy(
	ctx context.Context,
	namespaceID, actorID string,
	keys []security.PublicKeyTrust,
	identities []security.KeylessIdentity,
	now time.Time,
) (security.TrustPolicy, error) {
	if namespaceID == "" || actorID == "" || now.IsZero() || len(keys)+len(identities) < 1 ||
		len(keys)+len(identities) > security.MaxTrustPolicySubjects {
		return security.TrustPolicy{}, security.ErrInvalid
	}
	for _, key := range keys {
		if key.Validate() != nil {
			return security.TrustPolicy{}, security.ErrInvalid
		}
	}
	for _, identity := range identities {
		if identity.Validate() != nil {
			return security.TrustPolicy{}, security.ErrInvalid
		}
	}
	now = now.UTC().Round(time.Microsecond)
	var policy security.TrustPolicy
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var namespace struct{ ID string }
		if err := transaction.Raw("SELECT id FROM namespaces WHERE id = ? FOR UPDATE", namespaceID).
			Scan(&namespace).Error; err != nil {
			return err
		}
		if namespace.ID == "" {
			return gorm.ErrRecordNotFound
		}
		var current struct{ Version int64 }
		if err := transaction.Raw(
			"SELECT COALESCE(MAX(version), 0) AS version FROM trust_policies WHERE namespace_id = ?",
			namespaceID,
		).Scan(&current).Error; err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		record := trustPolicyRecord{
			ID: id, NamespaceID: namespaceID, Version: current.Version + 1,
			CreatedByUserID: actorID, CreatedAt: now,
		}
		if err := transaction.Create(&record).Error; err != nil {
			return err
		}
		for _, key := range keys {
			keyID, err := newID()
			if err != nil {
				return err
			}
			if err := transaction.Create(&trustPolicyPublicKeyRecord{
				ID: keyID, PolicyID: id, Fingerprint: key.Fingerprint,
				Name: key.Name, PublicKeyPEM: key.PublicKeyPEM, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		for _, identity := range identities {
			identityID, err := newID()
			if err != nil {
				return err
			}
			if err := transaction.Create(&trustPolicyIdentityRecord{
				ID: identityID, PolicyID: id, Issuer: identity.Issuer,
				Subject: identity.Subject, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		policy, err = policyByID(transaction, id)
		return err
	})
	if err != nil {
		return security.TrustPolicy{}, classify("create trust policy", err)
	}
	return policy, nil
}

func (s *Store) EnsureCurrentVerification(
	ctx context.Context,
	target security.Target,
	now time.Time,
) (security.VerificationWorkflow, bool, error) {
	if _, err := security.NewTarget(target.RepositoryID, target.Namespace, target.Repository, target.Digest.String()); err != nil || now.IsZero() {
		return security.VerificationWorkflow{}, false, security.ErrInvalid
	}
	now = now.UTC().Round(time.Microsecond)
	var workflow security.VerificationWorkflow
	created := false
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		persisted, err := signatureTargetByDigest(transaction, target.RepositoryID, target.Digest.String())
		if err != nil {
			return err
		}
		if persisted.Namespace != target.Namespace || persisted.Repository != target.Repository {
			return security.ErrConflict
		}
		policy, err := currentPolicyByNamespace(transaction, persisted.NamespaceID)
		if err != nil {
			return err
		}
		payload, err := security.MarshalVerificationPayload(target, policy)
		if err != nil {
			return err
		}
		intentKey, err := security.VerificationIntentKey(target, policy)
		if err != nil {
			return err
		}
		intent, err := jobs.NewIntent(
			security.VerificationJobKind, intentKey, payload, security.DefaultAttempts, now,
		)
		if err != nil {
			return err
		}
		job, _, err := jobstore.New(transaction).Enqueue(ctx, intent, now)
		if err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		record := signatureWorkflowRecord{
			ID: id, RepositoryID: target.RepositoryID, Digest: target.Digest.String(),
			PolicyID: policy.ID, PolicyVersion: policy.Version, JobID: job.ID, CreatedAt: now,
		}
		result := transaction.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "repository_id"}, {Name: "digest"}, {Name: "policy_id"}},
			DoNothing: true,
		}).Create(&record)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		current, err := signatureWorkflowByTargetPolicy(
			transaction, target.RepositoryID, target.Digest.String(), policy.ID,
		)
		if err != nil {
			return err
		}
		if current.JobID != job.ID || current.PolicyVersion != policy.Version {
			return security.ErrConflict
		}
		workflow, err = signatureWorkflowFromRecord(current)
		return err
	})
	if err != nil {
		return security.VerificationWorkflow{}, false, classify("ensure signature workflow", err)
	}
	return workflow, created, nil
}

func (s *Store) RepairMissingVerificationWorkflows(
	ctx context.Context,
	limit int,
	now time.Time,
) (int, error) {
	if limit < 1 || limit > security.MaxRepairBatch || now.IsZero() {
		return 0, security.ErrInvalid
	}
	var targets []signatureWorkflowReadRecord
	if err := s.database.WithContext(ctx).Raw(
		`WITH current_policies AS (
		   SELECT DISTINCT ON (namespace_id) id, namespace_id
		   FROM trust_policies ORDER BY namespace_id, version DESC
		 )
		 SELECT a.repository_id, a.digest, n.id AS namespace_id,
		        n.name AS namespace, r.name AS repository
		 FROM artifacts AS a
		 JOIN repositories AS r ON r.id = a.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 JOIN current_policies AS policy ON policy.namespace_id = n.id
		 LEFT JOIN signature_workflows AS workflow
		   ON workflow.repository_id = a.repository_id
		  AND workflow.digest = a.digest AND workflow.policy_id = policy.id
		 WHERE workflow.id IS NULL
		 ORDER BY a.discovered_at, a.repository_id, a.digest
		 LIMIT ?`,
		limit,
	).Scan(&targets).Error; err != nil {
		return 0, classify("list missing signature workflows", err)
	}
	repaired := 0
	for _, record := range targets {
		target, err := security.NewTarget(record.RepositoryID, record.Namespace, record.Repository, record.Digest)
		if err != nil {
			return repaired, classify("decode signature repair target", err)
		}
		_, created, err := s.EnsureCurrentVerification(ctx, target, now)
		if err != nil {
			return repaired, err
		}
		if created {
			repaired++
		}
	}
	return repaired, nil
}

func (s *Store) ResolveVerificationJob(
	ctx context.Context,
	job jobs.Job,
) (security.VerificationInput, error) {
	if err := job.Validate(); err != nil || job.Kind != jobs.Kind(security.VerificationJobKind) {
		return security.VerificationInput{}, security.ErrInvalid
	}
	target, policyID, policyVersion, err := security.ParseVerificationPayload(job.Payload)
	if err != nil {
		return security.VerificationInput{}, err
	}
	record, err := signatureWorkflowByTargetPolicy(
		s.database.WithContext(ctx), target.RepositoryID, target.Digest.String(), policyID,
	)
	if err != nil {
		return security.VerificationInput{}, classify("resolve signature workflow", err)
	}
	workflow, err := signatureWorkflowFromRecord(record)
	if err != nil || workflow.Target != target || workflow.PolicyVersion != policyVersion || workflow.JobID != job.ID {
		return security.VerificationInput{}, security.ErrConflict
	}
	policy, err := policyByID(s.database.WithContext(ctx), policyID)
	if err != nil {
		return security.VerificationInput{}, classify("resolve trust policy", err)
	}
	keys, identities, err := candidateSubjectsByNamespace(s.database.WithContext(ctx), record.NamespaceID)
	if err != nil {
		return security.VerificationInput{}, classify("resolve trust candidates", err)
	}
	input := security.VerificationInput{
		Workflow: workflow, Policy: policy, CandidateKeys: keys, CandidateIdentities: identities,
	}
	if input.Validate() != nil {
		return security.VerificationInput{}, security.ErrInvalid
	}
	return input, nil
}

func (s *Store) SaveVerificationResult(ctx context.Context, result security.VerificationResult) error {
	if result.Validate() != nil {
		return security.ErrInvalid
	}
	return classify("save signature verification", s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var workflow signatureWorkflowRecord
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", result.Workflow.ID).First(&workflow).Error; err != nil {
			return err
		}
		if workflow.RepositoryID != result.Workflow.Target.RepositoryID ||
			workflow.Digest != result.Workflow.Target.Digest.String() ||
			workflow.PolicyID != result.Workflow.PolicyID ||
			workflow.PolicyVersion != result.Workflow.PolicyVersion || workflow.JobID != result.Workflow.JobID {
			return security.ErrConflict
		}
		record := signatureVerificationRecord{
			WorkflowID: result.Workflow.ID, CosignVersion: result.CosignVersion,
			CompletedAt: result.CompletedAt.UTC(), UpdatedAt: result.CompletedAt.UTC(),
		}
		if err := transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workflow_id"}}, UpdateAll: true,
		}).Create(&record).Error; err != nil {
			return err
		}
		if err := transaction.Where("workflow_id = ?", result.Workflow.ID).
			Delete(&signatureEvidenceRecord{}).Error; err != nil {
			return err
		}
		evidence := make([]signatureEvidenceRecord, 0, len(result.Evidence))
		for _, item := range result.Evidence {
			id, err := newID()
			if err != nil {
				return err
			}
			evidence = append(evidence, signatureEvidenceRecord{
				ID: id, WorkflowID: result.Workflow.ID, Kind: string(item.Kind),
				SignatureDigest: item.SignatureDigest.String(), SignerType: string(item.SignerType),
				KeyFingerprint: item.KeyFingerprint, OIDCIssuer: item.OIDCIssuer,
				Subject: item.Subject, CryptographicState: string(item.State),
				TrustState: string(item.TrustState), Reason: item.Reason,
				VerifiedAt: result.CompletedAt.UTC(),
			})
		}
		if len(evidence) > 0 {
			if err := transaction.CreateInBatches(evidence, 100).Error; err != nil {
				return err
			}
		}
		tool := cosignToolStateRecord{
			Name: security.CosignName, Version: result.CosignVersion, ObservedAt: result.CompletedAt.UTC(),
		}
		return transaction.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"version": tool.Version, "observed_at": tool.ObservedAt,
			}),
			Where: clause.Where{Exprs: []clause.Expression{
				clause.Expr{SQL: "excluded.observed_at > cosign_tool_state.observed_at"},
			}},
		}).Create(&tool).Error
	}))
}

func currentPolicyByNamespace(database *gorm.DB, namespaceID string) (security.TrustPolicy, error) {
	var record trustPolicyRecord
	if err := database.Where("namespace_id = ?", namespaceID).Order("version DESC").First(&record).Error; err != nil {
		return security.TrustPolicy{}, err
	}
	return policyFromRecord(database, record)
}

func policyByID(database *gorm.DB, policyID string) (security.TrustPolicy, error) {
	var record trustPolicyRecord
	if err := database.Where("id = ?", policyID).First(&record).Error; err != nil {
		return security.TrustPolicy{}, err
	}
	return policyFromRecord(database, record)
}

func policyFromRecord(database *gorm.DB, record trustPolicyRecord) (security.TrustPolicy, error) {
	var keyRecords []trustPolicyPublicKeyRecord
	if err := database.Where("policy_id = ?", record.ID).Order("fingerprint").Find(&keyRecords).Error; err != nil {
		return security.TrustPolicy{}, err
	}
	var identityRecords []trustPolicyIdentityRecord
	if err := database.Where("policy_id = ?", record.ID).Order("issuer, subject").Find(&identityRecords).Error; err != nil {
		return security.TrustPolicy{}, err
	}
	policy := security.TrustPolicy{
		ID: record.ID, NamespaceID: record.NamespaceID, Version: record.Version,
		CreatedByUserID: record.CreatedByUserID, CreatedAt: record.CreatedAt.UTC(),
		PublicKeys:        make([]security.PublicKeyTrust, 0, len(keyRecords)),
		KeylessIdentities: make([]security.KeylessIdentity, 0, len(identityRecords)),
	}
	for _, key := range keyRecords {
		policy.PublicKeys = append(policy.PublicKeys, security.PublicKeyTrust{
			Fingerprint: key.Fingerprint, Name: key.Name, PublicKeyPEM: key.PublicKeyPEM,
		})
	}
	for _, identity := range identityRecords {
		policy.KeylessIdentities = append(policy.KeylessIdentities, security.KeylessIdentity{
			Issuer: identity.Issuer, Subject: identity.Subject,
		})
	}
	if policy.Validate() != nil {
		return security.TrustPolicy{}, security.ErrInvalid
	}
	return policy, nil
}

func signatureTargetByDigest(database *gorm.DB, repositoryID, digest string) (signatureWorkflowReadRecord, error) {
	var record signatureWorkflowReadRecord
	if err := database.Raw(
		`SELECT a.repository_id, a.digest, n.id AS namespace_id,
		        n.name AS namespace, r.name AS repository
		 FROM artifacts AS a
		 JOIN repositories AS r ON r.id = a.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 WHERE a.repository_id = ? AND a.digest = ?`,
		repositoryID, digest,
	).Scan(&record).Error; err != nil {
		return signatureWorkflowReadRecord{}, err
	}
	if record.RepositoryID == "" {
		return signatureWorkflowReadRecord{}, gorm.ErrRecordNotFound
	}
	return record, nil
}

func signatureWorkflowByTargetPolicy(
	database *gorm.DB,
	repositoryID, digest, policyID string,
) (signatureWorkflowReadRecord, error) {
	var record signatureWorkflowReadRecord
	if err := database.Raw(
		`SELECT workflow.*, n.id AS namespace_id, n.name AS namespace, r.name AS repository
		 FROM signature_workflows AS workflow
		 JOIN repositories AS r ON r.id = workflow.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 WHERE workflow.repository_id = ? AND workflow.digest = ? AND workflow.policy_id = ?`,
		repositoryID, digest, policyID,
	).Scan(&record).Error; err != nil {
		return signatureWorkflowReadRecord{}, err
	}
	if record.ID == "" {
		return signatureWorkflowReadRecord{}, gorm.ErrRecordNotFound
	}
	return record, nil
}

func latestSignatureWorkflowByTarget(
	database *gorm.DB,
	repositoryID, digest string,
) (signatureWorkflowReadRecord, error) {
	var record signatureWorkflowReadRecord
	if err := database.Raw(
		`SELECT workflow.*, n.id AS namespace_id, n.name AS namespace, r.name AS repository
		 FROM signature_workflows AS workflow
		 JOIN repositories AS r ON r.id = workflow.repository_id
		 JOIN namespaces AS n ON n.id = r.namespace_id
		 WHERE workflow.repository_id = ? AND workflow.digest = ?
		 ORDER BY workflow.policy_version DESC
		 LIMIT 1`,
		repositoryID, digest,
	).Scan(&record).Error; err != nil {
		return signatureWorkflowReadRecord{}, err
	}
	if record.ID == "" {
		return signatureWorkflowReadRecord{}, gorm.ErrRecordNotFound
	}
	return record, nil
}

func signatureWorkflowFromRecord(record signatureWorkflowReadRecord) (security.VerificationWorkflow, error) {
	target, err := security.NewTarget(record.RepositoryID, record.Namespace, record.Repository, record.Digest)
	if err != nil {
		return security.VerificationWorkflow{}, err
	}
	workflow := security.VerificationWorkflow{
		ID: record.ID, Target: target, PolicyID: record.PolicyID,
		PolicyVersion: record.PolicyVersion, JobID: record.JobID, CreatedAt: record.CreatedAt.UTC(),
	}
	if workflow.Validate() != nil {
		return security.VerificationWorkflow{}, security.ErrInvalid
	}
	return workflow, nil
}

func candidateSubjectsByNamespace(
	database *gorm.DB,
	namespaceID string,
) ([]security.PublicKeyTrust, []security.KeylessIdentity, error) {
	var keyRecords []trustPolicyPublicKeyRecord
	if err := database.Raw(
		`SELECT DISTINCT ON (key.fingerprint) key.*
		 FROM trust_policy_public_keys AS key
		 JOIN trust_policies AS policy ON policy.id = key.policy_id
		 WHERE policy.namespace_id = ?
		 ORDER BY key.fingerprint, policy.version DESC`, namespaceID,
	).Scan(&keyRecords).Error; err != nil {
		return nil, nil, err
	}
	var identityRecords []trustPolicyIdentityRecord
	if err := database.Raw(
		`SELECT DISTINCT ON (identity.issuer, identity.subject) identity.*
		 FROM trust_policy_identities AS identity
		 JOIN trust_policies AS policy ON policy.id = identity.policy_id
		 WHERE policy.namespace_id = ?
		 ORDER BY identity.issuer, identity.subject, policy.version DESC`, namespaceID,
	).Scan(&identityRecords).Error; err != nil {
		return nil, nil, err
	}
	keys := make([]security.PublicKeyTrust, 0, len(keyRecords))
	for _, record := range keyRecords {
		keys = append(keys, security.PublicKeyTrust{
			Fingerprint: record.Fingerprint, Name: record.Name, PublicKeyPEM: record.PublicKeyPEM,
		})
	}
	identities := make([]security.KeylessIdentity, 0, len(identityRecords))
	for _, record := range identityRecords {
		identities = append(identities, security.KeylessIdentity{Issuer: record.Issuer, Subject: record.Subject})
	}
	return keys, identities, nil
}

var _ security.TrustStore = (*Store)(nil)
