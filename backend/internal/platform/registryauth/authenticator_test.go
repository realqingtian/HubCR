package registryauth

import (
	"context"
	"errors"
	"testing"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/registry"
)

func TestAuthenticatorMapsLocalIdentityWithoutSessionCredential(t *testing.T) {
	passwords := &passwordService{
		user: auth.User{ID: "11111111-1111-4111-8111-111111111111", Username: "owner"},
	}
	authenticator, err := New(passwords)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	subject, err := authenticator.Authenticate(
		context.Background(), "owner", []byte("correct"),
	)
	if err != nil || subject.ID != string(passwords.user.ID) {
		t.Fatalf("Authenticate() = %#v, %v", subject, err)
	}
	if passwords.username != "owner" || string(passwords.password) != "correct" {
		t.Fatalf("password input = %q %q", passwords.username, passwords.password)
	}
}

func TestAuthenticatorClassifiesInvalidCredential(t *testing.T) {
	authenticator, err := New(&passwordService{err: auth.ErrUnauthenticated})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := authenticator.Authenticate(
		context.Background(), "owner", []byte("wrong"),
	); !errors.Is(err, registry.ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
	}
}

type passwordService struct {
	user     auth.User
	err      error
	username string
	password []byte
}

func (s *passwordService) AuthenticatePassword(
	_ context.Context,
	username string,
	password []byte,
) (auth.User, error) {
	s.username = username
	s.password = append([]byte(nil), password...)
	return s.user, s.err
}
