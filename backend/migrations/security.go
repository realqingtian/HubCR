package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type securityWorkflowMigrationRecord struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	RepositoryID string    `gorm:"type:uuid;not null;uniqueIndex:uq_security_workflows_target,priority:1"`
	Digest       string    `gorm:"type:varchar(71);not null;uniqueIndex:uq_security_workflows_target,priority:2"`
	ScanJobID    string    `gorm:"type:uuid;not null;uniqueIndex:uq_security_workflows_scan_job"`
	SBOMJobID    string    `gorm:"type:uuid;not null;uniqueIndex:uq_security_workflows_sbom_job"`
	CreatedAt    time.Time `gorm:"type:timestamptz(6);not null"`
	UpdatedAt    time.Time `gorm:"type:timestamptz(6);not null"`
}

func (securityWorkflowMigrationRecord) TableName() string { return "security_workflows" }

type securityScanReportMigrationRecord struct {
	WorkflowID           string    `gorm:"type:uuid;primaryKey"`
	ScannerVersion       string    `gorm:"type:varchar(128);not null"`
	DatabaseSchema       int       `gorm:"type:integer;not null"`
	DatabaseUpdatedAt    time.Time `gorm:"type:timestamptz(6);not null"`
	DatabaseDownloadedAt time.Time `gorm:"type:timestamptz(6);not null"`
	CompletedAt          time.Time `gorm:"type:timestamptz(6);not null"`
	UpdatedAt            time.Time `gorm:"type:timestamptz(6);not null"`
}

func (securityScanReportMigrationRecord) TableName() string { return "security_scan_reports" }

type vulnerabilityFindingMigrationRecord struct {
	ID               string     `gorm:"type:uuid;primaryKey"`
	WorkflowID       string     `gorm:"type:uuid;not null;index:idx_vulnerability_findings_workflow"`
	Target           string     `gorm:"type:varchar(4096);not null"`
	Class            string     `gorm:"type:varchar(4096);not null"`
	Type             string     `gorm:"type:varchar(4096);not null"`
	VulnerabilityID  string     `gorm:"type:varchar(4096);not null"`
	PackageName      string     `gorm:"type:varchar(4096);not null"`
	InstalledVersion string     `gorm:"type:varchar(4096);not null"`
	FixedVersion     string     `gorm:"type:varchar(4096);not null"`
	Status           string     `gorm:"type:varchar(4096);not null"`
	Severity         string     `gorm:"type:varchar(16);not null;index:idx_vulnerability_findings_severity"`
	SeveritySource   string     `gorm:"type:varchar(4096);not null"`
	PrimaryURL       string     `gorm:"type:varchar(4096);not null"`
	DataSourceID     string     `gorm:"type:varchar(4096);not null"`
	Title            string     `gorm:"type:varchar(4096);not null"`
	PublishedAt      *time.Time `gorm:"type:timestamptz(6)"`
	ModifiedAt       *time.Time `gorm:"type:timestamptz(6)"`
}

func (vulnerabilityFindingMigrationRecord) TableName() string { return "vulnerability_findings" }

type securitySBOMMigrationRecord struct {
	WorkflowID       string    `gorm:"type:uuid;primaryKey"`
	GeneratorVersion string    `gorm:"type:varchar(128);not null"`
	Format           string    `gorm:"type:varchar(32);not null"`
	Document         []byte    `gorm:"type:jsonb;not null"`
	CompletedAt      time.Time `gorm:"type:timestamptz(6);not null"`
	UpdatedAt        time.Time `gorm:"type:timestamptz(6);not null"`
}

func (securitySBOMMigrationRecord) TableName() string { return "security_sboms" }

type securityToolStateMigrationRecord struct {
	ScannerName          string    `gorm:"type:varchar(32);primaryKey"`
	ScannerVersion       string    `gorm:"type:varchar(128);not null"`
	DatabaseSchema       int       `gorm:"type:integer;not null"`
	DatabaseUpdatedAt    time.Time `gorm:"type:timestamptz(6);not null"`
	DatabaseDownloadedAt time.Time `gorm:"type:timestamptz(6);not null"`
	ObservedAt           time.Time `gorm:"type:timestamptz(6);not null"`
}

