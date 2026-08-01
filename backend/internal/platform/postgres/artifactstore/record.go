package artifactstore

import "time"

type artifactRecord struct {
	ID                  string
	RepositoryID        string
	Digest              string
	Kind                string
	MediaType           *string
	SizeBytes           *int64
	SourceCreatedAt     *time.Time
	DescriptorsComplete bool
	DiscoveredAt        time.Time
	UpdatedAt           time.Time `gorm:"autoUpdateTime:false"`
}

func (artifactRecord) TableName() string { return "artifacts" }

type tagRecord struct {
	RepositoryID string
	Name         string
	ArtifactID   string
	CreatedAt    time.Time `gorm:"autoCreateTime:false"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime:false"`
}

func (tagRecord) TableName() string { return "tags" }

type manifestDescriptorRecord struct {
	RepositoryID    string
	IndexArtifactID string
	Position        int
	ChildArtifactID string
	OS              *string
	Architecture    *string
	Variant         *string
}

func (manifestDescriptorRecord) TableName() string { return "manifest_descriptors" }

type tagReadRecord struct {
	RepositoryID string
	Name         string
	ArtifactID   string
	Digest       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type descriptorReadRecord struct {
	Position        int
	ChildArtifactID string
	Digest          string
	MediaType       *string
	SizeBytes       *int64
	OS              *string
	Architecture    *string
	Variant         *string
}
