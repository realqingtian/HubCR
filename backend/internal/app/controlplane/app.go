package controlplane

import (
	"context"
	"log/slog"
	"net/http"

	"hubcr.io/hubcr/internal/modules/health"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/httpserver"
)

// App composes the control-plane modules and their infrastructure adapters.
type App struct {
	server *httpserver.Server
}

func New(cfg config.API, logger *slog.Logger) *App {
	mux := http.NewServeMux()
	health.RegisterRoutes(mux)

	return &App{
		server: httpserver.New(cfg.Address, cfg.ShutdownTimeout, mux, logger),
	}
}

func (a *App) Run(ctx context.Context) error {
	return a.server.Run(ctx)
}
