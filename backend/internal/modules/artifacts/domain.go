package artifacts

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	MaxPageSize      = 100
	MaxMediaTypeSize = 255
	MaxPlatformSize  = 64
)

var (
	ErrInvalidDigest   = errors.New("invalid artifact digest")
	ErrInvalidTag      = errors.New("invalid artifact tag")
	ErrInvalidArtifact = errors.New("invalid artifact")
	ErrConflict        = errors.New("artifact conflicts with existing data")
	ErrNotFound        = errors.New("artifact not found")
	ErrUnavailable     = errors.New("artifact persistence unavailable")

	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	tagPattern    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

type Digest string

func ParseDigest(value string) (Digest, error) {
	if !digestPattern.MatchString(value) {
		return "", ErrInvalidDigest
	}
	return Digest(value), nil
}

func (d Digest) String() string { return string(d) }

type TagName string

func ParseTagName(value string) (TagName, error) {
	if !tagPattern.MatchString(value) {
		return "", ErrInvalidTag
	}
	return TagName(value), nil
}

func (t TagName) String() string { return string(t) }

type Kind string

const (
	KindManifest Kind = "MANIFEST"
	KindIndex    Kind = "INDEX"
)

func ParseKind(value string) (Kind, error) {
	kind := Kind(value)
	switch kind {
	case KindManifest, KindIndex:
		return kind, nil
	default:
		return "", ErrInvalidArtifact
	}
}

type Platform struct {
	OS           string
	Architecture string
	Variant      string
}

type Observation struct {
	RepositoryID    string
	Digest          string
	Kind            string
	MediaType       *string
	SizeBytes       *int64
	SourceCreatedAt *time.Time
	Descriptors     *DescriptorSetObservation
	Tag             *string
	ObservedAt      time.Time
}

type DescriptorSetObservation struct {
	Items []DescriptorObservation
}

type DescriptorObservation struct {
	Digest    string
	MediaType *string
	SizeBytes *int64
	Platform  *Platform
}

type ArtifactObservation struct {
	Digest          Digest
	Kind            Kind
	MediaType       *string
	SizeBytes       *int64
	SourceCreatedAt *time.Time
}

type DescriptorSet struct {
	Items []Descriptor
}

type Descriptor struct {
	Digest    Digest
	MediaType *string
	SizeBytes *int64
	Platform  *Platform
}

type Reconciliation struct {
	RepositoryID string
	Artifact     ArtifactObservation
	Descriptors  *DescriptorSet
	Tag          *TagName
	ObservedAt   time.Time
}

type Artifact struct {
	ID                  string
	RepositoryID        string
	Digest              Digest
	Kind                Kind
	MediaType           *string
	SizeBytes           *int64
	SourceCreatedAt     *time.Time
	DescriptorsComplete bool
	DiscoveredAt        time.Time
	UpdatedAt           time.Time
}

type Tag struct {
	RepositoryID string
	Name         TagName
	ArtifactID   string
	Digest       Digest
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ManifestDescriptor struct {
	Position        int
	ChildArtifactID string
	Digest          Digest
	MediaType       *string
	SizeBytes       *int64
	Platform        *Platform
}

type Snapshot struct {
	Artifact    Artifact
	Tag         *Tag
	Descriptors []ManifestDescriptor
}

type PageRequest struct {
	Limit int
	After string
}

type ArtifactPage struct {
	Items     []Artifact
	NextAfter string
}

type TagPage struct {
	Items     []Tag
	NextAfter string
}

func NormalizeObservation(input Observation) (Reconciliation, error) {
	if input.RepositoryID == "" || input.ObservedAt.IsZero() {
		return Reconciliation{}, ErrInvalidArtifact
	}
	digest, err := ParseDigest(input.Digest)
	if err != nil {
		return Reconciliation{}, err
	}
	kind, err := ParseKind(input.Kind)
	if err != nil {
		return Reconciliation{}, err
	}
	mediaType, err := normalizeOptionalText(input.MediaType, MaxMediaTypeSize)
	if err != nil {
		return Reconciliation{}, ErrInvalidArtifact
	}
	size, err := normalizeSize(input.SizeBytes)
	if err != nil {
		return Reconciliation{}, ErrInvalidArtifact
	}
	createdAt := normalizeOptionalTime(input.SourceCreatedAt)

	var tag *TagName
	if input.Tag != nil {
		parsed, err := ParseTagName(*input.Tag)
		if err != nil {
			return Reconciliation{}, err
		}
		tag = &parsed
	}

	var descriptors *DescriptorSet
	if input.Descriptors != nil {
		if kind != KindIndex {
			return Reconciliation{}, ErrInvalidArtifact
		}
		items := make([]Descriptor, 0, len(input.Descriptors.Items))
		for _, item := range input.Descriptors.Items {
			childDigest, err := ParseDigest(item.Digest)
			if err != nil {
				return Reconciliation{}, err
			}
			if childDigest == digest {
				return Reconciliation{}, ErrInvalidArtifact
			}
			childMediaType, err := normalizeOptionalText(item.MediaType, MaxMediaTypeSize)
			if err != nil {
				return Reconciliation{}, ErrInvalidArtifact
			}
			childSize, err := normalizeSize(item.SizeBytes)
			if err != nil {
				return Reconciliation{}, ErrInvalidArtifact
			}
			platform, err := normalizePlatform(item.Platform)
			if err != nil {
				return Reconciliation{}, ErrInvalidArtifact
			}
			items = append(items, Descriptor{
				Digest: childDigest, MediaType: childMediaType,
				SizeBytes: childSize, Platform: platform,
			})
		}
		descriptors = &DescriptorSet{Items: items}
	}

	return Reconciliation{
		RepositoryID: input.RepositoryID,
		Artifact: ArtifactObservation{
			Digest: digest, Kind: kind, MediaType: mediaType,
			SizeBytes: size, SourceCreatedAt: createdAt,
		},
		Descriptors: descriptors,
		Tag:         tag,
		ObservedAt:  input.ObservedAt.UTC().Round(time.Microsecond),
	}, nil
}

func NewArtifactPageRequest(limit int, after string) (PageRequest, error) {
	if limit < 1 || limit > MaxPageSize {
		return PageRequest{}, ErrInvalidArtifact
	}
	if after != "" {
		if _, err := ParseDigest(after); err != nil {
			return PageRequest{}, err
		}
	}
	return PageRequest{Limit: limit, After: after}, nil
}

func NewTagPageRequest(limit int, after string) (PageRequest, error) {
	if limit < 1 || limit > MaxPageSize {
		return PageRequest{}, ErrInvalidArtifact
	}
	if after != "" {
		if _, err := ParseTagName(after); err != nil {
			return PageRequest{}, err
		}
	}
	return PageRequest{Limit: limit, After: after}, nil
}

func normalizeOptionalText(value *string, max int) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if *value == "" || len(*value) > max || strings.TrimSpace(*value) != *value {
		return nil, ErrInvalidArtifact
	}
	for _, character := range *value {
		if character < 0x20 || character == 0x7f {
			return nil, ErrInvalidArtifact
		}
	}
	copy := *value
	return &copy, nil
}

func normalizeSize(value *int64) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if *value < 0 {
		return nil, ErrInvalidArtifact
	}
	copy := *value
	return &copy, nil
}

func normalizeOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC().Round(time.Microsecond)
	return &copy
}

func normalizePlatform(value *Platform) (*Platform, error) {
	if value == nil {
		return nil, nil
	}
	if !validPlatformValue(value.OS) || !validPlatformValue(value.Architecture) ||
		(value.Variant != "" && !validPlatformValue(value.Variant)) {
		return nil, ErrInvalidArtifact
	}
	copy := *value
	return &copy, nil
}

func validPlatformValue(value string) bool {
	if value == "" || len(value) > MaxPlatformSize || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
