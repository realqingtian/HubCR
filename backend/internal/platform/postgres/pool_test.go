package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsInvalidURLWithoutLeakingCredentials(t *testing.T) {
	const secret = "do-not-log-this-password"

	_, err := Open(context.Background(), Options{
		URL:            "postgres://hubcr:" + secret + "@%",
		ConnectTimeout: time.Second,
		MaxConnections: 1,
	})
	if err == nil {
		t.Fatal("Open() error = nil, want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Open() error leaked database password: %v", err)
	}
}

func TestPoolConnectivity(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := Open(ctx, Options{
		URL:            databaseURL,
		ConnectTimeout: 3 * time.Second,
		MaxConnections: 2,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Check(ctx); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}
