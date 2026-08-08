package registry

import (
	"errors"
	"time"
)

const (
	ResourceRepository = "repository"
	MaxScopes          = 16
	MaxScopeBytes      = 512
	MaxClientIDBytes   = 128
)

type Action string

const (
	ActionPull   Action = "pull"
	ActionPush   Action = "push"
	ActionDelete Action = "delete"
)

var (
	ErrInvalidRequest     = errors.New("invalid Registry token request")
	ErrUnsupportedRequest = errors.New("unsupported Registry token request")
	ErrInvalidCredentials = errors.New("invalid Registry credentials")
	ErrRateLimited        = errors.New("Registry authentication rate limit exceeded")
	ErrUnavailable        = errors.New("Registry token service unavailable")
	ErrInvalidToken       = errors.New("invalid Registry token")
)

type Credentials struct {
	Username string
	Password []byte
}

type Subject struct {
	ID string
}

func (s Subject) Anonymous() bool { return s.ID == "" }

type Scope struct {
	Type       string
	Name       string
	Namespace  string
	Repository string
	Actions    []Action
}

type Access struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []Action `json:"actions"`
}

type Claims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  string   `json:"aud"`
	ExpiresAt int64    `json:"exp"`
	NotBefore int64    `json:"nbf"`
	IssuedAt  int64    `json:"iat"`
	ID        string   `json:"jti"`
	Access    []Access `json:"access"`
}

type IssueRequest struct {
	Service      string
	RawScopes    []string
	ClientID     string
	RateLimitKey string
	Credentials  *Credentials
}

type IssueResult struct {
	Token     string
	ExpiresIn int
	IssuedAt  time.Time
	Subject   Subject
	Access    []Access
	KeyID     string
}
