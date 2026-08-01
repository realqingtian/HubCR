package controlplane

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/health"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
	"hubcr.io/hubcr/internal/platform/httpapi/organizationhandler"
	"hubcr.io/hubcr/internal/platform/httpapi/registryhandler"
	"hubcr.io/hubcr/internal/platform/httpapi/repositoryhandler"
	"hubcr.io/hubcr/internal/platform/httpserver"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/internal/platform/registryauth"
)

// App composes the control-plane modules and their infrastructure adapters.
type App struct {
	server   *httpserver.Server
	database *postgres.Pool
}

func New(ctx context.Context, cfg config.API, logger *slog.Logger) (*App, error) {
	database, err := postgres.Open(ctx, postgres.Options{
		URL:            cfg.Database.URL,
		ConnectTimeout: cfg.Database.ConnectTimeout,
		MaxConnections: cfg.Database.MaxConnections,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL: %w", err)
	}

	router := httpapi.NewRouter()
	health.RegisterRoutes(router, cfg.Database.HealthCheckTimeout, database)
	authService, err := auth.NewService(
		authstore.New(database.ORM()),
		auth.NewPasswordHasher(),
		auth.ServiceOptions{
			SessionTTL: cfg.Authentication.SessionTTL,
			Random:     rand.Reader,
			Clock:      time.Now,
			Limiter:    auth.AllowAllLoginLimiter{},
		},
	)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize authentication service: %w", err)
	}
	authhandler.RegisterRoutes(router, authhandler.New(authService, cfg.Authentication.SessionCookieSecure))
	policy := authorization.NewPolicy()
	organizationService, err := organizations.NewService(
		organizationstore.New(database.ORM()), namespaces.NormalizeName, time.Now, policy,
	)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize organization service: %w", err)
	}
	organizationhandler.RegisterRoutes(router, organizationhandler.New(authService, organizationService))
	repositoryService, err := repositories.NewService(repositorystore.New(database.ORM()), policy, time.Now)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize repository service: %w", err)
	}
	repositoryhandler.RegisterRoutes(router, repositoryhandler.New(authService, repositoryService))
	if cfg.Registry.Enabled {
		if err := registerRegistryRoutes(
			router, cfg.Registry, authService, repositoryService, policy, logger,
		); err != nil {
			database.Close()
			return nil, err
		}
	}
	handler := httpapi.WithRequestID(httpapi.Recover(router))

	return &App{
		server:   httpserver.New(cfg.Address, cfg.ShutdownTimeout, handler, logger),
		database: database,
	}, nil
}

func registerRegistryRoutes(
	router *httpapi.Router,
	cfg config.Registry,
	authService *auth.Service,
	repositoryService *repositories.Service,
	policy authorization.Policy,
	logger *slog.Logger,
) error {
	privateKeyPEM, err := os.ReadFile(cfg.PrivateKeyFile)
	if err != nil {
		return errors.New("initialize Registry token signing key: file is unreadable")
	}
	defer clear(privateKeyPEM)
	privateKey, err := registry.ParseRSAPrivateKeyPEM(privateKeyPEM)
	if err != nil {
		return fmt.Errorf("initialize Registry token signing key: %w", err)
	}
	signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
	if err != nil {
		return fmt.Errorf("initialize Registry token signer: %w", err)
	}
	publicJWKS, err := os.ReadFile(cfg.PublicJWKSFile)
	if err != nil {
		return errors.New("initialize Registry token trust set: file is unreadable")
	}
	trustedKeys, err := registry.ParseRS256JWKS(publicJWKS)
	if err != nil {
		return fmt.Errorf("initialize Registry token trust set: %w", err)
	}
	activePublicKey, exists := trustedKeys[signer.KeyID()]
	if !exists || activePublicKey.E != privateKey.PublicKey.E ||
		activePublicKey.N.Cmp(privateKey.PublicKey.N) != 0 {
		return errors.New("initialize Registry token trust set: active signing key is not trusted")
	}
	authenticator, err := registryauth.New(authService)
	if err != nil {
		return fmt.Errorf("initialize Registry credential authenticator: %w", err)
	}
	tokenService, err := registry.NewService(
		authenticator, repositoryService, policy, signer,
		registry.ServiceOptions{
			Service: cfg.Service, Issuer: cfg.Issuer,
			TokenTTL: cfg.TokenTTL, ClockSkew: cfg.ClockSkew,
			Clock: time.Now, Random: rand.Reader,
		},
	)
	if err != nil {
		return fmt.Errorf("initialize Registry token service: %w", err)
	}
	handler, err := registryhandler.New(tokenService, logger)
	if err != nil {
		return fmt.Errorf("initialize Registry token handler: %w", err)
	}
	registryhandler.RegisterRoutes(router, handler)
	return nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.database.Close()
	return a.server.Run(ctx)
}
