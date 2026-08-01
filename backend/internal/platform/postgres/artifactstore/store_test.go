package artifactstore

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/migrations"
)

func TestStoreReconciliationAndRepositoryScopedReads(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 12,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 22, 0, 0, 123000000, time.UTC)
	repositories := createArtifactRepositories(t, ctx, pool, now)
	store := New(pool.ORM())

	manifestDigest := digest("1")
	mediaType := "application/vnd.oci.image.manifest.v1+json"
	size := int64(731)
	latest := "latest"
	initial := mustReconciliation(t, artifacts.Observation{
		RepositoryID: repositories.first.ID,
		Digest:       manifestDigest,
		Kind:         string(artifacts.KindManifest),
		Tag:          &latest,
		ObservedAt:   now,
	})

	created, err := store.Reconcile(ctx, initial)
	if err != nil {
		t.Fatalf("Reconcile(initial manifest) error = %v", err)
	}
	if created.Artifact.Digest.String() != manifestDigest || created.Artifact.MediaType != nil ||
		created.Tag == nil || created.Tag.Name.String() != latest {
		t.Fatalf("initial snapshot = %#v", created)
	}

	replayed, err := store.Reconcile(ctx, withObservedAt(initial, now.Add(time.Minute)))
	if err != nil {
		t.Fatalf("Reconcile(replay) error = %v", err)
	}
	if replayed.Artifact.ID != created.Artifact.ID ||
		!replayed.Artifact.UpdatedAt.Equal(created.Artifact.UpdatedAt) ||
		replayed.Tag == nil || !replayed.Tag.UpdatedAt.Equal(created.Tag.UpdatedAt) {
		t.Fatalf("replay changed stable state: created=%#v replayed=%#v", created, replayed)
	}

	enrichedInput := initial
	enrichedInput.Artifact.MediaType = &mediaType
	enrichedInput.Artifact.SizeBytes = &size
	enrichedInput.ObservedAt = now.Add(2 * time.Minute)
	enriched, err := store.Reconcile(ctx, enrichedInput)
	if err != nil {
		t.Fatalf("Reconcile(enrichment) error = %v", err)
	}
	if enriched.Artifact.MediaType == nil || *enriched.Artifact.MediaType != mediaType ||
		enriched.Artifact.SizeBytes == nil || *enriched.Artifact.SizeBytes != size ||
		!enriched.Artifact.UpdatedAt.Equal(enrichedInput.ObservedAt) {
		t.Fatalf("enriched artifact = %#v", enriched.Artifact)
	}

	secondDigest := digest("2")
	second := mustReconciliation(t, artifacts.Observation{
		RepositoryID: repositories.first.ID,
		Digest:       secondDigest,
		Kind:         string(artifacts.KindManifest),
		Tag:          &latest,
		ObservedAt:   now.Add(3 * time.Minute),
	})
	moved, err := store.Reconcile(ctx, second)
	if err != nil {
		t.Fatalf("Reconcile(tag move) error = %v", err)
	}
	if moved.Tag == nil || moved.Tag.Digest.String() != secondDigest {
		t.Fatalf("moved tag = %#v", moved.Tag)
	}
	stale := initial
	stale.ObservedAt = now.Add(150 * time.Second)
	staleResult, err := store.Reconcile(ctx, stale)
	if err != nil {
		t.Fatalf("Reconcile(stale tag event) error = %v", err)
	}
	if staleResult.Tag == nil || staleResult.Tag.Digest.String() != secondDigest ||
		!staleResult.Tag.UpdatedAt.Equal(moved.Tag.UpdatedAt) {
		t.Fatalf("stale tag event regressed current mapping: %#v", staleResult.Tag)
	}
	if _, err := store.ArtifactByDigest(ctx, repositories.first.ID, artifacts.Digest(manifestDigest)); err != nil {
		t.Fatalf("old untagged ArtifactByDigest() error = %v", err)
	}

	conflictingMediaType := "application/vnd.docker.distribution.manifest.v2+json"
	conflict := enrichedInput
	conflict.Artifact.MediaType = &conflictingMediaType
	conflict.Tag = ptrTag(t, latest)
	conflict.ObservedAt = now.Add(4 * time.Minute)
	if _, err := store.Reconcile(ctx, conflict); !errors.Is(err, artifacts.ErrConflict) {
		t.Fatalf("Reconcile(conflicting metadata) error = %v, want ErrConflict", err)
	}
	stableTag, err := store.TagByName(ctx, repositories.first.ID, artifacts.TagName(latest))
	if err != nil || stableTag.Digest.String() != secondDigest {
		t.Fatalf("tag after conflicting transaction = %#v, %v; want second digest", stableTag, err)
	}

	if err := store.RemoveTag(ctx, repositories.first.ID, artifacts.TagName(latest)); err != nil {
		t.Fatalf("RemoveTag() error = %v", err)
	}
	if err := store.RemoveTag(ctx, repositories.first.ID, artifacts.TagName(latest)); err != nil {
		t.Fatalf("RemoveTag(idempotent) error = %v", err)
	}
	if _, err := store.TagByName(ctx, repositories.first.ID, artifacts.TagName(latest)); !errors.Is(err, artifacts.ErrNotFound) {
		t.Fatalf("removed TagByName() error = %v, want ErrNotFound", err)
	}
	if _, err := store.ArtifactByDigest(ctx, repositories.first.ID, artifacts.Digest(secondDigest)); err != nil {
		t.Fatalf("artifact after RemoveTag() error = %v", err)
	}

	indexDigest := digest("3")
	childOne := digest("4")
	childTwo := digest("5")
	indexTag := "multi"
	index := mustReconciliation(t, artifacts.Observation{
		RepositoryID: repositories.first.ID,
		Digest:       indexDigest,
		Kind:         string(artifacts.KindIndex),
		Tag:          &indexTag,
		Descriptors: &artifacts.DescriptorSetObservation{Items: []artifacts.DescriptorObservation{
			{Digest: childOne, MediaType: &mediaType, SizeBytes: &size, Platform: &artifacts.Platform{OS: "linux", Architecture: "amd64"}},
			{Digest: childTwo, Platform: &artifacts.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"}},
		}},
		ObservedAt: now.Add(5 * time.Minute),
	})
	indexSnapshot, err := store.Reconcile(ctx, index)
	if err != nil {
		t.Fatalf("Reconcile(index) error = %v", err)
	}
	if !indexSnapshot.Artifact.DescriptorsComplete || len(indexSnapshot.Descriptors) != 2 ||
		indexSnapshot.Descriptors[0].Position != 0 || indexSnapshot.Descriptors[0].Digest.String() != childOne ||
		indexSnapshot.Descriptors[1].Platform == nil || indexSnapshot.Descriptors[1].Platform.Variant != "v8" {
		t.Fatalf("index snapshot = %#v", indexSnapshot)
	}

	indexReplay, err := store.Reconcile(ctx, withObservedAt(index, now.Add(6*time.Minute)))
	if err != nil {
		t.Fatalf("Reconcile(index replay) error = %v", err)
	}
	if !indexReplay.Artifact.UpdatedAt.Equal(indexSnapshot.Artifact.UpdatedAt) ||
		indexReplay.Tag == nil || !indexReplay.Tag.UpdatedAt.Equal(indexSnapshot.Tag.UpdatedAt) {
		t.Fatalf("index replay changed timestamps: first=%#v replay=%#v", indexSnapshot, indexReplay)
	}

	differentSet := index
	differentSet.Descriptors = &artifacts.DescriptorSet{Items: []artifacts.Descriptor{{Digest: artifacts.Digest(childTwo)}}}
	differentSet.ObservedAt = now.Add(7 * time.Minute)
	if _, err := store.Reconcile(ctx, differentSet); !errors.Is(err, artifacts.ErrConflict) {
		t.Fatalf("Reconcile(different descriptor set) error = %v, want ErrConflict", err)
	}
	storedDescriptors, err := store.DescriptorsByIndex(ctx, repositories.first.ID, artifacts.Digest(indexDigest))
	if err != nil || len(storedDescriptors) != 2 || storedDescriptors[0].Digest.String() != childOne {
		t.Fatalf("DescriptorsByIndex() = %#v, %v", storedDescriptors, err)
	}

	emptyIndexDigest := digest("6")
	emptyIndex := mustReconciliation(t, artifacts.Observation{
		RepositoryID: repositories.first.ID,
		Digest:       emptyIndexDigest,
		Kind:         string(artifacts.KindIndex),
		ObservedAt:   now.Add(8 * time.Minute),
	})
	unknownDescriptors, err := store.Reconcile(ctx, emptyIndex)
	if err != nil || unknownDescriptors.Artifact.DescriptorsComplete {
		t.Fatalf("Reconcile(index with unknown descriptors) = %#v, %v", unknownDescriptors, err)
	}
	emptyIndex.Descriptors = &artifacts.DescriptorSet{Items: []artifacts.Descriptor{}}
	emptyIndex.ObservedAt = now.Add(9 * time.Minute)
	confirmedEmpty, err := store.Reconcile(ctx, emptyIndex)
	if err != nil || !confirmedEmpty.Artifact.DescriptorsComplete || len(confirmedEmpty.Descriptors) != 0 {
		t.Fatalf("Reconcile(confirmed empty index) = %#v, %v", confirmedEmpty, err)
	}

	sameDigestOtherRepository := mustReconciliation(t, artifacts.Observation{
		RepositoryID: repositories.second.ID,
		Digest:       manifestDigest,
		Kind:         string(artifacts.KindManifest),
		ObservedAt:   now.Add(10 * time.Minute),
	})
	otherSnapshot, err := store.Reconcile(ctx, sameDigestOtherRepository)
	if err != nil {
		t.Fatalf("Reconcile(same digest in another repository) error = %v", err)
	}
	if otherSnapshot.Artifact.ID == created.Artifact.ID {
		t.Fatal("same digest across repositories reused repository-scoped artifact ID")
	}
	if _, err := store.ArtifactByDigest(ctx, repositories.second.ID, artifacts.Digest(secondDigest)); !errors.Is(err, artifacts.ErrNotFound) {
		t.Fatalf("cross-repository ArtifactByDigest() error = %v, want ErrNotFound", err)
	}
	assertCrossRepositoryForeignKeys(
		t, ctx, store, repositories.first.ID, indexSnapshot.Artifact.ID, otherSnapshot.Artifact.ID, now,
	)

	artifactPageRequest, err := artifacts.NewArtifactPageRequest(2, "")
	if err != nil {
		t.Fatalf("NewArtifactPageRequest() error = %v", err)
	}
	firstPage, err := store.ListArtifacts(ctx, repositories.first.ID, artifactPageRequest)
	if err != nil || len(firstPage.Items) != 2 || firstPage.NextAfter == "" {
		t.Fatalf("first ListArtifacts() = %#v, %v", firstPage, err)
	}
	secondPageRequest, err := artifacts.NewArtifactPageRequest(10, firstPage.NextAfter)
	if err != nil {
		t.Fatalf("NewArtifactPageRequest(after) error = %v", err)
	}
	secondPage, err := store.ListArtifacts(ctx, repositories.first.ID, secondPageRequest)
	if err != nil || len(secondPage.Items) < 1 || secondPage.NextAfter != "" {
		t.Fatalf("second ListArtifacts() = %#v, %v", secondPage, err)
	}

	for number, tag := range []string{"alpha", "beta", "gamma"} {
		input := second
		input.Tag = ptrTag(t, tag)
		input.ObservedAt = now.Add(time.Duration(10+number) * time.Minute)
		if _, err := store.Reconcile(ctx, input); err != nil {
			t.Fatalf("Reconcile(tag %q) error = %v", tag, err)
		}
	}
	tagPageRequest, err := artifacts.NewTagPageRequest(2, "")
	if err != nil {
		t.Fatalf("NewTagPageRequest() error = %v", err)
	}
	tagPage, err := store.ListTags(ctx, repositories.first.ID, tagPageRequest)
	if err != nil || len(tagPage.Items) != 2 || tagPage.NextAfter == "" {
		t.Fatalf("ListTags() = %#v, %v", tagPage, err)
	}

	assertConcurrentReplay(t, ctx, store, repositories.first.ID, now.Add(20*time.Minute))
}

