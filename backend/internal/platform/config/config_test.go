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
	t.Setenv("HUBCR_REGISTRY_AUTH_ENABLED", "")
	t.Setenv("HUBCR_REGISTRY_ALLOW_INSECURE_HTTP", "")
	t.Setenv("HUBCR_REGISTRY_EXTERNAL_URL", "")
	t.Setenv("HUBCR_REGISTRY_SERVICE", "")
	t.Setenv("HUBCR_REGISTRY_ISSUER", "")
	t.Setenv("HUBCR_REGISTRY_TOKEN_TTL", "")
	t.Setenv("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", "")
	t.Setenv("HUBCR_REGISTRY_TOKEN_JWKS_FILE", "")
	t.Setenv("HUBCR_REGISTRY_EVENT_TOKEN", "")

	cfg, err := LoadAPI()
	if err != nil {
		t.Fatalf("LoadAPI() error = %v", err)
	}
	if cfg.Address != "127.0.0.1:8080" {
		t.Fatalf("Address = %q, want %q", cfg.Address, "127.0.0.1:8080")
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
	if cfg.Registry.Enabled || cfg.Registry.Service != "hubcr-registry" ||
		cfg.Registry.Issuer != "hubcr-token-service" ||
		cfg.Registry.TokenTTL != 5*time.Minute ||
		cfg.Registry.ClockSkew != 30*time.Second {
		t.Fatalf("Registry defaults = %#v", cfg.Registry)
	}
}

func TestLoadAPIRejectsInvalidAuthenticationSettings(t *testing.T) {
	t.Setenv("HUBCR_SESSION_COOKIE_SECURE", "sometimes")
	if _, err := LoadAPI(); err == nil {
		t.Fatal("LoadAPI() error = nil, want invalid boolean error")
	}
}

func TestLoadAPIRegistryAuthenticationConfiguration(t *testing.T) {
	t.Setenv("HUBCR_REGISTRY_AUTH_ENABLED", "true")
	t.Setenv("HUBCR_REGISTRY_EXTERNAL_URL", "https://registry.example")
	t.Setenv("HUBCR_REGISTRY_SERVICE", "registry.example")
	t.Setenv("HUBCR_REGISTRY_ISSUER", "auth.registry.example")
	t.Setenv("HUBCR_REGISTRY_TOKEN_TTL", "10m")
	t.Setenv("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", "/run/secrets/hubcr-registry-key.pem")
	t.Setenv("HUBCR_REGISTRY_TOKEN_JWKS_FILE", "/run/secrets/hubcr-registry-jwks.json")
	t.Setenv("HUBCR_REGISTRY_EVENT_TOKEN", "0123456789abcdef0123456789abcdef")

	cfg, err := LoadAPI()
	if err != nil {
		t.Fatalf("LoadAPI() error = %v", err)
	}
	if !cfg.Registry.Enabled ||
		cfg.Registry.ExternalURL != "https://registry.example" ||
		cfg.Registry.AllowInsecureHTTP ||
		cfg.Registry.Service != "registry.example" ||
		cfg.Registry.Issuer != "auth.registry.example" ||
		cfg.Registry.TokenTTL != 10*time.Minute ||
		cfg.Registry.PrivateKeyFile != "/run/secrets/hubcr-registry-key.pem" ||
		cfg.Registry.PublicJWKSFile != "/run/secrets/hubcr-registry-jwks.json" ||
		cfg.Registry.EventToken != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("Registry = %#v", cfg.Registry)
	}
}

func TestLoadAPIRejectsInvalidRegistryConfiguration(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "HTTP without explicit local opt-in",
			env: map[string]string{
				"HUBCR_REGISTRY_AUTH_ENABLED":           "true",
				"HUBCR_REGISTRY_EXTERNAL_URL":           "http://localhost:5000",
				"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE": "/tmp/key.pem",
				"HUBCR_REGISTRY_TOKEN_JWKS_FILE":        "/tmp/jwks.json",
			},
		},
		{
			name: "origin with path",
			env: map[string]string{
				"HUBCR_REGISTRY_AUTH_ENABLED":           "true",
				"HUBCR_REGISTRY_EXTERNAL_URL":           "https://registry.example/token",
				"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE": "/tmp/key.pem",
				"HUBCR_REGISTRY_TOKEN_JWKS_FILE":        "/tmp/jwks.json",
			},
		},
		{
			name: "relative private key",
			env: map[string]string{
				"HUBCR_REGISTRY_AUTH_ENABLED":           "true",
				"HUBCR_REGISTRY_EXTERNAL_URL":           "https://registry.example",
				"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE": "key.pem",
				"HUBCR_REGISTRY_TOKEN_JWKS_FILE":        "/tmp/jwks.json",
			},
		},
		{
			name: "relative JWKS",
			env: map[string]string{
				"HUBCR_REGISTRY_AUTH_ENABLED":           "true",
				"HUBCR_REGISTRY_EXTERNAL_URL":           "https://registry.example",
				"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE": "/tmp/key.pem",
				"HUBCR_REGISTRY_TOKEN_JWKS_FILE":        "jwks.json",
			},
		},
		{
			name: "short TTL",
			env: map[string]string{
				"HUBCR_REGISTRY_TOKEN_TTL": "59s",
			},
		},
		{
			name: "oversized TTL",
			env: map[string]string{
				"HUBCR_REGISTRY_TOKEN_TTL": "16m",
			},
		},
		{
			name: "unsafe service",
			env: map[string]string{
				"HUBCR_REGISTRY_SERVICE": `registry"challenge`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HUBCR_REGISTRY_EVENT_TOKEN", "0123456789abcdef0123456789abcdef")
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			if _, err := LoadAPI(); err == nil {
				t.Fatal("LoadAPI() error = nil")
			}
		})
	}
	t.Run("short event token", func(t *testing.T) {
		t.Setenv("HUBCR_REGISTRY_AUTH_ENABLED", "true")
		t.Setenv("HUBCR_REGISTRY_EXTERNAL_URL", "https://registry.example")
		t.Setenv("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", "/tmp/key.pem")
		t.Setenv("HUBCR_REGISTRY_TOKEN_JWKS_FILE", "/tmp/jwks.json")
		t.Setenv("HUBCR_REGISTRY_EVENT_TOKEN", "short")
		if _, err := LoadAPI(); err == nil {
			t.Fatal("LoadAPI() error = nil")
		}
	})
}

func TestLoadAPIAllowsExplicitLocalRegistryHTTP(t *testing.T) {
	t.Setenv("HUBCR_REGISTRY_AUTH_ENABLED", "true")
	t.Setenv("HUBCR_REGISTRY_ALLOW_INSECURE_HTTP", "true")
	t.Setenv("HUBCR_REGISTRY_EXTERNAL_URL", "http://localhost:5000")
	t.Setenv("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", "/tmp/key.pem")
	t.Setenv("HUBCR_REGISTRY_TOKEN_JWKS_FILE", "/tmp/jwks.json")
	t.Setenv("HUBCR_REGISTRY_EVENT_TOKEN", "0123456789abcdef0123456789abcdef")
	if _, err := LoadAPI(); err != nil {
		t.Fatalf("LoadAPI() error = %v", err)
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
