package registryauth

import (
	"context"
	"errors"
	"fmt"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/registry"
)

type PasswordAuthenticator interface {
	AuthenticatePasswordAttempt(context.Context, auth.LoginAttempt, []byte) (auth.User, error)
}

type Authenticator struct {
	passwords PasswordAuthenticator
}

func New(passwords PasswordAuthenticator) (*Authenticator, error) {
	if passwords == nil {
		return nil, errors.New("Registry credential authenticator must be configured")
	}
	return &Authenticator{passwords: passwords}, nil
}

func (a *Authenticator) Authenticate(
	ctx context.Context,
	username string,
	password []byte,
	rateLimitKey string,
) (registry.Subject, error) {
	user, err := a.passwords.AuthenticatePasswordAttempt(
		ctx,
		auth.LoginAttempt{Username: username, Key: rateLimitKey},
		password,
	)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			return registry.Subject{}, registry.ErrInvalidCredentials
		}
		if errors.Is(err, auth.ErrRateLimited) {
			return registry.Subject{}, registry.ErrRateLimited
		}
		return registry.Subject{}, fmt.Errorf("authenticate Registry credential: %w", err)
	}
	if user.ID == "" {
		return registry.Subject{}, errors.New("authenticate Registry credential: empty user ID")
	}
	return registry.Subject{ID: string(user.ID)}, nil
}

var _ registry.CredentialAuthenticator = (*Authenticator)(nil)
