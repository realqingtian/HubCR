package artifactstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hubcr.io/hubcr/internal/modules/artifacts"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (s *Store) Reconcile(
	ctx context.Context,
	input artifacts.Reconciliation,
) (artifacts.Snapshot, error) {
	if input.RepositoryID == "" || input.ObservedAt.IsZero() {
		return artifacts.Snapshot{}, artifacts.ErrInvalidArtifact
	}
	if _, err := artifacts.ParseDigest(input.Artifact.Digest.String()); err != nil {
		return artifacts.Snapshot{}, err
	}
	if _, err := artifacts.ParseKind(string(input.Artifact.Kind)); err != nil {
		return artifacts.Snapshot{}, err
	}
	input.ObservedAt = input.ObservedAt.UTC().Round(time.Microsecond)
	input.Artifact.SourceCreatedAt = cloneTime(input.Artifact.SourceCreatedAt)

	var snapshot artifacts.Snapshot
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		parent, err := reconcileArtifact(transaction, input.RepositoryID, input.Artifact, input.ObservedAt)
		if err != nil {
			return err
		}
		if err := reconcileDescriptors(
			transaction,
			input.RepositoryID,
			&parent,
			input.Descriptors,
			input.ObservedAt,
		); err != nil {
			return err
		}

		artifact, err := artifactFromRecord(parent)
		if err != nil {
			return err
		}
		snapshot.Artifact = artifact
		if input.Tag != nil {
			tag, err := reconcileTag(
				transaction,
				input.RepositoryID,
				*input.Tag,
				parent,
				input.ObservedAt,
			)
			if err != nil {
				return err
			}
			snapshot.Tag = &tag
		}
		if parent.Kind == string(artifacts.KindIndex) && parent.DescriptorsComplete {
			descriptors, err := descriptorsByArtifactID(transaction, input.RepositoryID, parent.ID)
			if err != nil {
				return err
			}
			snapshot.Descriptors = descriptors
		}
		return nil
	})
	if err != nil {
		return artifacts.Snapshot{}, classify("reconcile artifact", err)
	}
	return snapshot, nil
}

func (s *Store) RemoveTag(ctx context.Context, repositoryID string, tag artifacts.TagName) error {
	if repositoryID == "" {
		return artifacts.ErrInvalidArtifact
	}
	if _, err := artifacts.ParseTagName(tag.String()); err != nil {
		return err
	}
	if err := s.database.WithContext(ctx).
		Where("repository_id = ? AND name = ?", repositoryID, tag.String()).
		Delete(&tagRecord{}).Error; err != nil {
		return classify("remove artifact tag", err)
	}
	return nil
}

func (s *Store) ArtifactByDigest(
	ctx context.Context,
	repositoryID string,
	digest artifacts.Digest,
) (artifacts.Artifact, error) {
	if err := validateRepositoryDigest(repositoryID, digest); err != nil {
		return artifacts.Artifact{}, err
	}
	var record artifactRecord
	if err := s.database.WithContext(ctx).
		Where("repository_id = ? AND digest = ?", repositoryID, digest.String()).
		First(&record).Error; err != nil {
		return artifacts.Artifact{}, classify("find artifact by digest", err)
	}
	artifact, err := artifactFromRecord(record)
	if err != nil {
		return artifacts.Artifact{}, classify("decode artifact", err)
	}
	return artifact, nil
}

func (s *Store) TagByName(
	ctx context.Context,
	repositoryID string,
	tag artifacts.TagName,
) (artifacts.Tag, error) {
	if repositoryID == "" {
		return artifacts.Tag{}, artifacts.ErrInvalidArtifact
	}
	if _, err := artifacts.ParseTagName(tag.String()); err != nil {
		return artifacts.Tag{}, err
	}
	var record tagReadRecord
	if err := tagReadQuery(s.database.WithContext(ctx)).
		Where("tags.repository_id = ? AND tags.name = ?", repositoryID, tag.String()).
		First(&record).Error; err != nil {
		return artifacts.Tag{}, classify("find artifact tag", err)
	}
	result, err := tagFromRecord(record)
	if err != nil {
		return artifacts.Tag{}, classify("decode artifact tag", err)
	}
	return result, nil
}