func assertConcurrentReplay(t *testing.T, ctx context.Context, store *Store, repositoryID string, observedAt time.Time) {
	t.Helper()
	input := mustReconciliation(t, artifacts.Observation{
		RepositoryID: repositoryID,
		Digest:       digest("f"),
		Kind:         string(artifacts.KindManifest),
		ObservedAt:   observedAt,
	})
	start := make(chan struct{})
	errorsChannel := make(chan error, 6)
	var waitGroup sync.WaitGroup
	for range 6 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			_, err := store.Reconcile(ctx, input)
			errorsChannel <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent Reconcile() error = %v", err)
		}
	}

	var count int64
	if err := store.database.WithContext(ctx).Model(&artifactRecord{}).
		Where("repository_id = ? AND digest = ?", repositoryID, input.Artifact.Digest.String()).
		Count(&count).Error; err != nil {
		t.Fatalf("count concurrent artifact rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("concurrent artifact row count = %d, want 1", count)
	}
}

func assertCrossRepositoryForeignKeys(
	t *testing.T,
	ctx context.Context,
	store *Store,
	firstRepositoryID string,
	firstIndexID string,
	secondArtifactID string,
	at time.Time,
) {
	t.Helper()
	invalidTag := tagRecord{
		RepositoryID: firstRepositoryID, Name: "cross-repository", ArtifactID: secondArtifactID,
		CreatedAt: at.UTC(), UpdatedAt: at.UTC(),
	}
	if err := store.database.WithContext(ctx).Create(&invalidTag).Error; err == nil {
		t.Fatal("cross-repository tag insert error = nil, want composite foreign-key failure")
	}
	invalidDescriptor := manifestDescriptorRecord{
		RepositoryID: firstRepositoryID, IndexArtifactID: firstIndexID,
		Position: 99, ChildArtifactID: secondArtifactID,
	}
	if err := store.database.WithContext(ctx).Create(&invalidDescriptor).Error; err == nil {
		t.Fatal("cross-repository descriptor insert error = nil, want composite foreign-key failure")
	}
}

