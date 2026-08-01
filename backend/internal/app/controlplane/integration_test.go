package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/platform/config"
)

func TestControlPlaneComposesEnabledRegistryTokenService(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "registry-private-key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	clear(keyPEM)
	signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
	if err != nil {
		t.Fatalf("registry.NewRS256Signer() error = %v", err)
	}
	jwks, err := json.Marshal(registry.JWKSet{Keys: []registry.JWK{signer.PublicJWK()}})
	if err != nil {
		t.Fatalf("json.Marshal(JWKS) error = %v", err)
	}
	jwksPath := filepath.Join(t.TempDir(), "registry-jwks.json")
	if err := os.WriteFile(jwksPath, jwks, 0o644); err != nil {
		t.Fatalf("os.WriteFile(JWKS) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app, err := New(ctx, config.API{
		Address: ":0", ShutdownTimeout: time.Second,
		Database: config.Database{
			URL: databaseURL, ConnectTimeout: 3 * time.Second,
			HealthCheckTimeout: time.Second, MaxConnections: 2,
		},
		Authentication: config.Authentication{SessionTTL: time.Hour},
		Registry: config.Registry{
			Enabled: true, ExternalURL: "https://registry.example",
			Service: "hubcr-registry", Issuer: "hubcr-token-service",
			TokenTTL: 5 * time.Minute, ClockSkew: 30 * time.Second,
			PrivateKeyFile: keyPath, PublicJWKSFile: jwksPath,
			EventToken: "0123456789abcdef0123456789abcdef",
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	app.database.Close()
}