func (s *Store) ListArtifacts(
	ctx context.Context,
	repositoryID string,
	page artifacts.PageRequest,
) (artifacts.ArtifactPage, error) {
	if repositoryID == "" {
		return artifacts.ArtifactPage{}, artifacts.ErrInvalidArtifact
	}
	if _, err := artifacts.NewArtifactPageRequest(page.Limit, page.After); err != nil {
		return artifacts.ArtifactPage{}, err
	}
	query := s.database.WithContext(ctx).Where("repository_id = ?", repositoryID)
	if page.After != "" {
		query = query.Where("digest > ?", page.After)
	}
	var records []artifactRecord
	if err := query.Order("digest ASC").Limit(page.Limit + 1).Find(&records).Error; err != nil {
		return artifacts.ArtifactPage{}, classify("list artifacts", err)
	}
	hasMore := len(records) > page.Limit
	if hasMore {
		records = records[:page.Limit]
	}
	result := artifacts.ArtifactPage{Items: make([]artifacts.Artifact, 0, len(records))}
	for _, record := range records {
		artifact, err := artifactFromRecord(record)
		if err != nil {
			return artifacts.ArtifactPage{}, classify("decode listed artifact", err)
		}
		result.Items = append(result.Items, artifact)
	}
	if hasMore {
		result.NextAfter = records[len(records)-1].Digest
	}
	return result, nil
}

func (s *Store) ListTags(
	ctx context.Context,
	repositoryID string,
	page artifacts.PageRequest,
) (artifacts.TagPage, error) {
	if repositoryID == "" {
		return artifacts.TagPage{}, artifacts.ErrInvalidArtifact
	}
	if _, err := artifacts.NewTagPageRequest(page.Limit, page.After); err != nil {
		return artifacts.TagPage{}, err
	}
	query := tagReadQuery(s.database.WithContext(ctx)).Where("tags.repository_id = ?", repositoryID)
	if page.After != "" {
		query = query.Where("tags.name > ?", page.After)
	}
	var records []tagReadRecord
	if err := query.Order("tags.name ASC").Limit(page.Limit + 1).Find(&records).Error; err != nil {
		return artifacts.TagPage{}, classify("list artifact tags", err)
	}
	hasMore := len(records) > page.Limit
	if hasMore {
		records = records[:page.Limit]
	}
	result := artifacts.TagPage{Items: make([]artifacts.Tag, 0, len(records))}
	for _, record := range records {
		tag, err := tagFromRecord(record)
		if err != nil {
			return artifacts.TagPage{}, classify("decode listed artifact tag", err)
		}
		result.Items = append(result.Items, tag)
	}
	if hasMore {
		result.NextAfter = records[len(records)-1].Name
	}
	return result, nil
}

func (s *Store) DescriptorsByIndex(
	ctx context.Context,
	repositoryID string,
	digest artifacts.Digest,
) ([]artifacts.ManifestDescriptor, error) {
	if err := validateRepositoryDigest(repositoryID, digest); err != nil {
		return nil, err
	}
	var parent artifactRecord
	if err := s.database.WithContext(ctx).
		Where("repository_id = ? AND digest = ?", repositoryID, digest.String()).
		First(&parent).Error; err != nil {
		return nil, classify("find index artifact", err)
	}
	if parent.Kind != string(artifacts.KindIndex) {
		return nil, artifacts.ErrInvalidArtifact
	}
	items, err := descriptorsByArtifactID(s.database.WithContext(ctx), repositoryID, parent.ID)
	if err != nil {
		return nil, classify("list index descriptors", err)
	}
	return items, nil
}

