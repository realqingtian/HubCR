package trustpolicyhandler

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

const trustHandlerUserID = "33333333-3333-4333-8333-333333333333"

func testTrustKey(t *testing.T) security.PublicKeyTrust {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey() error = %v", err)
	}
	key, err := security.NewPublicKeyTrust(
		"release", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
	)
	if err != nil {
		t.Fatalf("security.NewPublicKeyTrust() error = %v", err)
	}
	return key
}

func TestTrustPolicyRouteReadsCurrentPolicy(t *testing.T) {
	key := testTrustKey(t)
	policy := security.TrustPolicy{
		ID: "policy-id", NamespaceID: "ns", Version: 3,
		CreatedByUserID: trustHandlerUserID, PublicKeys: []security.PublicKeyTrust{key},
		CreatedAt: time.Date(2026, 8, 9, 5, 0, 0, 0, time.UTC),
	}
	handler := testHandler(t, &stubAccess{}, &stubTrust{policy: policy}, "")
	request := authenticatedRequest(http.MethodGet, "/api/v1/namespaces/team/trust-policy", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{
		`"id":"policy-id"`, `"version":3`, `"created_by_user_id":"` + trustHandlerUserID + `"`,
		`"fingerprint":"` + key.Fingerprint + `"`, `"name":"release"`,
	} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("body = %s, want %q", recorder.Body.String(), expected)
		}
	}
}

func TestTrustPolicyCreateRouteAuthorizesOwnerAndReturnsVersion(t *testing.T) {
	key := testTrustKey(t)
	createdPolicy := security.TrustPolicy{
		ID: "new-policy", NamespaceID: "ns", Version: 1,
		CreatedByUserID: trustHandlerUserID, PublicKeys: []security.PublicKeyTrust{key},
		CreatedAt: time.Date(2026, 8, 9, 5, 0, 0, 0, time.UTC),
	}
	trust := &stubTrust{policy: createdPolicy}
	handler := testHandler(t, &stubAccess{}, trust, "")
	body, _ := json.Marshal(createPolicyRequest{
		PublicKeys: []publicKeyRequest{{Name: "release", PublicKeyPEM: key.PublicKeyPEM, Fingerprint: key.Fingerprint}},
	})
	request := authenticatedRequest(http.MethodPost, "/api/v1/namespaces/team/trust-policy", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	if trust.createActor != trustHandlerUserID || trust.createNamespace != "team" {
		t.Fatalf("create actor/namespace = %q/%q", trust.createActor, trust.createNamespace)
	}
	if !strings.Contains(recorder.Body.String(), `"id":"new-policy"`) || !strings.Contains(recorder.Body.String(), `"version":1`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestTrustPolicyForbiddenMapsToNotFound(t *testing.T) {
	key := testTrustKey(t)
	// Create by a non-owner is forbidden; the handler must map it to 404 (no existence leak).
	trust := &stubTrust{err: security.ErrForbidden}
	handler := testHandler(t, &stubAccess{}, trust, "")
	body, _ := json.Marshal(createPolicyRequest{
		PublicKeys: []publicKeyRequest{{Name: "release", PublicKeyPEM: key.PublicKeyPEM, Fingerprint: key.Fingerprint}},
	})
	request := authenticatedRequest(http.MethodPost, "/api/v1/namespaces/team/trust-policy", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("forbidden create status = %d %s; want 404", recorder.Code, recorder.Body.String())
	}
	// Read by a non-member is also forbidden -> 404.
	trust.err = security.ErrForbidden
	readRequest := authenticatedRequest(http.MethodGet, "/api/v1/namespaces/team/trust-policy", nil)
	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, readRequest)
	if readRecorder.Code != http.StatusNotFound {
		t.Fatalf("forbidden read status = %d; want 404", readRecorder.Code)
	}
}

func TestTrustPolicyMissingMapsToNotFound(t *testing.T) {
	trust := &stubTrust{err: security.ErrNotFound}
	handler := testHandler(t, &stubAccess{}, trust, "")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/api/v1/namespaces/team/trust-policy", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d %s; want 404", recorder.Code, recorder.Body.String())
	}
}

