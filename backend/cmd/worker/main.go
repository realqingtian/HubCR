package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	workerapp "hubcr.io/hubcr/internal/app/worker"
	"hubcr.io/hubcr/internal/platform/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadWorker()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := workerapp.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
