package artifacts

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceRejectsInvalidObservationBeforeStore(t *testing.T) {
	store := &serviceStore{}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	_, err = service.ReconcileArtifact(context.Background(), Observation{
		RepositoryID: "repository-id",
		Digest:       "invalid",
		Kind:         "MANIFEST",
		ObservedAt:   time.Now(),
	})
	if !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("ReconcileArtifact() error = %v, want ErrInvalidDigest", err)
	}
	if store.reconcileCalls != 0 {
		t.Fatalf("Store.Reconcile() calls = %d, want 0", store.reconcileCalls)
	}
}

func TestServicePassesNormalizedObservationToStore(t *testing.T) {
	store := &serviceStore{snapshot: Snapshot{Artifact: Artifact{RepositoryID: "repository-id"}}}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.ReconcileArtifact(context.Background(), Observation{
		RepositoryID: "repository-id",
		Digest:       testDigest,
		Kind:         "MANIFEST",
		ObservedAt:   time.Date(2026, 8, 1, 20, 0, 0, 0, time.FixedZone("test", 8*60*60)),
	})
	if err != nil {
		t.Fatalf("ReconcileArtifact() error = %v", err)
	}
	if result.Artifact.RepositoryID != "repository-id" ||
		store.reconciliation.Artifact.Digest.String() != testDigest ||
		store.reconciliation.ObservedAt.Location() != time.UTC {
		t.Fatalf("result/input = %#v / %#v", result, store.reconciliation)
	}
}

type serviceStore struct {
	reconciliation Reconciliation
	snapshot       Snapshot
	err            error
	reconcileCalls int
}

func (s *serviceStore) Reconcile(_ context.Context, input Reconciliation) (Snapshot, error) {
	s.reconcileCalls++
	s.reconciliation = input
	return s.snapshot, s.err
}

func (s *serviceStore) RemoveTag(context.Context, string, TagName) error { return s.err }
