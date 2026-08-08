package authstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/testsupport/authlimit"
	"hubcr.io/hubcr/migrations"
)

const testPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$AQEBAQEBAQEBAQEBAQEBAQ$AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI"

func TestStoreIdentitySessionAndInvitationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL:            databaseURL,
		ConnectTimeout: 3 * time.Second,
		MaxConnections: 3,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	store := New(pool.ORM())
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	issuer := testIdentity("11111111-1111-4111-8111-111111111111", "owner", now)
	if err := store.CreateIdentity(ctx, issuer); err != nil {
		t.Fatalf("CreateIdentity(issuer) error = %v", err)
	}
	if err := store.CreateIdentity(ctx, testIdentity(
		"22222222-2222-4222-8222-222222222222",
		"owner",
		now,
	)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("CreateIdentity(duplicate username) error = %v, want ErrConflict", err)
	}

	found, err := store.CredentialByUsername(ctx, "owner")
	if err != nil {
		t.Fatalf("CredentialByUsername() error = %v", err)
	}
	if found.User != issuer.User || found.Credential.PasswordHash != testPasswordHash {
		t.Fatalf("CredentialByUsername() = %#v, want issuer identity", found)
	}

	session := auth.Session{
		ID:          "33333333-3333-4333-8333-333333333333",
		UserID:      issuer.User.ID,
		TokenDigest: auth.DigestSecret([]byte("session-secret-one")),
		ExpiresAt:   now.Add(24 * time.Hour),
		CreatedAt:   now,
	}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	duplicateSession := session
	duplicateSession.ID = "88888888-8888-4888-8888-888888888888"
	if err := store.CreateSession(ctx, duplicateSession); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("CreateSession(duplicate digest) error = %v, want ErrConflict", err)
	}
	storedSession, err := store.SessionByTokenDigest(ctx, session.TokenDigest)
	if err != nil {
		t.Fatalf("SessionByTokenDigest() error = %v", err)
	}
	if storedSession.ID != session.ID || storedSession.TokenDigest != session.TokenDigest {
		t.Fatalf("SessionByTokenDigest() = %#v, want %#v", storedSession, session)
	}
	revokedAt := now.Add(time.Hour)
	if err := store.RevokeSession(ctx, session.ID, revokedAt); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if err := store.RevokeSession(ctx, session.ID, revokedAt.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeSession(idempotent) error = %v", err)
	}
	storedSession, err = store.SessionByTokenDigest(ctx, session.TokenDigest)
	if err != nil || storedSession.RevokedAt == nil || !storedSession.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoked SessionByTokenDigest() = %#v, %v", storedSession, err)
	}

	issuerID := issuer.User.ID
	invitation := auth.Invitation{
		ID:             "44444444-4444-4444-8444-444444444444",
		IssuedByUserID: &issuerID,
		TokenDigest:    auth.DigestSecret([]byte("invitation-secret-one")),
		ExpiresAt:      now.Add(2 * time.Hour),
		CreatedAt:      now,
	}
	if err := store.CreateInvitation(ctx, invitation); err != nil {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
	invitee := testIdentity("55555555-5555-4555-8555-555555555555", "invitee", now.Add(time.Minute))
	redeemedAt := now.Add(30 * time.Minute)
	if err := store.RedeemInvitationWithIdentity(ctx, invitation.ID, redeemedAt, invitee); err != nil {
		t.Fatalf("RedeemInvitationWithIdentity() error = %v", err)
	}
	storedInvitation, err := store.InvitationByTokenDigest(ctx, invitation.TokenDigest)
	if err != nil {
		t.Fatalf("InvitationByTokenDigest() error = %v", err)
	}
	if storedInvitation.RedeemedAt == nil || !storedInvitation.RedeemedAt.Equal(redeemedAt) ||
		storedInvitation.RedeemedByUserID == nil || *storedInvitation.RedeemedByUserID != invitee.User.ID {
		t.Fatalf("redeemed invitation = %#v", storedInvitation)
	}

	rolledBackIdentity := testIdentity("66666666-6666-4666-8666-666666666666", "rolled-back", now)
	if err := store.RedeemInvitationWithIdentity(
		ctx,
		invitation.ID,
		redeemedAt.Add(time.Minute),
		rolledBackIdentity,
	); !errors.Is(err, auth.ErrInvalidState) {
		t.Fatalf("second RedeemInvitationWithIdentity() error = %v, want ErrInvalidState", err)
	}
	if _, err := store.CredentialByUsername(ctx, "rolled-back"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("rolled-back identity lookup error = %v, want ErrNotFound", err)
	}

	revocable := auth.Invitation{
		ID:          "77777777-7777-4777-8777-777777777777",
		TokenDigest: auth.DigestSecret([]byte("invitation-secret-two")),
		ExpiresAt:   now.Add(2 * time.Hour),
		CreatedAt:   now,
	}
	if err := store.CreateInvitation(ctx, revocable); err != nil {
		t.Fatalf("CreateInvitation(revocable) error = %v", err)
	}
	if err := store.RevokeInvitation(ctx, revocable.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("RevokeInvitation() error = %v", err)
	}
	if err := store.RevokeInvitation(ctx, revocable.ID, now.Add(90*time.Minute)); err != nil {
		t.Fatalf("RevokeInvitation(idempotent) error = %v", err)
	}

	expired := auth.Invitation{
		ID:          "99999999-9999-4999-8999-999999999999",
		TokenDigest: auth.DigestSecret([]byte("invitation-secret-expired")),
		ExpiresAt:   now.Add(time.Minute),
		CreatedAt:   now,
	}
	if err := store.CreateInvitation(ctx, expired); err != nil {
		t.Fatalf("CreateInvitation(expired) error = %v", err)
	}
	expiredIdentity := testIdentity("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "expired-invitee", now)
	if err := store.RedeemInvitationWithIdentity(
		ctx,
		expired.ID,
		now.Add(2*time.Minute),
		expiredIdentity,
	); !errors.Is(err, auth.ErrInvalidState) {
		t.Fatalf("RedeemInvitationWithIdentity(expired) error = %v, want ErrInvalidState", err)
	}
	if _, err := store.CredentialByUsername(ctx, "expired-invitee"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("expired invitation identity lookup error = %v, want ErrNotFound", err)
	}
}

func TestAuthServiceWithPostgresStore(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 3,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	hasher := auth.NewPasswordHasher()
	passwordHash, err := hasher.Hash([]byte("correct integration password"))
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	identity := testIdentity("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "service-owner", now)
	identity.Credential.PasswordHash = passwordHash
	store := New(pool.ORM())
	if err := store.CreateIdentity(ctx, identity); err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	service, err := auth.NewService(store, hasher, auth.ServiceOptions{
		SessionTTL: time.Hour,
		Random:     bytes.NewReader(bytes.Repeat([]byte{9}, 32)),
		Clock:      func() time.Time { return now },
		Limiter:    authlimit.AllowAll{},
	})
	if err != nil {
		t.Fatalf("auth.NewService() error = %v", err)
	}

	login, err := service.Login(ctx, auth.LoginInput{
		Username: "service-owner", Password: []byte("correct integration password"), RateLimitKey: "integration",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	user, err := service.Authenticate(ctx, login.Token)
	if err != nil || user.ID != identity.User.ID {
		t.Fatalf("Authenticate() = %#v, %v", user, err)
	}
	if err := service.Logout(ctx, login.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("Authenticate(revoked) error = %v, want ErrUnauthenticated", err)
	}
}

func testIdentity(id auth.ID, username string, now time.Time) auth.Identity {
	namespaceName := strings.ToLower(username)
	return auth.Identity{
		User: auth.User{
			ID:                id,
			Username:          username,
			PersonalNamespace: namespaceName,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		Credential: auth.LocalCredential{
			UserID:            id,
			PasswordHash:      testPasswordHash,
			PasswordChangedAt: now,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: namespaceName},
	}
}
