package securitystore

import "time"

type trustPolicyRecord struct {
	ID              string
	NamespaceID     string
	Version         int64
	CreatedByUserID string
	CreatedAt       time.Time
}

func (trustPolicyRecord) TableName() string { return "trust_policies" }

type trustPolicyPublicKeyRecord struct {
	ID           string
	PolicyID     string
	Fingerprint  string
	Name         string
	PublicKeyPEM string
	CreatedAt    time.Time
}

func (trustPolicyPublicKeyRecord) TableName() string { return "trust_policy_public_keys" }

type trustPolicyIdentityRecord struct {
	ID        string
	PolicyID  string
	Issuer    string
	Subject   string
	CreatedAt time.Time
}

func (trustPolicyIdentityRecord) TableName() string { return "trust_policy_identities" }

type signatureWorkflowRecord struct {
	ID            string
	RepositoryID  string
	Digest        string
	PolicyID      string
	PolicyVersion int64
	JobID         string
	CreatedAt     time.Time
}

func (signatureWorkflowRecord) TableName() string { return "signature_workflows" }

type signatureWorkflowReadRecord struct {
	ID            string
	RepositoryID  string
	Digest        string
	PolicyID      string
	PolicyVersion int64
	JobID         string
	CreatedAt     time.Time
	Namespace     string
	Repository    string
	NamespaceID   string
}

type signatureVerificationRecord struct {
	WorkflowID    string
	CosignVersion string
	CompletedAt   time.Time
	UpdatedAt     time.Time
}

func (signatureVerificationRecord) TableName() string { return "signature_verifications" }

type signatureEvidenceRecord struct {
	ID                 string
	WorkflowID         string
	Kind               string
	SignatureDigest    string
	SignerType         string
	KeyFingerprint     string
	OIDCIssuer         string `gorm:"column:oidc_issuer"`
	Subject            string
	CryptographicState string
	TrustState         string
	Reason             string
	VerifiedAt         time.Time
}

func (signatureEvidenceRecord) TableName() string { return "signature_evidence" }

type cosignToolStateRecord struct {
	Name       string
	Version    string
	ObservedAt time.Time
}

func (cosignToolStateRecord) TableName() string { return "cosign_tool_state" }
