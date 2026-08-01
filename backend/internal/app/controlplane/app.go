package controlplane

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/health"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
	"hubcr.io/hubcr/internal/platform/httpapi/organizationhandler"
	"hubcr.io/hubcr/internal/platform/httpserver"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
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
	organizationService, err := organizations.NewService(
		organizationstore.New(database.ORM()), namespaces.NormalizeName, time.Now, authorization.NewPolicy(),
	)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize organization service: %w", err)
	}
	organizationhandler.RegisterRoutes(router, organizationhandler.New(authService, organizationService))
	handler := httpapi.WithRequestID(httpapi.Recover(router))

	return &App{
		server:   httpserver.New(cfg.Address, cfg.ShutdownTimeout, handler, logger),
		database: database,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.database.Close()
	return a.server.Run(ctx)
}
