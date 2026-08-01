package artifacts

import (
	"context"
	"errors"
)

type Store interface {
	Reconcile(context.Context, Reconciliation) (Snapshot, error)
	RemoveTag(context.Context, string, TagName) error
}

type Reader interface {
	ArtifactByDigest(context.Context, string, Digest) (Artifact, error)
	TagByName(context.Context, string, TagName) (Tag, error)
	ListArtifacts(context.Context, string, PageRequest) (ArtifactPage, error)
	ListTags(context.Context, string, PageRequest) (TagPage, error)
	DescriptorsByIndex(context.Context, string, Digest) ([]ManifestDescriptor, error)
}

type Service struct{ store Store }

type QueryService struct{ reader Reader }

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("artifact Store must be configured")
	}
	return &Service{store: store}, nil
}

func NewQueryService(reader Reader) (*QueryService, error) {
	if reader == nil {
		return nil, errors.New("artifact Reader must be configured")
	}
	return &QueryService{reader: reader}, nil
}

func (s *Service) ReconcileArtifact(
	ctx context.Context,
	observation Observation,
) (Snapshot, error) {
	input, err := NormalizeObservation(observation)
	if err != nil {
		return Snapshot{}, err
	}
	return s.store.Reconcile(ctx, input)
}

func (s *Service) RemoveTag(ctx context.Context, repositoryID, rawTag string) error {
	if repositoryID == "" {
		return ErrInvalidArtifact
	}
	tag, err := ParseTagName(rawTag)
	if err != nil {
		return err
	}
	return s.store.RemoveTag(ctx, repositoryID, tag)
}

func (s *QueryService) ArtifactDetail(
	ctx context.Context,
	repositoryID, rawDigest string,
) (Snapshot, error) {
	digest, err := validateRepositoryDigest(repositoryID, rawDigest)
	if err != nil {
		return Snapshot{}, err
	}
	artifact, err := s.reader.ArtifactByDigest(ctx, repositoryID, digest)
	if err != nil {
		return Snapshot{}, err
	}
	if err := validateReadArtifact(repositoryID, digest, artifact); err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Artifact: artifact}
	if artifact.Kind == KindIndex && artifact.DescriptorsComplete {
		result.Descriptors, err = s.reader.DescriptorsByIndex(ctx, repositoryID, digest)
		if err != nil {
			return Snapshot{}, err
		}
		if result.Descriptors == nil {
			result.Descriptors = []ManifestDescriptor{}
		}
		for position, descriptor := range result.Descriptors {
			if err := validateReadDescriptor(digest, position, descriptor); err != nil {
				return Snapshot{}, err
			}
		}
	}
	return result, nil
}

func (s *QueryService) TagDetail(
	ctx context.Context,
	repositoryID, rawTag string,
) (Tag, error) {
	tag, err := validateRepositoryTag(repositoryID, rawTag)
	if err != nil {
		return Tag{}, err
	}
	result, err := s.reader.TagByName(ctx, repositoryID, tag)
	if err != nil {
		return Tag{}, err
	}
	if err := validateReadTag(repositoryID, result); err != nil || result.Name != tag {
		return Tag{}, ErrInvalidArtifact
	}
	return result, nil
}

func (s *QueryService) ListArtifacts(
	ctx context.Context,
	repositoryID string,
	limit int,
	after string,
) (ArtifactPage, error) {
	if repositoryID == "" {
		return ArtifactPage{}, ErrInvalidArtifact
	}
	page, err := NewArtifactPageRequest(limit, after)
	if err != nil {
		return ArtifactPage{}, err
	}
	result, err := s.reader.ListArtifacts(ctx, repositoryID, page)
	if err != nil {
		return ArtifactPage{}, err
	}
	for _, artifact := range result.Items {
		if err := validateReadArtifact(repositoryID, artifact.Digest, artifact); err != nil {
			return ArtifactPage{}, err
		}
	}
	if result.NextAfter != "" {
		if _, err := ParseDigest(result.NextAfter); err != nil {
			return ArtifactPage{}, ErrInvalidArtifact
		}
	}
	return result, nil
}

