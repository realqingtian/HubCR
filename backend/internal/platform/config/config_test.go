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

func TestLoadWorkerDefaults(t *testing.T) {
	for _, key := range []string{
		"HUBCR_DATABASE_URL", "HUBCR_DATABASE_CONNECT_TIMEOUT", "HUBCR_DATABASE_HEALTH_TIMEOUT",
		"HUBCR_DATABASE_MAX_CONNECTIONS", "HUBCR_WORKER_POLL_INTERVAL",
		"HUBCR_WORKER_LEASE_DURATION", "HUBCR_WORKER_JOB_TIMEOUT",
		"HUBCR_WORKER_SHUTDOWN_TIMEOUT", "HUBCR_WORKER_RETRY_BASE",
		"HUBCR_WORKER_RETRY_MAX", "HUBCR_WORKER_MAX_CONCURRENCY",
		"HUBCR_SECURITY_SCANNER_ENABLED", "HUBCR_TRIVY_BINARY", "HUBCR_TRIVY_CACHE_DIR",
		"HUBCR_COSIGN_BINARY", "HUBCR_COSIGN_SCRATCH_DIR",
		"HUBCR_SCANNER_REGISTRY_HOST", "HUBCR_SCANNER_REGISTRY_INSECURE",
		"HUBCR_SCANNER_REGISTRY_TOKEN_TTL", "HUBCR_REGISTRY_SERVICE", "HUBCR_REGISTRY_ISSUER",
		"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", "HUBCR_SECURITY_REPAIR_INTERVAL",
		"HUBCR_SECURITY_REPAIR_BATCH",
	} {
		t.Setenv(key, "")
	}
	cfg, err := LoadWorker()
	if err != nil {
		t.Fatalf("LoadWorker() error = %v", err)
	}
	if cfg.PollInterval != 5*time.Second || cfg.LeaseDuration != 15*time.Minute ||
		cfg.JobTimeout != 10*time.Minute || cfg.ShutdownTimeout != 20*time.Second ||
		cfg.RetryBase != 5*time.Second || cfg.RetryMax != 5*time.Minute ||
		cfg.MaxConcurrency != 2 || cfg.Database.MaxConnections != 10 || cfg.Scanner.Enabled ||
		cfg.Scanner.Binary != "trivy" || cfg.Scanner.CacheDir != "/tmp/hubcr-trivy" ||
		cfg.Scanner.CosignBinary != "cosign" || cfg.Scanner.CosignScratchDir != "/tmp/hubcr-cosign" ||
		cfg.Scanner.RepairInterval != 30*time.Second || cfg.Scanner.RepairBatch != 100 {
		t.Fatalf("Worker defaults = %#v", cfg)
	}
}

func TestLoadWorkerSecurityScannerConfiguration(t *testing.T) {
	t.Setenv("HUBCR_SECURITY_SCANNER_ENABLED", "true")
	t.Setenv("HUBCR_TRIVY_BINARY", "/usr/local/bin/trivy")
	t.Setenv("HUBCR_TRIVY_CACHE_DIR", "/var/lib/hubcr/trivy")
	t.Setenv("HUBCR_COSIGN_BINARY", "/usr/local/bin/cosign")
	t.Setenv("HUBCR_COSIGN_SCRATCH_DIR", "/var/lib/hubcr/cosign")
	t.Setenv("HUBCR_SCANNER_REGISTRY_HOST", "registry:5000")
	t.Setenv("HUBCR_SCANNER_REGISTRY_INSECURE", "true")
	t.Setenv("HUBCR_SCANNER_REGISTRY_TOKEN_TTL", "3m")
	t.Setenv("HUBCR_REGISTRY_SERVICE", "hubcr-registry")
	t.Setenv("HUBCR_REGISTRY_ISSUER", "hubcr-token-service")
	t.Setenv("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", "/run/hubcr/registry-auth/private.pem")
	t.Setenv("HUBCR_SECURITY_REPAIR_INTERVAL", "15s")
	t.Setenv("HUBCR_SECURITY_REPAIR_BATCH", "50")
	cfg, err := LoadWorker()
	if err != nil {
		t.Fatalf("LoadWorker() error = %v", err)
	}
	if !cfg.Scanner.Enabled || cfg.Scanner.RegistryHost != "registry:5000" ||
		!cfg.Scanner.RegistryInsecure || cfg.Scanner.RegistryTokenTTL != 3*time.Minute ||
		cfg.Scanner.CosignBinary != "/usr/local/bin/cosign" ||
		cfg.Scanner.CosignScratchDir != "/var/lib/hubcr/cosign" ||
		cfg.Scanner.RegistryPrivateKey != "/run/hubcr/registry-auth/private.pem" ||
		cfg.Scanner.RepairInterval != 15*time.Second || cfg.Scanner.RepairBatch != 50 {
		t.Fatalf("Scanner = %#v", cfg.Scanner)
	}
}

func TestLoadWorkerRejectsUnsafeExecutionBounds(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
	}{
		{
			name: "lease does not outlive job timeout",
			env: map[string]string{
				"HUBCR_WORKER_LEASE_DURATION": "1m", "HUBCR_WORKER_JOB_TIMEOUT": "1m",
			},
		},
		{
			name: "retry cap below base",
			env: map[string]string{
				"HUBCR_WORKER_RETRY_BASE": "1m", "HUBCR_WORKER_RETRY_MAX": "30s",
			},
		},
		{
			name: "excessive concurrency",
			env: map[string]string{
				"HUBCR_WORKER_MAX_CONCURRENCY": "65",
			},
		},
		{
			name: "scanner relative key",
			env: map[string]string{
				"HUBCR_SECURITY_SCANNER_ENABLED":        "true",
				"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE": "private.pem",
			},
		},
		{
			name: "scanner host has scheme",
			env: map[string]string{
				"HUBCR_SECURITY_SCANNER_ENABLED":        "true",
				"HUBCR_SCANNER_REGISTRY_HOST":           "http://registry:5000",
				"HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE": "/tmp/private.pem",
			},
		},
		{
			name: "repair batch too large",
			env:  map[string]string{"HUBCR_SECURITY_REPAIR_BATCH": "501"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			if _, err := LoadWorker(); err == nil {
				t.Fatal("LoadWorker() error = nil")
			}
		})
	}
}
