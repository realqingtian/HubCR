package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/repositories"
)

const (
	notificationManifestDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	notificationIndexDigest    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	notificationChildDigest    = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

func TestNotificationServiceReconcilesSupportedManifestPushes(t *testing.T) {
	now := time.Date(2026, 8, 1, 23, 0, 0, 123456789, time.UTC)
	manifestSize := int64(731)
	childSize := int64(512)
	resolver := &serviceRepositoryResolver{contexts: map[string]repositories.AuthorizationContext{
		"team/image": {
			Repository: repositories.Repository{ID: "repository-id", NamespaceID: "namespace-id", Name: "image"},
			Namespace:  repositories.NamespaceAccess{NamespaceID: "namespace-id", NamespaceName: "team"},
		},
	}}
	reconciler := &notificationArtifactReconciler{}
	service, err := NewNotificationService(resolver, reconciler)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	result, err := service.Process(context.Background(), NotificationEnvelope{Events: []NotificationEvent{
		{
			ID: "manifest-push", Timestamp: now, Action: NotificationActionPush,
			Target: NotificationTarget{
				MediaType: OCIImageManifestMediaType, Digest: notificationManifestDigest,
				SizeBytes: &manifestSize, Repository: "team/image", Tag: "latest",
			},
		},
		{
			ID: "index-push", Timestamp: now.Add(time.Second), Action: NotificationActionPush,
			Target: NotificationTarget{
				MediaType: OCIImageIndexMediaType, Digest: notificationIndexDigest,
				SizeBytes: &manifestSize, Repository: "team/image", Tag: "multi",
				References: []NotificationDescriptor{{
					MediaType: OCIImageManifestMediaType, Digest: notificationChildDigest,
					SizeBytes: &childSize,
					Platform:  &NotificationPlatform{OS: "linux", Architecture: "arm64", Variant: "v8"},
				}},
			},
		},
		{
			ID: "blob-push", Timestamp: now, Action: NotificationActionPush,
			Target: NotificationTarget{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    notificationChildDigest, Repository: "team/image",
			},
		},
		{ID: "pull", Timestamp: now, Action: "pull", Target: NotificationTarget{Repository: "team/image"}},
		{ID: "delete", Timestamp: now, Action: "delete", Target: NotificationTarget{Repository: "team/image", Tag: "latest"}},
	}})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result.Processed != 2 || result.Ignored != 3 {
		t.Fatalf("Process() result = %#v, want 2 processed and 3 ignored", result)
	}
	if resolver.actor != "" {
		t.Fatalf("notification repository actor = %q, want internal empty actor", resolver.actor)
	}
	if len(reconciler.observations) != 2 {
		t.Fatalf("reconciliation count = %d, want 2", len(reconciler.observations))
	}
	manifest := reconciler.observations[0]
	if manifest.RepositoryID != "repository-id" || manifest.Kind != string(artifacts.KindManifest) ||
		manifest.Tag == nil || *manifest.Tag != "latest" || manifest.Descriptors != nil ||
		manifest.MediaType == nil || *manifest.MediaType != OCIImageManifestMediaType {
		t.Fatalf("manifest observation = %#v", manifest)
	}
	index := reconciler.observations[1]
	if index.Kind != string(artifacts.KindIndex) || index.Descriptors == nil ||
		len(index.Descriptors.Items) != 1 ||
		index.Descriptors.Items[0].Digest != notificationChildDigest ||
		index.Descriptors.Items[0].Platform == nil ||
		index.Descriptors.Items[0].Platform.Variant != "v8" {
		t.Fatalf("index observation = %#v", index)
	}
}

func TestNotificationServiceLeavesEmptyIndexReferencesUnknown(t *testing.T) {
	reconciler := &notificationArtifactReconciler{}
	service, err := NewNotificationService(&serviceRepositoryResolver{
		contexts: map[string]repositories.AuthorizationContext{
			"team/image": {Repository: repositories.Repository{ID: "repository-id"}},
		},
	}, reconciler)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}
	_, err = service.Process(context.Background(), NotificationEnvelope{Events: []NotificationEvent{{
		ID: "empty-index", Timestamp: time.Now(), Action: NotificationActionPush,
		Target: NotificationTarget{
			MediaType: OCIImageIndexMediaType, Digest: notificationIndexDigest,
			Repository: "team/image",
		},
	}}})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(reconciler.observations) != 1 || reconciler.observations[0].Descriptors != nil {
		t.Fatalf("empty index observation = %#v, want unknown descriptors", reconciler.observations)
	}
}

