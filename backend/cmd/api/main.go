package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hubcr.io/hubcr/internal/app/controlplane"
	"hubcr.io/hubcr/internal/platform/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadAPI()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := controlplane.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("initialize control plane", "error", err)
		os.Exit(1)
	}
	if err := app.Run(ctx); err != nil {
		logger.Error("control plane stopped", "error", err)
		os.Exit(1)
	}
}
