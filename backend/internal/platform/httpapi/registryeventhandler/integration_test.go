package registryeventhandler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/observability"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/artifactstore"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/migrations"
)

func TestRegistryEventHandlerReconcilesRealPostgresState(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 8,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 23, 30, 0, 123456789, time.UTC)
	repository := createRegistryEventRepository(t, ctx, pool, now)
	policy := authorization.NewPolicy()
	repositoryService, err := repositories.NewService(repositorystore.New(pool.ORM()), policy, func() time.Time { return now })
	if err != nil {
		t.Fatalf("repositories.NewService() error = %v", err)
	}
	artifactStore := artifactstore.New(pool.ORM())
	artifactService, err := artifacts.NewService(artifactStore)
	if err != nil {
		t.Fatalf("artifacts.NewService() error = %v", err)
	}
	notificationService, err := registry.NewNotificationService(repositoryService, artifactService)
	if err != nil {
		t.Fatalf("registry.NewNotificationService() error = %v", err)
	}
	handler, err := New(
		notificationService,
		[]byte(registryEventTestToken),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		observability.NewRegistryMetrics(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	firstDigest := registryEventDigest("a")
	secondDigest := registryEventDigest("b")
	indexDigest := registryEventDigest("c")
	childDigest := registryEventDigest("d")
	manifestSize := int64(731)
	first := registry.NotificationEvent{
		ID: "first", Timestamp: now, Action: registry.NotificationActionPush,
		Target: registry.NotificationTarget{
			MediaType: registry.OCIImageManifestMediaType, Digest: firstDigest,
			SizeBytes: &manifestSize, Repository: "registry-event-owner/event-image", Tag: "latest",
		},
	}
	assertNotificationStatus(t, handler, registry.NotificationEnvelope{Events: []registry.NotificationEvent{first}}, http.StatusAccepted)
	created, err := artifactStore.TagByName(ctx, repository.ID, artifacts.TagName("latest"))
	if err != nil || created.Digest.String() != firstDigest {
		t.Fatalf("initial TagByName() = %#v, %v", created, err)
	}

	assertNotificationStatus(t, handler, registry.NotificationEnvelope{Events: []registry.NotificationEvent{first}}, http.StatusAccepted)
	replayed, err := artifactStore.TagByName(ctx, repository.ID, artifacts.TagName("latest"))
	if err != nil || !replayed.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("replayed TagByName() = %#v, %v; initial %#v", replayed, err, created)
	}

	second := first
	second.ID = "second"
	second.Timestamp = now.Add(2 * time.Second)
	second.Target.Digest = secondDigest
	assertNotificationStatus(t, handler, registry.NotificationEnvelope{Events: []registry.NotificationEvent{second}}, http.StatusAccepted)
	stale := first
	stale.ID = "stale"
	stale.Timestamp = now.Add(time.Second)
	assertNotificationStatus(t, handler, registry.NotificationEnvelope{Events: []registry.NotificationEvent{stale}}, http.StatusAccepted)
	winningTag, err := artifactStore.TagByName(ctx, repository.ID, artifacts.TagName("latest"))
	if err != nil || winningTag.Digest.String() != secondDigest || !winningTag.UpdatedAt.Equal(second.Timestamp.Round(time.Microsecond)) {
		t.Fatalf("winning TagByName() = %#v, %v", winningTag, err)
	}
	if _, err := artifactStore.ArtifactByDigest(ctx, repository.ID, artifacts.Digest(firstDigest)); err != nil {
		t.Fatalf("old untagged ArtifactByDigest() error = %v", err)
	}

	index := registry.NotificationEvent{
		ID: "index", Timestamp: now.Add(3 * time.Second), Action: registry.NotificationActionPush,
		Target: registry.NotificationTarget{
			MediaType: registry.OCIImageIndexMediaType, Digest: indexDigest,
			SizeBytes: &manifestSize, Repository: "registry-event-owner/event-image", Tag: "multi",
			References: []registry.NotificationDescriptor{{
				MediaType: registry.OCIImageManifestMediaType, Digest: childDigest,
				SizeBytes: &manifestSize,
				Platform:  &registry.NotificationPlatform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			}},
		},
	}
	assertNotificationStatus(t, handler, registry.NotificationEnvelope{Events: []registry.NotificationEvent{index}}, http.StatusAccepted)
	descriptors, err := artifactStore.DescriptorsByIndex(ctx, repository.ID, artifacts.Digest(indexDigest))
	if err != nil || len(descriptors) != 1 || descriptors[0].Digest.String() != childDigest ||
		descriptors[0].Platform == nil || descriptors[0].Platform.Variant != "v8" {
		t.Fatalf("DescriptorsByIndex() = %#v, %v", descriptors, err)
	}

	missing := first
	missing.ID = "missing"
	missing.Target.Repository = "registry-event-owner/missing"
	assertNotificationStatus(t, handler, registry.NotificationEnvelope{Events: []registry.NotificationEvent{missing}}, http.StatusServiceUnavailable)
}

func assertNotificationStatus(
	t *testing.T,
	handler *Handler,
	envelope registry.NotificationEnvelope,
	want int,
) {
	t.Helper()
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, RegistryEventPath, strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+registryEventTestToken)
	request.Header.Set("Content-Type", registry.NotificationEventsMediaType)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("notification status = %d, want %d", recorder.Code, want)
	}
}

func createRegistryEventRepository(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	now time.Time,
) repositories.Repository {
	t.Helper()
	ownerID := auth.ID("42424242-4242-4424-8424-424242424242")
	identity := auth.Identity{
		User: auth.User{
			ID: ownerID, Username: "registry-event-owner", PersonalNamespace: "registry-event-owner",
			CreatedAt: now, UpdatedAt: now,
		},
		Credential: auth.LocalCredential{
			UserID: ownerID, PasswordHash: "test-hash", PasswordChangedAt: now,
			CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: ownerID, Name: "registry-event-owner"},
	}
	if err := authstore.New(pool.ORM()).CreateIdentity(ctx, identity); err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	repository, err := repositories.New(repositories.NewRepository{
		NamespaceID: string(ownerID), RequestedName: "event-image",
		Visibility: repositories.VisibilityPrivate, CreatedByUserID: string(ownerID),
	}, now)
	if err != nil {
		t.Fatalf("repositories.New() error = %v", err)
	}
	if err := repositorystore.New(pool.ORM()).Create(ctx, repository); err != nil {
		t.Fatalf("repositoryStore.Create() error = %v", err)
	}
	return repository
}

func registryEventDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
