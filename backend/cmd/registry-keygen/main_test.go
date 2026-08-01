package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDevelopmentKeysCreatesAndReusesSecureMatchingPair(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "registry-auth")
	if err := ensureDevelopmentKeys(directory); err != nil {
		t.Fatalf("ensureDevelopmentKeys(create) error = %v", err)
	}
	privatePath := filepath.Join(directory, privateKeyName)
	jwksPath := filepath.Join(directory, publicJWKSName)
	privateInfo, err := os.Stat(privatePath)
	if err != nil {
		t.Fatalf("os.Stat(private) error = %v", err)
	}
	if privateInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %o, want 600", privateInfo.Mode().Perm())
	}
	if _, err := os.Stat(jwksPath); err != nil {
		t.Fatalf("os.Stat(JWKS) error = %v", err)
	}
	before, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("os.ReadFile(private) error = %v", err)
	}
	if err := ensureDevelopmentKeys(directory); err != nil {
		t.Fatalf("ensureDevelopmentKeys(reuse) error = %v", err)
	}
	after, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("os.ReadFile(private after reuse) error = %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("ensureDevelopmentKeys replaced an existing private key")
	}
}

func TestEnsureDevelopmentKeysRejectsIncompletePair(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, privateKeyName), []byte("incomplete"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := ensureDevelopmentKeys(directory); err == nil {
		t.Fatal("ensureDevelopmentKeys(incomplete) error = nil")
	}
}
