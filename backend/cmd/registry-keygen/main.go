// Command registry-keygen creates or validates local-only Registry signing
// material. It never replaces an existing key.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"hubcr.io/hubcr/internal/modules/registry"
)

const (
	privateKeyName = "private.pem"
	publicJWKSName = "jwks.json"
)

func main() {
	outputDirectory := flag.String("output-dir", "", "absolute directory for local Registry signing material")
	flag.Parse()
	if *outputDirectory == "" || !filepath.IsAbs(*outputDirectory) {
		fail(errors.New("--output-dir must be an absolute path"))
	}
	if err := ensureDevelopmentKeys(*outputDirectory); err != nil {
		fail(err)
	}
	fmt.Printf("Registry development signing material is ready in %s\n", *outputDirectory)
}

func ensureDevelopmentKeys(directory string) error {
	privateKeyPath := filepath.Join(directory, privateKeyName)
	publicJWKSPath := filepath.Join(directory, publicJWKSName)
	privateExists, err := fileExists(privateKeyPath)
	if err != nil {
		return err
	}
	publicExists, err := fileExists(publicJWKSPath)
	if err != nil {
		return err
	}
	switch {
	case privateExists && publicExists:
		return validateExistingPair(privateKeyPath, publicJWKSPath)
	case privateExists || publicExists:
		return errors.New("Registry development signing material is incomplete; remove the local key directory and regenerate it")
	}

	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create Registry key directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure Registry key directory: %w", err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate Registry development key: %w", err)
	}
	signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
	if err != nil {
		return fmt.Errorf("initialize Registry development signer: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	defer clear(privatePEM)
	publicJWKS, err := json.MarshalIndent(
		registry.JWKSet{Keys: []registry.JWK{signer.PublicJWK()}}, "", "  ",
	)
	if err != nil {
		return fmt.Errorf("marshal Registry development JWKS: %w", err)
	}
	publicJWKS = append(publicJWKS, '\n')
	if err := writeExclusive(privateKeyPath, privatePEM, 0o600); err != nil {
		return err
	}
	if err := writeExclusive(publicJWKSPath, publicJWKS, 0o644); err != nil {
		return err
	}
	return nil
}

func validateExistingPair(privateKeyPath, publicJWKSPath string) error {
	privatePEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return errors.New("read Registry development private key")
	}
	defer clear(privatePEM)
	privateKey, err := registry.ParseRSAPrivateKeyPEM(privatePEM)
	if err != nil {
		return err
	}
	signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
	if err != nil {
		return err
	}
	publicJWKS, err := os.ReadFile(publicJWKSPath)
	if err != nil {
		return errors.New("read Registry development JWKS")
	}
	trusted, err := registry.ParseRS256JWKS(publicJWKS)
	if err != nil {
		return err
	}
	publicKey, exists := trusted[signer.KeyID()]
	if !exists || publicKey.E != privateKey.PublicKey.E ||
		publicKey.N.Cmp(privateKey.PublicKey.N) != 0 {
		return errors.New("Registry development private key is absent from the trusted JWKS")
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("inspect Registry development signing material: %w", err)
	}
}

func writeExclusive(path string, contents []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create Registry development signing material: %w", err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return fmt.Errorf("write Registry development signing material: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync Registry development signing material: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Registry development signing material: %w", err)
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
