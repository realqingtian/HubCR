package security

import (
	"context"
	"errors"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/modules/organizations"
)

// stubTrustStore records calls and returns configured responses for the management paths.
type stubTrustStore struct {
	createPolicy        func(context.Context, string, string, []PublicKeyTrust, []KeylessIdentity, time.Time) (TrustPolicy, error)
	currentPolicy       func(context.Context, string) (TrustPolicy, error)
	repairForNamespace  func(context.Context, string, int, time.Time) (int, error)
	createCalls         int
	repairNamespaceArgs []string
}

func (s *stubTrustStore) CreateTrustPolicy(ctx context.Context, ns, actor string, keys []PublicKeyTrust, ids []KeylessIdentity, now time.Time) (TrustPolicy, error) {
	s.createCalls++
	return s.createPolicy(ctx, ns, actor, keys, ids, now)
}
func (s *stubTrustStore) CurrentTrustPolicy(_ context.Context, _ string) (TrustPolicy, error) {
	return s.currentPolicy(nil, "")
}
func (s *stubTrustStore) EnsureCurrentVerification(context.Context, Target, time.Time) (VerificationWorkflow, bool, error) {
	return VerificationWorkflow{}, false, nil
}
func (s *stubTrustStore) RepairMissingVerificationWorkflows(context.Context, int, time.Time) (int, error) {
	return 0, nil
}
func (s *stubTrustStore) RepairMissingVerificationWorkflowsForNamespace(_ context.Context, namespaceID string, _ int, _ time.Time) (int, error) {
	s.repairNamespaceArgs = append(s.repairNamespaceArgs, namespaceID)
	if s.repairForNamespace != nil {
		return s.repairForNamespace(nil, namespaceID, 0, time.Time{})
	}
	return 1, nil
}
func (s *stubTrustStore) ResolveVerificationJob(context.Context, jobs.Job) (VerificationInput, error) {
	return VerificationInput{}, nil
}
func (s *stubTrustStore) SaveVerificationResult(context.Context, VerificationResult) error {
	return nil
}

// stubTrustAuth is a configurable TrustAuthorization for the management tests.
type stubTrustAuth struct {
	personal bool
	org      map[organizations.Role]bool
}

func (a stubTrustAuth) AllowsOrganization(role organizations.Role, _ authorization.Capability) bool {
	return a.org[role]
}
func (a stubTrustAuth) AllowsPersonalNamespace(isOwner bool, _ authorization.Capability) bool {
	return a.personal && isOwner
}

func newManagingService(t *testing.T, auth TrustAuthorization, resolve NamespaceAccessResolver) (*TrustService, *stubTrustStore) {
	t.Helper()
	store := &stubTrustStore{
		createPolicy: func(_ context.Context, ns, _ string, _ []PublicKeyTrust, _ []KeylessIdentity, _ time.Time) (TrustPolicy, error) {
			return TrustPolicy{ID: "policy-id", NamespaceID: ns, Version: 1, CreatedByUserID: "actor",
				PublicKeys: []PublicKeyTrust{testPublicKey(t, "k")}, CreatedAt: time.Now().UTC()}, nil
		},
		currentPolicy: func(context.Context, string) (TrustPolicy, error) {
			return TrustPolicy{ID: "policy-id", NamespaceID: "ns", Version: 2, CreatedByUserID: "actor",
				PublicKeys: []PublicKeyTrust{testPublicKey(t, "k")}, CreatedAt: time.Now().UTC()}, nil
		},
	}
	svc, err := NewManagingTrustService(store, time.Now, auth, resolve)
	if err != nil {
		t.Fatalf("NewManagingTrustService() error = %v", err)
	}
	return svc, store
}

func TestCreatePolicyAuthorizesNamespaceOwnerOnly(t *testing.T) {
	key := testPublicKey(t, "release")
	resolve := func(_ context.Context, _, _ string) (NamespaceAccess, error) {
		return NamespaceAccess{NamespaceID: "ns", Kind: NamespacePersonal, IsPersonalOwner: true}, nil
	}

	// Personal owner succeeds and triggers eager namespace repair.
	svc, store := newManagingService(t, stubTrustAuth{personal: true}, resolve)
	policy, err := svc.CreatePolicy(context.Background(), "ns", "actor", []PublicKeyTrust{key}, nil)
	if err != nil || policy.Version != 1 || store.createCalls != 1 {
		t.Fatalf("CreatePolicy(owner) = %#v, %v; createCalls=%d", policy, err, store.createCalls)
	}
	if len(store.repairNamespaceArgs) != 1 || store.repairNamespaceArgs[0] != "ns" {
		t.Fatalf("eager repair args = %#v; want [ns]", store.repairNamespaceArgs)
	}

	// Non-owner is denied before any store write, and no repair runs.
	svc2, store2 := newManagingService(t, stubTrustAuth{personal: false}, resolve)
	_, err = svc2.CreatePolicy(context.Background(), "ns", "actor", []PublicKeyTrust{key}, nil)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreatePolicy(non-owner) error = %v; want ErrForbidden", err)
	}
	if store2.createCalls != 0 || len(store2.repairNamespaceArgs) != 0 {
		t.Fatalf("non-owner wrote or repaired: createCalls=%d repair=%v", store2.createCalls, store2.repairNamespaceArgs)
	}

	// Empty actor is denied.
	_, err = svc.CreatePolicy(context.Background(), "ns", "", []PublicKeyTrust{key}, nil)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreatePolicy(empty actor) error = %v; want ErrForbidden", err)
	}
}

