package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type artifactMigrationRecord struct {
	ID           string                    `gorm:"type:uuid;primaryKey;uniqueIndex:uq_artifacts_repository_id_id,priority:2"`
	RepositoryID string                    `gorm:"type:uuid;not null;uniqueIndex:uq_artifacts_repository_digest,priority:1;uniqueIndex:uq_artifacts_repository_id_id,priority:1"`
	Repository   repositoryMigrationRecord `gorm:"foreignKey:RepositoryID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Digest       string                    `gorm:"type:varchar(71);not null;uniqueIndex:uq_artifacts_repository_digest,priority:2;check:ck_artifacts_digest,digest ~ '^sha256:[0-9a-f]{64}$'"`
	Kind         string                    `gorm:"type:varchar(16);not null;check:ck_artifacts_kind,kind IN ('MANIFEST','INDEX')"`
	MediaType    *string                   `gorm:"type:varchar(255);check:ck_artifacts_media_type,media_type IS NULL OR char_length(media_type) > 0"`
	SizeBytes    *int64                    `gorm:"type:bigint;check:ck_artifacts_size,size_bytes IS NULL OR size_bytes >= 0"`

	SourceCreatedAt     *time.Time `gorm:"type:timestamptz(6)"`
	DescriptorsComplete bool       `gorm:"not null;default:false;check:ck_artifacts_descriptor_kind,kind = 'INDEX' OR descriptors_complete = false"`
	DiscoveredAt        time.Time  `gorm:"type:timestamptz(6);not null"`
	UpdatedAt           time.Time  `gorm:"type:timestamptz(6);not null;check:ck_artifacts_timestamps,updated_at >= discovered_at"`
}

func (artifactMigrationRecord) TableName() string { return "artifacts" }

type tagMigrationRecord struct {
	RepositoryID string    `gorm:"type:uuid;primaryKey"`
	Name         string    `gorm:"type:varchar(128);primaryKey;check:ck_tags_name,name ~ '^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$'"`
	ArtifactID   string    `gorm:"type:uuid;not null"`
	CreatedAt    time.Time `gorm:"type:timestamptz(6);not null"`
	UpdatedAt    time.Time `gorm:"type:timestamptz(6);not null;check:ck_tags_timestamps,updated_at >= created_at"`
}

func (tagMigrationRecord) TableName() string { return "tags" }

type manifestDescriptorMigrationRecord struct {
	RepositoryID    string  `gorm:"type:uuid;primaryKey"`
	IndexArtifactID string  `gorm:"type:uuid;primaryKey;check:ck_manifest_descriptors_distinct,index_artifact_id <> child_artifact_id"`
	Position        int     `gorm:"type:integer;primaryKey;check:ck_manifest_descriptors_position,position >= 0"`
	ChildArtifactID string  `gorm:"type:uuid;not null"`
	OS              *string `gorm:"type:varchar(64)"`
	Architecture    *string `gorm:"type:varchar(64)"`
	Variant         *string `gorm:"type:varchar(64);check:ck_manifest_descriptors_platform,(os IS NULL AND architecture IS NULL AND variant IS NULL) OR (os IS NOT NULL AND char_length(os) > 0 AND architecture IS NOT NULL AND char_length(architecture) > 0 AND (variant IS NULL OR char_length(variant) > 0))"`
}

func (manifestDescriptorMigrationRecord) TableName() string { return "manifest_descriptors" }

func artifactMetadataMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000006_artifact_metadata",
		Migrate: func(database *gorm.DB) error {
			if err := database.Migrator().CreateTable(&artifactMigrationRecord{}); err != nil {
				return err
			}
			if err := database.Migrator().CreateTable(
				&tagMigrationRecord{},
				&manifestDescriptorMigrationRecord{},
			); err != nil {
				return err
			}
			constraints := []string{
				"ALTER TABLE tags ADD CONSTRAINT fk_tags_artifact FOREIGN KEY (repository_id, artifact_id) REFERENCES artifacts(repository_id, id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE manifest_descriptors ADD CONSTRAINT fk_manifest_descriptors_index FOREIGN KEY (repository_id, index_artifact_id) REFERENCES artifacts(repository_id, id) ON UPDATE RESTRICT ON DELETE RESTRICT",
				"ALTER TABLE manifest_descriptors ADD CONSTRAINT fk_manifest_descriptors_child FOREIGN KEY (repository_id, child_artifact_id) REFERENCES artifacts(repository_id, id) ON UPDATE RESTRICT ON DELETE RESTRICT",
			}
			for _, statement := range constraints {
				if err := database.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(database *gorm.DB) error {
			return database.Migrator().DropTable(
				&manifestDescriptorMigrationRecord{},
				&tagMigrationRecord{},
				&artifactMigrationRecord{},
			)
		},
	}
}
