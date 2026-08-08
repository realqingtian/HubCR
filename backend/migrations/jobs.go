package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type jobMigrationRecord struct {
	ID             string     `gorm:"type:uuid;primaryKey"`
	Kind           string     `gorm:"type:varchar(64);not null"`
	IntentKey      string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_jobs_intent_key"`
	Payload        []byte     `gorm:"type:jsonb;not null"`
	State          string     `gorm:"type:varchar(16);not null"`
	AttemptCount   int        `gorm:"type:integer;not null"`
	MaxAttempts    int        `gorm:"type:integer;not null"`
	AvailableAt    time.Time  `gorm:"type:timestamptz(6);not null"`
	LeaseOwner     *string    `gorm:"type:varchar(128)"`
	LeaseExpiresAt *time.Time `gorm:"type:timestamptz(6)"`
	LastErrorCode  *string    `gorm:"type:varchar(64)"`
	CreatedAt      time.Time  `gorm:"type:timestamptz(6);not null"`
	UpdatedAt      time.Time  `gorm:"type:timestamptz(6);not null"`
	StartedAt      *time.Time `gorm:"type:timestamptz(6)"`
	CompletedAt    *time.Time `gorm:"type:timestamptz(6)"`
}

func (jobMigrationRecord) TableName() string { return "jobs" }

func jobFoundationMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000007_job_foundation",
		Migrate: func(database *gorm.DB) error {
			if err := database.Migrator().CreateTable(&jobMigrationRecord{}); err != nil {
				return err
			}
			statements := []string{
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_kind CHECK (kind ~ '^[A-Z][A-Z0-9_]{0,63}$')",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_intent_key CHECK (char_length(intent_key) BETWEEN 1 AND 255 AND intent_key !~ '[[:space:]]')",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_payload CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 65536)",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_state CHECK (state IN ('QUEUED','RUNNING','SUCCEEDED','DEAD'))",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_attempts CHECK (attempt_count BETWEEN 0 AND max_attempts AND max_attempts BETWEEN 1 AND 100)",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_error_code CHECK (last_error_code IS NULL OR last_error_code ~ '^[A-Z][A-Z0-9_]{0,63}$')",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_timestamps CHECK (updated_at >= created_at AND (started_at IS NULL OR started_at >= created_at) AND (completed_at IS NULL OR completed_at >= created_at))",
				"ALTER TABLE jobs ADD CONSTRAINT ck_jobs_lifecycle CHECK ((state = 'QUEUED' AND lease_owner IS NULL AND lease_expires_at IS NULL AND completed_at IS NULL) OR (state = 'RUNNING' AND char_length(lease_owner) BETWEEN 1 AND 128 AND lease_expires_at > updated_at AND started_at IS NOT NULL AND completed_at IS NULL AND attempt_count > 0) OR (state IN ('SUCCEEDED','DEAD') AND lease_owner IS NULL AND lease_expires_at IS NULL AND completed_at IS NOT NULL))",
				"CREATE INDEX idx_jobs_claim_queued ON jobs (available_at, created_at, id) WHERE state = 'QUEUED'",
				"CREATE INDEX idx_jobs_claim_expired ON jobs (lease_expires_at, created_at, id) WHERE state = 'RUNNING'",
			}
			for _, statement := range statements {
				if err := database.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(database *gorm.DB) error {
			return database.Migrator().DropTable(&jobMigrationRecord{})
		},
	}
}