func TestCreatePolicyBlocksOrganizationNonOwners(t *testing.T) {
	key := testPublicKey(t, "release")
	for _, tc := range []struct {
		name    string
		role    organizations.Role
		allowed bool
	}{
		{"owner", organizations.RoleOwner, true},
		{"admin", organizations.RoleAdmin, false},
		{"writer", organizations.RoleWriter, false},
		{"reader", organizations.RoleReader, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolve := func(_ context.Context, _, _ string) (NamespaceAccess, error) {
				return NamespaceAccess{NamespaceID: "ns", Kind: NamespaceOrganization, OrganizationRole: tc.role}, nil
			}
			auth := stubTrustAuth{org: map[organizations.Role]bool{organizations.RoleOwner: true}}
			svc, store := newManagingService(t, auth, resolve)
			_, err := svc.CreatePolicy(context.Background(), "ns", "actor", []PublicKeyTrust{key}, nil)
			if tc.allowed && (err != nil || store.createCalls != 1) {
				t.Fatalf("CreatePolicy(%s) error = %v; want success", tc.name, err)
			}
			if !tc.allowed && !errors.Is(err, ErrForbidden) {
				t.Fatalf("CreatePolicy(%s) error = %v; want ErrForbidden", tc.name, err)
			}
		})
	}
}

func TestCreatePolicyRejectsInvalidSubjects(t *testing.T) {
	resolve := func(_ context.Context, _, _ string) (NamespaceAccess, error) {
		return NamespaceAccess{NamespaceID: "ns", Kind: NamespacePersonal, IsPersonalOwner: true}, nil
	}
	svc, _ := newManagingService(t, stubTrustAuth{personal: true}, resolve)
	// No subjects at all.
	if _, err := svc.CreatePolicy(context.Background(), "ns", "actor", nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CreatePolicy(no subjects) error = %v; want ErrInvalid", err)
	}
	// Invalid key (bad fingerprint).
	badKey := PublicKeyTrust{Fingerprint: "bad", Name: "k", PublicKeyPEM: "not-a-key"}
	if _, err := svc.CreatePolicy(context.Background(), "ns", "actor", []PublicKeyTrust{badKey}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CreatePolicy(bad key) error = %v; want ErrInvalid", err)
	}
}

func TestCurrentPolicyRespectsViewOrganization(t *testing.T) {
	resolve := func(_ context.Context, _, _ string) (NamespaceAccess, error) {
		return NamespaceAccess{NamespaceID: "ns", Kind: NamespaceOrganization, OrganizationRole: organizations.RoleReader}, nil
	}
	// Reader has ViewOrganization, so read succeeds.
	svc, _ := newManagingService(t, stubTrustAuth{
		org: map[organizations.Role]bool{
			organizations.RoleOwner: true, organizations.RoleAdmin: true,
			organizations.RoleWriter: true, organizations.RoleReader: true,
		},
	}, resolve)
	policy, err := svc.CurrentPolicy(context.Background(), "ns", "actor")
	if err != nil || policy.Version != 2 {
		t.Fatalf("CurrentPolicy(reader) = %#v, %v", policy, err)
	}

	// Non-member (no role match) is denied.
	resolveNonMember := func(_ context.Context, _, _ string) (NamespaceAccess, error) {
		return NamespaceAccess{NamespaceID: "ns", Kind: NamespaceOrganization, OrganizationRole: ""}, nil
	}
	svc2, _ := newManagingService(t, stubTrustAuth{
		org: map[organizations.Role]bool{
			organizations.RoleOwner: true, organizations.RoleAdmin: true,
			organizations.RoleWriter: true, organizations.RoleReader: true,
		},
	}, resolveNonMember)
	if _, err := svc2.CurrentPolicy(context.Background(), "ns", "actor"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("CurrentPolicy(non-member) error = %v; want ErrForbidden", err)
	}
}

func TestWorkerTrustServiceRejectsManagementPaths(t *testing.T) {
	store := &stubTrustStore{}
	svc, err := NewTrustService(store, time.Now)
	if err != nil {
		t.Fatalf("NewTrustService() error = %v", err)
	}
	if _, err := svc.CreatePolicy(context.Background(), "ns", "actor", []PublicKeyTrust{testPublicKey(t, "k")}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("worker CreatePolicy error = %v; want ErrForbidden", err)
	}
	if _, err := svc.CurrentPolicy(context.Background(), "ns", "actor"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("worker CurrentPolicy error = %v; want ErrForbidden", err)
	}
}

func TestCurrentPolicyPropagatesNotFound(t *testing.T) {
	resolve := func(_ context.Context, _, _ string) (NamespaceAccess, error) {
		return NamespaceAccess{NamespaceID: "ns", Kind: NamespacePersonal, IsPersonalOwner: true}, nil
	}
	store := &stubTrustStore{
		currentPolicy: func(context.Context, string) (TrustPolicy, error) { return TrustPolicy{}, ErrNotFound },
	}
	svc, err := NewManagingTrustService(store, time.Now, stubTrustAuth{personal: true}, resolve)
	if err != nil {
		t.Fatalf("NewManagingTrustService() error = %v", err)
	}
	if _, err := svc.CurrentPolicy(context.Background(), "ns", "actor"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CurrentPolicy(missing) error = %v; want ErrNotFound", err)
	}
}
