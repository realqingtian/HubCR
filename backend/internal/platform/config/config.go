package config

import (
	"fmt"
	"os"
	"time"
)

type API struct {
	Address         string
	ShutdownTimeout time.Duration
}

type Worker struct {
	PollInterval time.Duration
}

func LoadAPI() (API, error) {
	shutdownTimeout, err := duration("HUBCR_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return API{}, err
	}

	return API{
		Address:         stringValue("HUBCR_API_ADDRESS", ":8080"),
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

func LoadWorker() (Worker, error) {
	pollInterval, err := duration("HUBCR_WORKER_POLL_INTERVAL", 5*time.Second)
	if err != nil {
		return Worker{}, err
	}

	return Worker{PollInterval: pollInterval}, nil
}

func stringValue(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}