type artifactRepositoryFixture struct {
	first  repositories.Repository
	second repositories.Repository
}

func createArtifactRepositories(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	now time.Time,
) artifactRepositoryFixture {
	t.Helper()
	ownerID := auth.ID("41414141-4141-4414-8414-414141414141")
	identity := auth.Identity{
		User: auth.User{
			ID: ownerID, Username: "artifact-owner", PersonalNamespace: "artifact-owner",
			CreatedAt: now, UpdatedAt: now,
		},
		Credential: auth.LocalCredential{
			UserID: ownerID, PasswordHash: "test-hash", PasswordChangedAt: now,
			CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: ownerID, Name: "artifact-owner"},
	}
	if err := authstore.New(pool.ORM()).CreateIdentity(ctx, identity); err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	repositoryStore := repositorystore.New(pool.ORM())
	create := func(name string) repositories.Repository {
		repository, err := repositories.New(repositories.NewRepository{
			NamespaceID: string(ownerID), RequestedName: name,
			Visibility: repositories.VisibilityPrivate, CreatedByUserID: string(ownerID),
		}, now)
		if err != nil {
			t.Fatalf("repositories.New(%q) error = %v", name, err)
		}
		if err := repositoryStore.Create(ctx, repository); err != nil {
			t.Fatalf("repositoryStore.Create(%q) error = %v", name, err)
		}
		return repository
	}
	return artifactRepositoryFixture{first: create("artifacts-one"), second: create("artifacts-two")}
}

func mustReconciliation(t *testing.T, input artifacts.Observation) artifacts.Reconciliation {
	t.Helper()
	reconciliation, err := artifacts.NormalizeObservation(input)
	if err != nil {
		t.Fatalf("NormalizeObservation() error = %v", err)
	}
	return reconciliation
}

func withObservedAt(input artifacts.Reconciliation, observedAt time.Time) artifacts.Reconciliation {
	input.ObservedAt = observedAt.UTC()
	return input
}

func ptrTag(t *testing.T, value string) *artifacts.TagName {
	t.Helper()
	tag, err := artifacts.ParseTagName(value)
	if err != nil {
		t.Fatalf("ParseTagName(%q) error = %v", value, err)
	}
	return &tag
}

func digest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
