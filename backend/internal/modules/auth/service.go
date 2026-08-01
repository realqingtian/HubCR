package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"
)

const sessionSecretBytes = 32

var (
	ErrUnauthenticated = errors.New("authentication failed")
	ErrRateLimited     = errors.New("authentication rate limit exceeded")
)

type PasswordCodec interface {
	Hash([]byte) (string, error)
	Verify([]byte, string) (bool, error)
}

type LoginAttempt struct {
	Username string
	Key      string
}

type LoginLimiter interface {
	Allow(context.Context, LoginAttempt) error
}

type AllowAllLoginLimiter struct{}

func (AllowAllLoginLimiter) Allow(context.Context, LoginAttempt) error { return nil }

type ServiceOptions struct {
	SessionTTL time.Duration
	Random     io.Reader
	Clock      func() time.Time
	Limiter    LoginLimiter
}

type Service struct {
	store         Store
	passwords     PasswordCodec
	sessionTTL    time.Duration
	random        io.Reader
	clock         func() time.Time
	limiter       LoginLimiter
	dummyPassword string
}

type LoginInput struct {
	Username     string
	Password     []byte
	RateLimitKey string
}

type LoginResult struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

func NewService(store Store, passwords PasswordCodec, options ServiceOptions) (*Service, error) {
	if store == nil || passwords == nil || options.Random == nil || options.Clock == nil ||
		options.Limiter == nil || options.SessionTTL <= 0 {
		return nil, errors.New("auth service dependencies and session TTL must be configured")
	}
	dummyPassword, err := passwords.Hash([]byte("hubcr-dummy-authentication-password"))
	if err != nil {
		return nil, fmt.Errorf("initialize constant-work authentication hash: %w", err)
	}
	return &Service{
		store:         store,
		passwords:     passwords,
		sessionTTL:    options.SessionTTL,
		random:        options.Random,
		clock:         options.Clock,
		limiter:       options.Limiter,
		dummyPassword: dummyPassword,
	}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	if err := s.limiter.Allow(ctx, LoginAttempt{Username: input.Username, Key: input.RateLimitKey}); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return LoginResult{}, ErrRateLimited
		}
		return LoginResult{}, fmt.Errorf("check login limit: %w", err)
	}

	user, err := s.AuthenticatePassword(ctx, input.Username, input.Password)
	if err != nil {
		return LoginResult{}, err
	}

	secret := make([]byte, sessionSecretBytes)
	if _, err := io.ReadFull(s.random, secret); err != nil {
		return LoginResult{}, fmt.Errorf("generate session secret: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	now := s.clock().UTC()
	sessionID, err := NewID()
	if err != nil {
		return LoginResult{}, err
	}
	expiresAt := now.Add(s.sessionTTL)
	if err := s.store.CreateSession(ctx, Session{
		ID:          sessionID,
		UserID:      user.ID,
		TokenDigest: DigestSecret([]byte(token)),
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}); err != nil {
		return LoginResult{}, fmt.Errorf("persist session: %w", err)
	}
	return LoginResult{User: user, Token: token, ExpiresAt: expiresAt}, nil
}

// AuthenticatePassword verifies a local credential without creating a web session.
// Registry adapters use this boundary so Registry credentials and browser sessions
// remain separate.
func (s *Service) AuthenticatePassword(ctx context.Context, username string, password []byte) (User, error) {
	identity, err := s.store.CredentialByUsername(ctx, username)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return User{}, fmt.Errorf("load local credential: %w", err)
		}
		_, _ = s.passwords.Verify(password, s.dummyPassword)
		return User{}, ErrUnauthenticated
	}
	verified, err := s.passwords.Verify(password, identity.Credential.PasswordHash)
	if err != nil {
		return User{}, fmt.Errorf("verify stored credential: %w", err)
	}
	if !verified {
		return User{}, ErrUnauthenticated
	}
	return identity.User, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthenticated
	}
	session, err := s.store.SessionByTokenDigest(ctx, DigestSecret([]byte(token)))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrUnauthenticated
		}
		return User{}, fmt.Errorf("load session: %w", err)
	}
	if session.RevokedAt != nil || !s.clock().UTC().Before(session.ExpiresAt) {
		return User{}, ErrUnauthenticated
	}
	user, err := s.store.UserByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrUnauthenticated
		}
		return User{}, fmt.Errorf("load session user: %w", err)
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	session, err := s.store.SessionByTokenDigest(ctx, DigestSecret([]byte(token)))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load session for logout: %w", err)
	}
	if err := s.store.RevokeSession(ctx, session.ID, s.clock().UTC()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