func TestNotificationServiceRejectsInvalidRelevantEvent(t *testing.T) {
	service, err := NewNotificationService(&serviceRepositoryResolver{}, &notificationArtifactReconciler{})
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}
	tests := []NotificationEvent{
		{Action: NotificationActionPush, Timestamp: time.Now(), Target: NotificationTarget{MediaType: OCIImageManifestMediaType, Digest: notificationManifestDigest, Repository: "nested/team/image"}},
		{Action: NotificationActionPush, Target: NotificationTarget{MediaType: OCIImageManifestMediaType, Digest: notificationManifestDigest, Repository: "team/image"}},
		{Action: NotificationActionPush, Timestamp: time.Now(), Target: NotificationTarget{MediaType: OCIImageManifestMediaType, Digest: "invalid", Repository: "team/image"}},
	}
	for _, event := range tests {
		if _, err := service.Process(context.Background(), NotificationEnvelope{Events: []NotificationEvent{event}}); !errors.Is(err, ErrInvalidNotification) {
			t.Fatalf("Process(%#v) error = %v, want ErrInvalidNotification", event, err)
		}
	}
}

func TestNotificationServiceClassifiesDependencies(t *testing.T) {
	validEvent := NotificationEvent{
		ID: "push", Timestamp: time.Now(), Action: NotificationActionPush,
		Target: NotificationTarget{
			MediaType: OCIImageManifestMediaType, Digest: notificationManifestDigest,
			Repository: "team/image",
		},
	}
	tests := []struct {
		name       string
		resolver   *serviceRepositoryResolver
		reconciler *notificationArtifactReconciler
		want       error
	}{
		{
			name: "missing repository is retryable",
			resolver: &serviceRepositoryResolver{
				errors: map[string]error{"team/image": repositories.ErrNotFound},
			},
			reconciler: &notificationArtifactReconciler{}, want: ErrNotificationUnavailable,
		},
		{
			name: "artifact conflict",
			resolver: &serviceRepositoryResolver{contexts: map[string]repositories.AuthorizationContext{
				"team/image": {Repository: repositories.Repository{ID: "repository-id"}},
			}},
			reconciler: &notificationArtifactReconciler{err: artifacts.ErrConflict},
			want:       ErrNotificationConflict,
		},
		{
			name: "artifact database unavailable",
			resolver: &serviceRepositoryResolver{contexts: map[string]repositories.AuthorizationContext{
				"team/image": {Repository: repositories.Repository{ID: "repository-id"}},
			}},
			reconciler: &notificationArtifactReconciler{err: artifacts.ErrUnavailable},
			want:       ErrNotificationUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewNotificationService(test.resolver, test.reconciler)
			if err != nil {
				t.Fatalf("NewNotificationService() error = %v", err)
			}
			if _, err := service.Process(context.Background(), NotificationEnvelope{Events: []NotificationEvent{validEvent}}); !errors.Is(err, test.want) {
				t.Fatalf("Process() error = %v, want %v", err, test.want)
			}
		})
	}
}

type notificationArtifactReconciler struct {
	observations []artifacts.Observation
	err          error
}

func (r *notificationArtifactReconciler) ReconcileArtifact(
	_ context.Context,
	observation artifacts.Observation,
) (artifacts.Snapshot, error) {
	r.observations = append(r.observations, observation)
	return artifacts.Snapshot{}, r.err
}
