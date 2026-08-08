package securitystore

import "time"

type workflowRecord struct {
	ID           string
	RepositoryID string
	Digest       string
	ScanJobID    string
	SBOMJobID    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (workflowRecord) TableName() string { return "security_workflows" }

type workflowReadRecord struct {
	ID           string
	RepositoryID string
	Digest       string
	ScanJobID    string
	SBOMJobID    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Namespace    string
	Repository   string
}

type scanReportRecord struct {
	WorkflowID           string
	ScannerVersion       string
	DatabaseSchema       int
	DatabaseUpdatedAt    time.Time
	DatabaseDownloadedAt time.Time
	CompletedAt          time.Time
	UpdatedAt            time.Time
}

func (scanReportRecord) TableName() string { return "security_scan_reports" }

type findingRecord struct {
	ID               string
	WorkflowID       string
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

func (findingRecord) TableName() string { return "vulnerability_findings" }

type sbomRecord struct {
	WorkflowID       string
	GeneratorVersion string
	Format           string
	Document         []byte
	CompletedAt      time.Time
	UpdatedAt        time.Time
}

func (sbomRecord) TableName() string { return "security_sboms" }

type toolStateRecord struct {
	ScannerName          string
	ScannerVersion       string
	DatabaseSchema       int
	DatabaseUpdatedAt    time.Time
	DatabaseDownloadedAt time.Time
	ObservedAt           time.Time
}

func (toolStateRecord) TableName() string { return "security_tool_state" }
