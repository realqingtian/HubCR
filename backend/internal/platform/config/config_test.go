package config

import (
	"testing"
	"time"
)

func TestLoadAPIDefaults(t *testing.T) {
	t.Setenv("HUBCR_API_ADDRESS", "")
	t.Setenv("HUBCR_SHUTDOWN_TIMEOUT", "")

	cfg, err := LoadAPI()
	if err != nil {
		t.Fatalf("LoadAPI() error = %v", err)
	}
	if cfg.Address != ":8080" {
		t.Fatalf("Address = %q, want %q", cfg.Address, ":8080")
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 10*time.Second)
	}
}

func TestLoadWorkerRejectsInvalidInterval(t *testing.T) {
	t.Setenv("HUBCR_WORKER_POLL_INTERVAL", "not-a-duration")

	if _, err := LoadWorker(); err == nil {
		t.Fatal("LoadWorker() error = nil, want an error")
	}
}