func reconcileArtifact(
	transaction *gorm.DB,
	repositoryID string,
	observation artifacts.ArtifactObservation,
	observedAt time.Time,
) (artifactRecord, error) {
	id, err := newID()
	if err != nil {
		return artifactRecord{}, err
	}
	candidate := artifactRecord{
		ID: id, RepositoryID: repositoryID, Digest: observation.Digest.String(),
		Kind: string(observation.Kind), MediaType: cloneString(observation.MediaType),
		SizeBytes: cloneInt64(observation.SizeBytes), SourceCreatedAt: cloneTime(observation.SourceCreatedAt),
		DiscoveredAt: observedAt.UTC(), UpdatedAt: observedAt.UTC(),
	}
	if err := transaction.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_id"}, {Name: "digest"}},
		DoNothing: true,
	}).Create(&candidate).Error; err != nil {
		return artifactRecord{}, err
	}

	var current artifactRecord
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("repository_id = ? AND digest = ?", repositoryID, observation.Digest.String()).
		First(&current).Error; err != nil {
		return artifactRecord{}, err
	}
	if current.Kind != string(observation.Kind) {
		return artifactRecord{}, artifacts.ErrConflict
	}

	updates := make(map[string]any)
	metadataChanged := false
	if changed, err := mergeString(&current.MediaType, observation.MediaType); err != nil {
		return artifactRecord{}, err
	} else if changed {
		updates["media_type"] = current.MediaType
		metadataChanged = true
	}
	if changed, err := mergeInt64(&current.SizeBytes, observation.SizeBytes); err != nil {
		return artifactRecord{}, err
	} else if changed {
		updates["size_bytes"] = current.SizeBytes
		metadataChanged = true
	}
	if changed, err := mergeTime(&current.SourceCreatedAt, observation.SourceCreatedAt); err != nil {
		return artifactRecord{}, err
	} else if changed {
		updates["source_created_at"] = current.SourceCreatedAt
		metadataChanged = true
	}
	if observedAt.Before(current.DiscoveredAt) {
		current.DiscoveredAt = observedAt.UTC()
		updates["discovered_at"] = current.DiscoveredAt
	}
	if metadataChanged && observedAt.After(current.UpdatedAt) {
		current.UpdatedAt = observedAt.UTC()
		updates["updated_at"] = current.UpdatedAt
	}
	if len(updates) > 0 {
		if err := transaction.Model(&artifactRecord{}).
			Where("id = ?", current.ID).Updates(updates).Error; err != nil {
			return artifactRecord{}, err
		}
	}
	return current, nil
}

func reconcileDescriptors(
	transaction *gorm.DB,
	repositoryID string,
	parent *artifactRecord,
	set *artifacts.DescriptorSet,
	observedAt time.Time,
) error {
	if set == nil {
		return nil
	}
	if parent.Kind != string(artifacts.KindIndex) {
		return artifacts.ErrInvalidArtifact
	}

	desired := make([]manifestDescriptorRecord, 0, len(set.Items))
	for position, descriptor := range set.Items {
		child, err := reconcileArtifact(transaction, repositoryID, artifacts.ArtifactObservation{
			Digest: descriptor.Digest, Kind: artifacts.KindManifest,
			MediaType: descriptor.MediaType, SizeBytes: descriptor.SizeBytes,
		}, observedAt)
		if err != nil {
			return err
		}
		record := manifestDescriptorRecord{
			RepositoryID: repositoryID, IndexArtifactID: parent.ID,
			Position: position, ChildArtifactID: child.ID,
		}
		if descriptor.Platform != nil {
			record.OS = cloneRequiredString(descriptor.Platform.OS)
			record.Architecture = cloneRequiredString(descriptor.Platform.Architecture)
			if descriptor.Platform.Variant != "" {
				record.Variant = cloneRequiredString(descriptor.Platform.Variant)
			}
		}
		desired = append(desired, record)
	}

	if parent.DescriptorsComplete {
		var existing []manifestDescriptorRecord
		if err := transaction.Where(
			"repository_id = ? AND index_artifact_id = ?", repositoryID, parent.ID,
		).Order("position ASC").Find(&existing).Error; err != nil {
			return err
		}
		if !sameDescriptorRecords(existing, desired) {
			return artifacts.ErrConflict
		}
		return nil
	}

	if len(desired) > 0 {
		if err := transaction.Create(&desired).Error; err != nil {
			return err
		}
	}
	updates := map[string]any{"descriptors_complete": true}
	parent.DescriptorsComplete = true
	if observedAt.After(parent.UpdatedAt) {
		parent.UpdatedAt = observedAt.UTC()
		updates["updated_at"] = parent.UpdatedAt
	}
	return transaction.Model(&artifactRecord{}).Where("id = ?", parent.ID).Updates(updates).Error
}