func (securityToolStateMigrationRecord) TableName() string { return "security_tool_state" }

func securityScanMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000008_security_scan",
		Migrate: func(database *gorm.DB) error {
			if err := database.Migrator().CreateTable(
				&securityWorkflowMigrationRecord{},
				&securityScanReportMigrationRecord{},
				&vulnerabilityFindingMigrationRecord{},
				&securitySBOMMigrationRecord{},
				&securityToolStateMigrationRecord{},
			); err != nil {
				return err
			}
			statements := []string{
				"ALTER TABLE security_workflows ADD CONSTRAINT fk_security_workflows_artifact FOREIGN KEY (repository_id, digest) REFERENCES artifacts(repository_id, digest) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE security_workflows ADD CONSTRAINT fk_security_workflows_scan_job FOREIGN KEY (scan_job_id) REFERENCES jobs(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE security_workflows ADD CONSTRAINT fk_security_workflows_sbom_job FOREIGN KEY (sbom_job_id) REFERENCES jobs(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE security_workflows ADD CONSTRAINT ck_security_workflows_jobs_distinct CHECK (scan_job_id <> sbom_job_id)",
				"ALTER TABLE security_workflows ADD CONSTRAINT ck_security_workflows_timestamps CHECK (updated_at >= created_at)",
				"ALTER TABLE security_scan_reports ADD CONSTRAINT fk_security_scan_reports_workflow FOREIGN KEY (workflow_id) REFERENCES security_workflows(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE security_scan_reports ADD CONSTRAINT ck_security_scan_reports_version CHECK (scanner_version ~ '^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$' AND database_schema >= 1)",
				"ALTER TABLE security_scan_reports ADD CONSTRAINT ck_security_scan_reports_timestamps CHECK (database_downloaded_at >= database_updated_at AND updated_at >= completed_at)",
				"ALTER TABLE vulnerability_findings ADD CONSTRAINT fk_vulnerability_findings_workflow FOREIGN KEY (workflow_id) REFERENCES security_workflows(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE vulnerability_findings ADD CONSTRAINT ck_vulnerability_findings_required CHECK (char_length(target) BETWEEN 1 AND 4096 AND char_length(class) BETWEEN 1 AND 4096 AND char_length(type) BETWEEN 1 AND 4096 AND char_length(vulnerability_id) BETWEEN 1 AND 4096 AND char_length(package_name) BETWEEN 1 AND 4096 AND char_length(installed_version) BETWEEN 1 AND 4096 AND severity IN ('UNKNOWN','LOW','MEDIUM','HIGH','CRITICAL'))",
				"ALTER TABLE security_sboms ADD CONSTRAINT fk_security_sboms_workflow FOREIGN KEY (workflow_id) REFERENCES security_workflows(id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE security_sboms ADD CONSTRAINT ck_security_sboms_document CHECK (format = 'CYCLONEDX_JSON' AND jsonb_typeof(document) = 'object' AND document->>'bomFormat' = 'CycloneDX' AND octet_length(document::text) <= 16777216)",
				"ALTER TABLE security_sboms ADD CONSTRAINT ck_security_sboms_version CHECK (generator_version ~ '^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$' AND updated_at >= completed_at)",
				"ALTER TABLE security_tool_state ADD CONSTRAINT ck_security_tool_state_name CHECK (scanner_name = 'TRIVY')",
				"ALTER TABLE security_tool_state ADD CONSTRAINT ck_security_tool_state_version CHECK (scanner_version ~ '^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$' AND database_schema >= 1)",
				"ALTER TABLE security_tool_state ADD CONSTRAINT ck_security_tool_state_timestamps CHECK (database_downloaded_at >= database_updated_at AND observed_at >= database_downloaded_at)",
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
				&securityToolStateMigrationRecord{},
				&securitySBOMMigrationRecord{},
				&vulnerabilityFindingMigrationRecord{},
				&securityScanReportMigrationRecord{},
				&securityWorkflowMigrationRecord{},
			)
		},
	}
}
