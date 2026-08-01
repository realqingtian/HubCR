package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceLoginAuthenticateAndLogout(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	identity := Identity{
		User: User{ID: "11111111-1111-4111-8111-111111111111", Username: "owner", CreatedAt: now, UpdatedAt: now},
		Credential: LocalCredential{
			UserID:       "11111111-1111-4111-8111-111111111111",
			PasswordHash: "stored-hash",
		},
	}
	store := &serviceTestStore{identity: identity, user: identity.User}
	service, err := NewService(store, serviceTestPasswords{}, ServiceOptions{
		SessionTTL: 24 * time.Hour,
		Random:     bytes.NewReader(bytes.Repeat([]byte{7}, sessionSecretBytes)),
		Clock:      func() time.Time { return now },
		Limiter:    AllowAllLoginLimiter{},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Login(context.Background(), LoginInput{
		Username:     "owner",
		Password:     []byte("correct"),
		RateLimitKey: "client",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.User != identity.User || result.Token == "" || !result.ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("Login() = %#v", result)
	}
	if store.session.TokenDigest != DigestSecret([]byte(result.Token)) {
		t.Fatal("Login() did not persist the returned token digest")
	}

	store.session = Session{
		ID:          store.session.ID,
		UserID:      identity.User.ID,
		TokenDigest: DigestSecret([]byte(result.Token)),
		ExpiresAt:   result.ExpiresAt,
		CreatedAt:   now,
	}
	user, err := service.Authenticate(context.Background(), result.Token)
	if err != nil || user != identity.User {
		t.Fatalf("Authenticate() = %#v, %v", user, err)
	}
	if err := service.Logout(context.Background(), result.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if store.revokedSession != store.session.ID {
		t.Fatalf("revoked session = %q, want %q", store.revokedSession, store.session.ID)
	}
}

func TestServiceAuthenticationFailuresAreUniform(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		store *serviceTestStore
		login LoginInput
	}{
		{
			name:  "unknown user",
			store: &serviceTestStore{credentialError: ErrNotFound},
			login: LoginInput{Username: "missing", Password: []byte("correct")},
		},
		{
			name: "wrong password",
			store: &serviceTestStore{identity: Identity{
				User:       User{ID: "11111111-1111-4111-8111-111111111111", Username: "owner"},
				Credential: LocalCredential{PasswordHash: "stored-hash"},
			}},
			login: LoginInput{Username: "owner", Password: []byte("wrong")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(test.store, serviceTestPasswords{}, ServiceOptions{
				SessionTTL: time.Hour,
				Random:     bytes.NewReader(bytes.Repeat([]byte{1}, sessionSecretBytes)),
				Clock:      func() time.Time { return now },
				Limiter:    AllowAllLoginLimiter{},
			})
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}
			if _, err := service.Login(context.Background(), test.login); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("Login() error = %v, want ErrUnauthenticated", err)
			}
		})
	}
}

func TestServiceRejectsExpiredOrRevokedSession(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)
	for _, session := range []Session{
		{ExpiresAt: now},
		{ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt},
	} {
		store := &serviceTestStore{session: session}
		service, err := NewService(store, serviceTestPasswords{}, ServiceOptions{
			SessionTTL: time.Hour,
			Random:     bytes.NewReader(bytes.Repeat([]byte{1}, sessionSecretBytes)),
			Clock:      func() time.Time { return now },
			Limiter:    AllowAllLoginLimiter{},
		})
		if err != nil {
			t.Fatalf("NewService() error = %v", err)
		}
		if _, err := service.Authenticate(context.Background(), "token"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
		}
	}
}

type serviceTestPasswords struct{}

func (serviceTestPasswords) Hash([]byte) (string, error) { return "dummy-hash", nil }
func (serviceTestPasswords) Verify(password []byte, encoded string) (bool, error) {
	return string(password) == "correct" && encoded != "", nil
}

type serviceTestStore struct {
	identity        Identity
	credentialError error
	user            User
	session         Session
	sessionError    error
	revokedSession  ID
}

func (s *serviceTestStore) CreateIdentity(context.Context, Identity) error { return nil }
func (s *serviceTestStore) CredentialByUsername(context.Context, string) (Identity, error) {
	return s.identity, s.credentialError
}
func (s *serviceTestStore) UserByID(context.Context, ID) (User, error) { return s.user, nil }
func (s *serviceTestStore) CreateSession(_ context.Context, session Session) error {
	s.session = session
	return nil
}
func (s *serviceTestStore) SessionByTokenDigest(context.Context, SecretDigest) (Session, error) {
	return s.session, s.sessionError
}
func (s *serviceTestStore) RevokeSession(_ context.Context, id ID, _ time.Time) error {
	s.revokedSession = id
	return nil
}
func (s *serviceTestStore) CreateInvitation(context.Context, Invitation) error { return nil }
func (s *serviceTestStore) InvitationByTokenDigest(context.Context, SecretDigest) (Invitation, error) {
	return Invitation{}, ErrNotFound
}
func (s *serviceTestStore) RedeemInvitationWithIdentity(context.Context, ID, time.Time, Identity) error {
	return nil
}
func (s *serviceTestStore) RevokeInvitation(context.Context, ID, time.Time) error { return nil }
