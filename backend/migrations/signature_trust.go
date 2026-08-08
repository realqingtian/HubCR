package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type trustPolicyMigrationRecord struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	NamespaceID     string    `gorm:"type:uuid;not null;uniqueIndex:uq_trust_policies_namespace_version,priority:1"`
	Version         int64     `gorm:"type:bigint;not null;uniqueIndex:uq_trust_policies_namespace_version,priority:2"`
	CreatedByUserID string    `gorm:"type:uuid;not null;index:idx_trust_policies_created_by"`
	CreatedAt       time.Time `gorm:"type:timestamptz(6);not null"`
}

func (trustPolicyMigrationRecord) TableName() string { return "trust_policies" }

type trustPolicyPublicKeyMigrationRecord struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	PolicyID     string    `gorm:"type:uuid;not null;uniqueIndex:uq_trust_policy_public_keys,priority:1"`
	Fingerprint  string    `gorm:"type:varchar(71);not null;uniqueIndex:uq_trust_policy_public_keys,priority:2"`
	Name         string    `gorm:"type:varchar(128);not null"`
	PublicKeyPEM string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"type:timestamptz(6);not null"`
}

func (trustPolicyPublicKeyMigrationRecord) TableName() string { return "trust_policy_public_keys" }

type trustPolicyIdentityMigrationRecord struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	PolicyID  string    `gorm:"type:uuid;not null;uniqueIndex:uq_trust_policy_identities,priority:1"`
	Issuer    string    `gorm:"type:varchar(2048);not null;uniqueIndex:uq_trust_policy_identities,priority:2"`
	Subject   string    `gorm:"type:varchar(2048);not null;uniqueIndex:uq_trust_policy_identities,priority:3"`
	CreatedAt time.Time `gorm:"type:timestamptz(6);not null"`
}

func (trustPolicyIdentityMigrationRecord) TableName() string { return "trust_policy_identities" }

type signatureWorkflowMigrationRecord struct {
	ID            string    `gorm:"type:uuid;primaryKey"`
	RepositoryID  string    `gorm:"type:uuid;not null;uniqueIndex:uq_signature_workflows_target_policy,priority:1"`
	Digest        string    `gorm:"type:varchar(71);not null;uniqueIndex:uq_signature_workflows_target_policy,priority:2"`
	PolicyID      string    `gorm:"type:uuid;not null;uniqueIndex:uq_signature_workflows_target_policy,priority:3"`
	PolicyVersion int64     `gorm:"type:bigint;not null"`
	JobID         string    `gorm:"type:uuid;not null;uniqueIndex:uq_signature_workflows_job"`
	CreatedAt     time.Time `gorm:"type:timestamptz(6);not null"`
}

func (signatureWorkflowMigrationRecord) TableName() string { return "signature_workflows" }

type signatureVerificationMigrationRecord struct {
	WorkflowID    string    `gorm:"type:uuid;primaryKey"`
	CosignVersion string    `gorm:"type:varchar(128);not null"`
	CompletedAt   time.Time `gorm:"type:timestamptz(6);not null"`
	UpdatedAt     time.Time `gorm:"type:timestamptz(6);not null"`
}

func (signatureVerificationMigrationRecord) TableName() string { return "signature_verifications" }

type signatureEvidenceMigrationRecord struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	WorkflowID         string    `gorm:"type:uuid;not null;uniqueIndex:uq_signature_evidence_workflow_kind_digest,priority:1"`
	Kind               string    `gorm:"type:varchar(16);not null;uniqueIndex:uq_signature_evidence_workflow_kind_digest,priority:2"`
	SignatureDigest    string    `gorm:"type:varchar(71);not null;uniqueIndex:uq_signature_evidence_workflow_kind_digest,priority:3"`
	SignerType         string    `gorm:"type:varchar(16);not null"`
	KeyFingerprint     string    `gorm:"type:varchar(71);not null"`
	OIDCIssuer         string    `gorm:"column:oidc_issuer;type:varchar(2048);not null"`
	Subject            string    `gorm:"type:varchar(2048);not null"`
	CryptographicState string    `gorm:"type:varchar(16);not null"`
	TrustState         string    `gorm:"type:varchar(16);not null"`
	Reason             string    `gorm:"type:varchar(128);not null"`
	VerifiedAt         time.Time `gorm:"type:timestamptz(6);not null"`
}

func (signatureEvidenceMigrationRecord) TableName() string { return "signature_evidence" }

type cosignToolStateMigrationRecord struct {
	Name       string    `gorm:"type:varchar(32);primaryKey"`
	Version    string    `gorm:"type:varchar(128);not null"`
	ObservedAt time.Time `gorm:"type:timestamptz(6);not null"`
}

func (cosignToolStateMigrationRecord) TableName() string { return "cosign_tool_state" }

func signatureTrustMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000009_signature_trust",
		Migrate: func(database *gorm.DB) error {
			if err := database.Migrator().CreateTable(
				&trustPolicyMigrationRecord{},
				&trustPolicyPublicKeyMigrationRecord{},
				&trustPolicyIdentityMigrationRecord{},
				&signatureWorkflowMigrationRecord{},
				&signatureVerificationMigrationRecord{},
				&signatureEvidenceMigrationRecord{},
				&cosignToolStateMigrationRecord{},
			); err != nil {
				return err
			}
			statements := []string{
				"ALTER TABLE trust_policies ADD CONSTRAINT fk_trust_policies_namespace FOREIGN KEY (namespace_id) REFERENCES namespaces(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE trust_policies ADD CONSTRAINT fk_trust_policies_creator FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE trust_policies ADD CONSTRAINT ck_trust_policies_version CHECK (version >= 1)",
				"ALTER TABLE trust_policy_public_keys ADD CONSTRAINT fk_trust_policy_public_keys_policy FOREIGN KEY (policy_id) REFERENCES trust_policies(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE trust_policy_public_keys ADD CONSTRAINT ck_trust_policy_public_keys_fingerprint CHECK (fingerprint ~ '^sha256:[0-9a-f]{64}$')",
				"ALTER TABLE trust_policy_public_keys ADD CONSTRAINT ck_trust_policy_public_keys_content CHECK (char_length(name) BETWEEN 1 AND 128 AND octet_length(public_key_pem) BETWEEN 1 AND 16384 AND public_key_pem LIKE '-----BEGIN PUBLIC KEY-----%')",
				"ALTER TABLE trust_policy_identities ADD CONSTRAINT fk_trust_policy_identities_policy FOREIGN KEY (policy_id) REFERENCES trust_policies(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE trust_policy_identities ADD CONSTRAINT ck_trust_policy_identities_exact CHECK (char_length(issuer) BETWEEN 9 AND 2048 AND issuer LIKE 'https://%' AND issuer NOT LIKE '%*%' AND char_length(subject) BETWEEN 1 AND 2048 AND subject NOT LIKE '%*%')",
				"ALTER TABLE signature_workflows ADD CONSTRAINT fk_signature_workflows_artifact FOREIGN KEY (repository_id, digest) REFERENCES artifacts(repository_id, digest) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE signature_workflows ADD CONSTRAINT fk_signature_workflows_policy FOREIGN KEY (policy_id) REFERENCES trust_policies(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE signature_workflows ADD CONSTRAINT fk_signature_workflows_job FOREIGN KEY (job_id) REFERENCES jobs(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE signature_workflows ADD CONSTRAINT ck_signature_workflows_policy_version CHECK (policy_version >= 1)",
				"ALTER TABLE signature_verifications ADD CONSTRAINT fk_signature_verifications_workflow FOREIGN KEY (workflow_id) REFERENCES signature_workflows(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE signature_verifications ADD CONSTRAINT ck_signature_verifications_version CHECK (cosign_version ~ '^v?[0-9A-Za-z][0-9A-Za-z._+-]{0,126}$' AND updated_at >= completed_at)",
				"ALTER TABLE signature_evidence ADD CONSTRAINT fk_signature_evidence_workflow FOREIGN KEY (workflow_id) REFERENCES signature_workflows(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE signature_evidence ADD CONSTRAINT ck_signature_evidence_digest CHECK (signature_digest ~ '^sha256:[0-9a-f]{64}$')",
				"ALTER TABLE signature_evidence ADD CONSTRAINT ck_signature_evidence_kind CHECK (kind IN ('SIGNATURE','ATTESTATION'))",
				"ALTER TABLE signature_evidence ADD CONSTRAINT ck_signature_evidence_signer CHECK ((signer_type = 'PUBLIC_KEY' AND key_fingerprint ~ '^sha256:[0-9a-f]{64}$' AND oidc_issuer = '' AND subject = '') OR (signer_type = 'KEYLESS' AND key_fingerprint = '' AND oidc_issuer <> '' AND subject <> '') OR (signer_type = 'UNKNOWN' AND key_fingerprint = '' AND oidc_issuer = '' AND subject = ''))",
				"ALTER TABLE signature_evidence ADD CONSTRAINT ck_signature_evidence_state CHECK (cryptographic_state IN ('VALID','INVALID','UNVERIFIED','UNAVAILABLE') AND trust_state IN ('TRUSTED','UNTRUSTED','NOT_EVALUATED') AND ((cryptographic_state = 'VALID' AND trust_state IN ('TRUSTED','UNTRUSTED')) OR (cryptographic_state <> 'VALID' AND trust_state = 'NOT_EVALUATED')))",
				"ALTER TABLE signature_evidence ADD CONSTRAINT ck_signature_evidence_reason CHECK (char_length(reason) BETWEEN 1 AND 128)",
				"ALTER TABLE cosign_tool_state ADD CONSTRAINT ck_cosign_tool_state CHECK (name = 'COSIGN' AND version ~ '^v?[0-9A-Za-z][0-9A-Za-z._+-]{0,126}$')",
			}
			for _, statement := range statements {
				if err := database.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(database *gorm.DB) error {
			return database.Migrator().DropTable(
				&cosignToolStateMigrationRecord{},
				&signatureEvidenceMigrationRecord{},
				&signatureVerificationMigrationRecord{},
				&signatureWorkflowMigrationRecord{},
				&trustPolicyIdentityMigrationRecord{},
				&trustPolicyPublicKeyMigrationRecord{},
				&trustPolicyMigrationRecord{},
			)
		},
	}
}
