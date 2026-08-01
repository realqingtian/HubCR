package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAPIDefaults(t *testing.T) {
	t.Setenv("HUBCR_API_ADDRESS", "")
	t.Setenv("HUBCR_SHUTDOWN_TIMEOUT", "")
	t.Setenv("HUBCR_DATABASE_URL", "")
	t.Setenv("HUBCR_DATABASE_CONNECT_TIMEOUT", "")
	t.Setenv("HUBCR_DATABASE_HEALTH_TIMEOUT", "")
	t.Setenv("HUBCR_DATABASE_MAX_CONNECTIONS", "")
	t.Setenv("HUBCR_SESSION_TTL", "")
	t.Setenv("HUBCR_SESSION_COOKIE_SECURE", "")

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
	if cfg.Database.URL != "postgres://hubcr:hubcr-dev-only@localhost:5432/hubcr?sslmode=disable" {
		t.Fatalf("Database.URL = %q, want local development default", cfg.Database.URL)
	}
	if cfg.Database.ConnectTimeout != 5*time.Second {
		t.Fatalf("Database.ConnectTimeout = %v, want %v", cfg.Database.ConnectTimeout, 5*time.Second)
	}
	if cfg.Database.HealthCheckTimeout != 2*time.Second {
		t.Fatalf("Database.HealthCheckTimeout = %v, want %v", cfg.Database.HealthCheckTimeout, 2*time.Second)
	}
	if cfg.Database.MaxConnections != 10 {
		t.Fatalf("Database.MaxConnections = %d, want 10", cfg.Database.MaxConnections)
	}
	if cfg.Authentication.SessionTTL != 24*time.Hour {
		t.Fatalf("Authentication.SessionTTL = %v, want 24h", cfg.Authentication.SessionTTL)
	}
	if cfg.Authentication.SessionCookieSecure {
		t.Fatal("Authentication.SessionCookieSecure = true, want false for local HTTP default")
	}
}

func TestLoadAPIRejectsInvalidAuthenticationSettings(t *testing.T) {
	t.Setenv("HUBCR_SESSION_COOKIE_SECURE", "sometimes")
	if _, err := LoadAPI(); err == nil {
		t.Fatal("LoadAPI() error = nil, want invalid boolean error")
	}
}

func TestLoadAPIRejectsInvalidDatabaseSettingsWithoutLeakingCredentials(t *testing.T) {
	const secret = "do-not-log-this-password"
	t.Setenv("HUBCR_DATABASE_URL", "postgres://hubcr:"+secret+"@%")

	_, err := LoadAPI()
	if err == nil {
		t.Fatal("LoadAPI() error = nil, want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("LoadAPI() error leaked database password: %v", err)
	}
}

func TestLoadAPIRejectsNonPositiveDatabaseConnectionLimit(t *testing.T) {
	t.Setenv("HUBCR_DATABASE_MAX_CONNECTIONS", "0")

	if _, err := LoadAPI(); err == nil {
		t.Fatal("LoadAPI() error = nil, want an error")
	}
}

func TestLoadWorkerRejectsInvalidInterval(t *testing.T) {
	t.Setenv("HUBCR_WORKER_POLL_INTERVAL", "not-a-duration")

	if _, err := LoadWorker(); err == nil {
		t.Fatal("LoadWorker() error = nil, want an error")
	}
}
