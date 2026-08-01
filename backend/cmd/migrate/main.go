package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseConfig, err := config.LoadDatabase()
	if err != nil {
		logger.Error("load database configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL:            databaseConfig.URL,
		ConnectTimeout: databaseConfig.ConnectTimeout,
		MaxConnections: databaseConfig.MaxConnections,
	})
	if err != nil {
		logger.Error("initialize PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Check(ctx); err != nil {
		logger.Error("connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		logger.Error("apply migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations applied")
}