func reconcileTag(
	transaction *gorm.DB,
	repositoryID string,
	name artifacts.TagName,
	artifact artifactRecord,
	observedAt time.Time,
) (artifacts.Tag, error) {
	candidate := tagRecord{
		RepositoryID: repositoryID, Name: name.String(), ArtifactID: artifact.ID,
		CreatedAt: observedAt.UTC(), UpdatedAt: observedAt.UTC(),
	}
	if err := transaction.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_id"}, {Name: "name"}},
		DoNothing: true,
	}).Create(&candidate).Error; err != nil {
		return artifacts.Tag{}, err
	}

	var current tagRecord
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("repository_id = ? AND name = ?", repositoryID, name.String()).
		First(&current).Error; err != nil {
		return artifacts.Tag{}, err
	}
	if current.ArtifactID != artifact.ID {
		if !observedAt.After(current.UpdatedAt) {
			var currentArtifact artifactRecord
			if err := transaction.Where(
				"repository_id = ? AND id = ?", repositoryID, current.ArtifactID,
			).First(&currentArtifact).Error; err != nil {
				return artifacts.Tag{}, err
			}
			return artifacts.Tag{
				RepositoryID: repositoryID, Name: name, ArtifactID: current.ArtifactID,
				Digest:    artifacts.Digest(currentArtifact.Digest),
				CreatedAt: current.CreatedAt.UTC(), UpdatedAt: current.UpdatedAt.UTC(),
			}, nil
		}
		updates := map[string]any{"artifact_id": artifact.ID}
		current.ArtifactID = artifact.ID
		if observedAt.After(current.UpdatedAt) {
			current.UpdatedAt = observedAt.UTC()
			updates["updated_at"] = current.UpdatedAt
		}
		if err := transaction.Model(&tagRecord{}).
			Where("repository_id = ? AND name = ?", repositoryID, name.String()).
			Updates(updates).Error; err != nil {
			return artifacts.Tag{}, err
		}
	}
	return artifacts.Tag{
		RepositoryID: repositoryID, Name: name, ArtifactID: artifact.ID,
		Digest: artifacts.Digest(artifact.Digest), CreatedAt: current.CreatedAt.UTC(), UpdatedAt: current.UpdatedAt.UTC(),
	}, nil
}