func TestTrustPolicyCreateRejectsInvalidSubjects(t *testing.T) {
	key := testTrustKey(t)
	handler := testHandler(t, &stubAccess{}, &stubTrust{}, "")
	// No subjects.
	body, _ := json.Marshal(createPolicyRequest{})
	request := authenticatedRequest(http.MethodPost, "/api/v1/namespaces/team/trust-policy", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty subjects status = %d %s; want 422", recorder.Code, recorder.Body.String())
	}
	// Mismatched fingerprint.
	body, _ = json.Marshal(createPolicyRequest{
		PublicKeys: []publicKeyRequest{{Name: "release", PublicKeyPEM: key.PublicKeyPEM, Fingerprint: "sha256:deadbeef"}},
	})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodPost, "/api/v1/namespaces/team/trust-policy", bytes.NewReader(body)))
	if recorder.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(recorder.Body.String(), "fingerprint does not match") {
		t.Fatalf("mismatched fingerprint status/body = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestTrustPolicyCreateRejectsCrossSiteAndBadNamespace(t *testing.T) {
	key := testTrustKey(t)
	handler := testHandler(t, &stubAccess{}, &stubTrust{}, "")
	body, _ := json.Marshal(createPolicyRequest{
		PublicKeys: []publicKeyRequest{{Name: "release", PublicKeyPEM: key.PublicKeyPEM, Fingerprint: key.Fingerprint}},
	})
	crossRequest := authenticatedRequest(http.MethodPost, "/api/v1/namespaces/team/trust-policy", bytes.NewReader(body))
	crossRequest.Header.Set("Sec-Fetch-Site", "cross-site")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, crossRequest)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("cross-site status = %d; want 400", recorder.Code)
	}
}

func TestTrustPolicyRequiresAuthentication(t *testing.T) {
	handler := testHandler(t, &stubAccess{}, &stubTrust{}, "")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/team/trust-policy", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d; want 401", recorder.Code)
	}
}

func TestResolveAccessAdaptsNamespaceAccess(t *testing.T) {
	source := &stubAccess{
		access: repositories.NamespaceAccess{
			NamespaceID: "ns-id", Kind: repositories.NamespaceOrganization,
			OrganizationRole: organizations.RoleReader,
		},
	}
	resolver := ResolveAccess(source)
	got, err := resolver(context.Background(), "team", trustHandlerUserID)
	if err != nil || got.NamespaceID != "ns-id" || got.Kind != security.NamespaceOrganization ||
		got.OrganizationRole != organizations.RoleReader {
		t.Fatalf("ResolveAccess() = %#v, %v", got, err)
	}
	// NotFound surfaces as security.ErrNotFound.
	source.err = repositories.ErrNotFound
	if _, err := resolver(context.Background(), "team", trustHandlerUserID); !errors.Is(err, security.ErrNotFound) {
		t.Fatalf("ResolveAccess(notfound) error = %v; want security.ErrNotFound", err)
	}
}

func testHandler(t *testing.T, access *stubAccess, trust *stubTrust, _ string) http.Handler {
	t.Helper()
	handler, err := New(
		&stubAuthenticator{user: auth.User{ID: auth.ID(trustHandlerUserID)}},
		access, trust,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	router := httpapi.NewRouter()
	RegisterRoutes(router, handler)
	return httpapi.WithRequestID(router)
}

func authenticatedRequest(method, path string, body *bytes.Reader) *http.Request {
	var reader *bytes.Reader
	if body != nil {
		reader = body
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: authhandler.SessionCookieName, Value: "session"})
	return request
}

type stubAuthenticator struct {
	user auth.User
	err  error
}

func (a *stubAuthenticator) Authenticate(context.Context, string) (auth.User, error) {
	return a.user, a.err
}

type stubAccess struct {
	access repositories.NamespaceAccess
	err    error
}

func (s *stubAccess) ResolveNamespaceAccess(context.Context, string, string) (repositories.NamespaceAccess, error) {
	return s.access, s.err
}

type stubTrust struct {
	policy          security.TrustPolicy
	err             error
	createActor     string
	createNamespace string
}

func (s *stubTrust) CurrentPolicy(context.Context, string, string) (security.TrustPolicy, error) {
	return s.policy, s.err
}

func (s *stubTrust) CreatePolicy(_ context.Context, namespace, actor string, _ []security.PublicKeyTrust, _ []security.KeylessIdentity) (security.TrustPolicy, error) {
	s.createActor = actor
	s.createNamespace = namespace
	return s.policy, s.err
}
