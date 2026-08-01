// Command m2token creates a deliberately expired Registry token for the M2
// full-stack test. It is not a product token-issuance entry point.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"hubcr.io/hubcr/internal/modules/registry"
)

const repositoryName = "m2-e2e-team/private-image"

func main() {
	if len(os.Args) != 2 || os.Args[1] == "" {
		fail("usage: m2token PRIVATE_KEY_FILE")
	}
	privateKeyPEM, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail("read Registry private key: %v", err)
	}
	privateKey, err := registry.ParseRSAPrivateKeyPEM(privateKeyPEM)
	if err != nil {
		fail("parse Registry private key: %v", err)
	}
	signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
	if err != nil {
		fail("initialize Registry signer: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	token, err := signer.Sign(context.Background(), registry.Claims{
		Issuer: "hubcr-token-service", Subject: "m2-expired-token-subject",
		Audience: "hubcr-registry", IssuedAt: now.Add(-10 * time.Minute).Unix(),
		NotBefore: now.Add(-10 * time.Minute).Unix(),
		ExpiresAt: now.Add(-5 * time.Minute).Unix(), ID: "m2-expired-token",
		Access: []registry.Access{{
			Type: registry.ResourceRepository, Name: repositoryName,
			Actions: []registry.Action{registry.ActionPull},
		}},
	})
	if err != nil {
		fail("sign expired Registry token: %v", err)
	}
	fmt.Println(token)
}

func fail(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