func (s *QueryService) ListTags(
	ctx context.Context,
	repositoryID string,
	limit int,
	after string,
) (TagPage, error) {
	if repositoryID == "" {
		return TagPage{}, ErrInvalidArtifact
	}
	page, err := NewTagPageRequest(limit, after)
	if err != nil {
		return TagPage{}, err
	}
	result, err := s.reader.ListTags(ctx, repositoryID, page)
	if err != nil {
		return TagPage{}, err
	}
	for _, tag := range result.Items {
		if err := validateReadTag(repositoryID, tag); err != nil {
			return TagPage{}, ErrInvalidArtifact
		}
	}
	if result.NextAfter != "" {
		if _, err := ParseTagName(result.NextAfter); err != nil {
			return TagPage{}, ErrInvalidArtifact
		}
	}
	return result, nil
}

func validateRepositoryDigest(repositoryID, rawDigest string) (Digest, error) {
	if repositoryID == "" {
		return "", ErrInvalidArtifact
	}
	return ParseDigest(rawDigest)
}

func validateRepositoryTag(repositoryID, rawTag string) (TagName, error) {
	if repositoryID == "" {
		return "", ErrInvalidArtifact
	}
	return ParseTagName(rawTag)
}

func validateReadArtifact(repositoryID string, digest Digest, artifact Artifact) error {
	if artifact.ID == "" || artifact.RepositoryID != repositoryID || artifact.Digest != digest {
		return ErrInvalidArtifact
	}
	if _, err := ParseKind(string(artifact.Kind)); err != nil {
		return ErrInvalidArtifact
	}
	if artifact.DiscoveredAt.IsZero() || artifact.UpdatedAt.IsZero() {
		return ErrInvalidArtifact
	}
	if artifact.UpdatedAt.Before(artifact.DiscoveredAt) ||
		(artifact.DescriptorsComplete && artifact.Kind != KindIndex) {
		return ErrInvalidArtifact
	}
	if _, err := normalizeOptionalText(artifact.MediaType, MaxMediaTypeSize); err != nil {
		return ErrInvalidArtifact
	}
	if _, err := normalizeSize(artifact.SizeBytes); err != nil {
		return ErrInvalidArtifact
	}
	if artifact.SourceCreatedAt != nil && artifact.SourceCreatedAt.IsZero() {
		return ErrInvalidArtifact
	}
	return nil
}

func validateReadTag(repositoryID string, tag Tag) error {
	if tag.RepositoryID != repositoryID || tag.ArtifactID == "" ||
		tag.CreatedAt.IsZero() || tag.UpdatedAt.IsZero() || tag.UpdatedAt.Before(tag.CreatedAt) {
		return ErrInvalidArtifact
	}
	if _, err := ParseTagName(tag.Name.String()); err != nil {
		return ErrInvalidArtifact
	}
	if _, err := ParseDigest(tag.Digest.String()); err != nil {
		return ErrInvalidArtifact
	}
	return nil
}

func validateReadDescriptor(parent Digest, position int, descriptor ManifestDescriptor) error {
	if descriptor.Position != position || descriptor.ChildArtifactID == "" ||
		descriptor.Digest == parent {
		return ErrInvalidArtifact
	}
	if _, err := ParseDigest(descriptor.Digest.String()); err != nil {
		return ErrInvalidArtifact
	}
	if _, err := normalizeOptionalText(descriptor.MediaType, MaxMediaTypeSize); err != nil {
		return ErrInvalidArtifact
	}
	if _, err := normalizeSize(descriptor.SizeBytes); err != nil {
		return ErrInvalidArtifact
	}
	if _, err := normalizePlatform(descriptor.Platform); err != nil {
		return ErrInvalidArtifact
	}
	return nil
}
