package worker

import (
	"context"
	"log/slog"
	"time"

	"hubcr.io/hubcr/internal/platform/config"
)

// App hosts asynchronous jobs such as scanning and signature verification.
type App struct {
	config config.Worker
	logger *slog.Logger
}

func New(cfg config.Worker, logger *slog.Logger) *App {
	return &App{config: cfg, logger: logger}
}

func (a *App) Run(ctx context.Context) error {
	ticker := time.NewTicker(a.config.PollInterval)
	defer ticker.Stop()

	a.logger.Info("worker started", "poll_interval", a.config.PollInterval)
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("worker stopped")
			return nil
		case <-ticker.C:
			// PostgreSQL-backed jobs will be claimed here once persistence is added.
			a.logger.Debug("polling for jobs")
		}
	}
}