func descriptorsByArtifactID(
	database *gorm.DB,
	repositoryID string,
	artifactID string,
) ([]artifacts.ManifestDescriptor, error) {
	var records []descriptorReadRecord
	if err := database.Table("manifest_descriptors").Select(
		"manifest_descriptors.position, manifest_descriptors.child_artifact_id, artifacts.digest, "+
			"artifacts.media_type, artifacts.size_bytes, "+
			"manifest_descriptors.os, manifest_descriptors.architecture, manifest_descriptors.variant",
	).Joins(
		"JOIN artifacts ON artifacts.repository_id = manifest_descriptors.repository_id AND artifacts.id = manifest_descriptors.child_artifact_id",
	).Where(
		"manifest_descriptors.repository_id = ? AND manifest_descriptors.index_artifact_id = ?",
		repositoryID,
		artifactID,
	).Order("manifest_descriptors.position ASC").Scan(&records).Error; err != nil {
		return nil, err
	}
	items := make([]artifacts.ManifestDescriptor, 0, len(records))
	for _, record := range records {
		digest, err := artifacts.ParseDigest(record.Digest)
		if err != nil {
			return nil, err
		}
		item := artifacts.ManifestDescriptor{
			Position: record.Position, ChildArtifactID: record.ChildArtifactID, Digest: digest,
			MediaType: cloneString(record.MediaType), SizeBytes: cloneInt64(record.SizeBytes),
		}
		if record.OS != nil || record.Architecture != nil || record.Variant != nil {
			if record.OS == nil || record.Architecture == nil {
				return nil, artifacts.ErrInvalidArtifact
			}
			item.Platform = &artifacts.Platform{OS: *record.OS, Architecture: *record.Architecture}
			if record.Variant != nil {
				item.Platform.Variant = *record.Variant
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func artifactFromRecord(record artifactRecord) (artifacts.Artifact, error) {
	digest, err := artifacts.ParseDigest(record.Digest)
	if err != nil {
		return artifacts.Artifact{}, err
	}
	kind, err := artifacts.ParseKind(record.Kind)
	if err != nil {
		return artifacts.Artifact{}, err
	}
	return artifacts.Artifact{
		ID: record.ID, RepositoryID: record.RepositoryID, Digest: digest, Kind: kind,
		MediaType: cloneString(record.MediaType), SizeBytes: cloneInt64(record.SizeBytes),
		SourceCreatedAt: cloneTime(record.SourceCreatedAt), DescriptorsComplete: record.DescriptorsComplete,
		DiscoveredAt: record.DiscoveredAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}, nil
}

func tagReadQuery(database *gorm.DB) *gorm.DB {
	return database.Table("tags").Select(
		"tags.repository_id, tags.name, tags.artifact_id, artifacts.digest, tags.created_at, tags.updated_at",
	).Joins(
		"JOIN artifacts ON artifacts.repository_id = tags.repository_id AND artifacts.id = tags.artifact_id",
	)
}

func tagFromRecord(record tagReadRecord) (artifacts.Tag, error) {
	name, err := artifacts.ParseTagName(record.Name)
	if err != nil {
		return artifacts.Tag{}, err
	}
	digest, err := artifacts.ParseDigest(record.Digest)
	if err != nil {
		return artifacts.Tag{}, err
	}
	return artifacts.Tag{
		RepositoryID: record.RepositoryID, Name: name, ArtifactID: record.ArtifactID, Digest: digest,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}, nil
}

func sameDescriptorRecords(first, second []manifestDescriptorRecord) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index].Position != second[index].Position ||
			first[index].ChildArtifactID != second[index].ChildArtifactID ||
			!sameOptionalString(first[index].OS, second[index].OS) ||
			!sameOptionalString(first[index].Architecture, second[index].Architecture) ||
			!sameOptionalString(first[index].Variant, second[index].Variant) {
			return false
		}
	}
	return true
}

func mergeString(current **string, incoming *string) (bool, error) {
	if incoming == nil {
		return false, nil
	}
	if *current != nil {
		if **current != *incoming {
			return false, artifacts.ErrConflict
		}
		return false, nil
	}
	*current = cloneString(incoming)
	return true, nil
}

func mergeInt64(current **int64, incoming *int64) (bool, error) {
	if incoming == nil {
		return false, nil
	}
	if *current != nil {
		if **current != *incoming {
			return false, artifacts.ErrConflict
		}
		return false, nil
	}
	*current = cloneInt64(incoming)
	return true, nil
}

func mergeTime(current **time.Time, incoming *time.Time) (bool, error) {
	if incoming == nil {
		return false, nil
	}
	normalized := incoming.UTC().Round(time.Microsecond)
	if *current != nil {
		if !(**current).Equal(normalized) {
			return false, artifacts.ErrConflict
		}
		return false, nil
	}
	*current = &normalized
	return true, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneRequiredString(value string) *string {
	copy := value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC().Round(time.Microsecond)
	return &copy
}

func sameOptionalString(first, second *string) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}

func validateRepositoryDigest(repositoryID string, digest artifacts.Digest) error {
	if repositoryID == "" {
		return artifacts.ErrInvalidArtifact
	}
	_, err := artifacts.ParseDigest(digest.String())
	return err
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate artifact ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}

func classify(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, artifacts.ErrInvalidDigest) || errors.Is(err, artifacts.ErrInvalidTag) ||
		errors.Is(err, artifacts.ErrInvalidArtifact) || errors.Is(err, artifacts.ErrConflict) ||
		errors.Is(err, artifacts.ErrNotFound) || errors.Is(err, artifacts.ErrUnavailable) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, artifacts.ErrNotFound)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.ConstraintName {
		case "ck_artifacts_digest":
			return fmt.Errorf("%s: %w", operation, artifacts.ErrInvalidDigest)
		case "ck_tags_name":
			return fmt.Errorf("%s: %w", operation, artifacts.ErrInvalidTag)
		}
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, artifacts.ErrConflict)
		case "23502", "23503", "23514", "22P02":
			return fmt.Errorf("%s: %w", operation, artifacts.ErrInvalidArtifact)
		}
	}
	return fmt.Errorf("%s: %v: %w", operation, err, artifacts.ErrUnavailable)
}

var _ artifacts.Store = (*Store)(nil)
var _ artifacts.Reader = (*Store)(nil)
