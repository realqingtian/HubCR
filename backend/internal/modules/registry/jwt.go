package registry

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
)

const (
	rs256Algorithm     = "RS256"
	minimumRSAKeyBits  = 2048
	maximumTokenBytes  = 32 * 1024
	maximumJWKSBytes   = 64 * 1024
	maximumTrustedKeys = 16
)

type TokenSigner interface {
	KeyID() string
	Sign(context.Context, Claims) (string, error)
}

type RS256Signer struct {
	privateKey *rsa.PrivateKey
	keyID      string
	random     io.Reader
}

type JWTHeader struct {
	Type      string `json:"typ"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

type JWK struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

func ParseRSAPrivateKeyPEM(value []byte) (*rsa.PrivateKey, error) {
	block, rest := pem.Decode(value)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("parse Registry private key: expected one PEM block")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("parse Registry PKCS#1 private key")
		}
		key = parsed
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("parse Registry PKCS#8 private key")
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("parse Registry private key: key is not RSA")
		}
	default:
		return nil, errors.New("parse Registry private key: unsupported PEM type")
	}
	if err := validatePrivateKey(key); err != nil {
		return nil, err
	}
	return key, nil
}

func NewRS256Signer(privateKey *rsa.PrivateKey, random io.Reader) (*RS256Signer, error) {
	if err := validatePrivateKey(privateKey); err != nil {
		return nil, err
	}
	if random == nil {
		return nil, errors.New("Registry signer random source must be configured")
	}
	return &RS256Signer{
		privateKey: privateKey,
		keyID:      RSAKeyID(&privateKey.PublicKey),
		random:     random,
	}, nil
}

func (s *RS256Signer) KeyID() string { return s.keyID }

func (s *RS256Signer) PublicJWK() JWK {
	exponent := big.NewInt(int64(s.privateKey.PublicKey.E)).Bytes()
	return JWK{
		KeyType: "RSA", KeyID: s.keyID, Use: "sig", Algorithm: rs256Algorithm,
		Modulus:  base64.RawURLEncoding.EncodeToString(s.privateKey.PublicKey.N.Bytes()),
		Exponent: base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func ParseRS256JWKS(value []byte) (map[string]*rsa.PublicKey, error) {
	if len(value) == 0 || len(value) > maximumJWKSBytes {
		return nil, errors.New("parse Registry JWKS: invalid size")
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	var set JWKSet
	if err := decoder.Decode(&set); err != nil {
		return nil, errors.New("parse Registry JWKS")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("parse Registry JWKS: trailing data")
	}
	if len(set.Keys) == 0 || len(set.Keys) > maximumTrustedKeys {
		return nil, errors.New("parse Registry JWKS: invalid key count")
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, jwk := range set.Keys {
		if jwk.KeyType != "RSA" || jwk.Algorithm != rs256Algorithm ||
			(jwk.Use != "" && jwk.Use != "sig") || jwk.KeyID == "" {
			return nil, errors.New("parse Registry JWKS: unsupported key")
		}
		modulus, err := base64.RawURLEncoding.DecodeString(jwk.Modulus)
		if err != nil || len(modulus) == 0 {
			return nil, errors.New("parse Registry JWKS: invalid modulus")
		}
		exponent, err := base64.RawURLEncoding.DecodeString(jwk.Exponent)
		if err != nil || len(exponent) == 0 || len(exponent) > 4 {
			return nil, errors.New("parse Registry JWKS: invalid exponent")
		}
		exponentValue := new(big.Int).SetBytes(exponent)
		if !exponentValue.IsInt64() || exponentValue.Int64() < 3 ||
			exponentValue.Int64()%2 == 0 ||
			exponentValue.Int64() > int64(^uint(0)>>1) {
			return nil, errors.New("parse Registry JWKS: invalid exponent")
		}
		publicKey := &rsa.PublicKey{
			N: new(big.Int).SetBytes(modulus),
			E: int(exponentValue.Int64()),
		}
		if publicKey.N.BitLen() < minimumRSAKeyBits ||
			RSAKeyID(publicKey) != jwk.KeyID {
			return nil, errors.New("parse Registry JWKS: invalid key ID or key size")
		}
		if _, exists := keys[jwk.KeyID]; exists {
			return nil, errors.New("parse Registry JWKS: duplicate key ID")
		}
		keys[jwk.KeyID] = publicKey
	}
	return keys, nil
}

func (s *RS256Signer) Sign(ctx context.Context, claims Claims) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateClaimsShape(claims); err != nil {
		return "", err
	}
	header, err := json.Marshal(JWTHeader{
		Type: "JWT", Algorithm: rs256Algorithm, KeyID: s.keyID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal Registry JWT header: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal Registry JWT claims: %w", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(s.random, s.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign Registry JWT: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func RSAKeyID(publicKey *rsa.PublicKey) string {
	if publicKey == nil || publicKey.N == nil || publicKey.E < 3 || publicKey.E%2 == 0 {
		return ""
	}
	thumbprintInput := struct {
		Exponent string `json:"e"`
		KeyType  string `json:"kty"`
		Modulus  string `json:"n"`
	}{
		Exponent: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
		KeyType:  "RSA",
		Modulus:  base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
	}
	encoded, err := json.Marshal(thumbprintInput)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

type RS256Verifier struct {
	keys     map[string]*rsa.PublicKey
	issuer   string
	audience string
	clock    func() time.Time
	skew     time.Duration
}

func NewRS256Verifier(
	keys map[string]*rsa.PublicKey,
	issuer, audience string,
	clock func() time.Time,
	skew time.Duration,
) (*RS256Verifier, error) {
	if len(keys) == 0 || issuer == "" || audience == "" || clock == nil ||
		skew < 0 || skew > time.Minute {
		return nil, errors.New("Registry verifier dependencies must be configured")
	}
	trusted := make(map[string]*rsa.PublicKey, len(keys))
	for keyID, publicKey := range keys {
		if keyID == "" || publicKey == nil || publicKey.N == nil ||
			publicKey.N.BitLen() < minimumRSAKeyBits || publicKey.E < 3 ||
			publicKey.E%2 == 0 ||
			RSAKeyID(publicKey) != keyID {
			return nil, errors.New("Registry verifier contains an invalid trusted key")
		}
		trusted[keyID] = publicKey
	}
	return &RS256Verifier{
		keys: trusted, issuer: issuer, audience: audience, clock: clock, skew: skew,
	}, nil
}

func (v *RS256Verifier) Verify(token string) (Claims, error) {
	if token == "" || len(token) > maximumTokenBytes {
		return Claims{}, ErrInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header JWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil ||
		header.Type != "JWT" || header.Algorithm != rs256Algorithm || header.KeyID == "" {
		return Claims{}, ErrInvalidToken
	}
	publicKey, exists := v.keys[header.KeyID]
	if !exists {
		return Claims{}, ErrInvalidToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return Claims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if err := v.validateClaims(claims); err != nil {
		return Claims{}, err
	}
	return claims, nil
}

func (v *RS256Verifier) validateClaims(claims Claims) error {
	if validateClaimsShape(claims) != nil ||
		claims.Issuer != v.issuer ||
		claims.Audience != v.audience {
		return ErrInvalidToken
	}
	now := v.clock().UTC()
	if !now.Before(time.Unix(claims.ExpiresAt, 0)) ||
		now.Add(v.skew).Before(time.Unix(claims.NotBefore, 0)) ||
		now.Add(v.skew).Before(time.Unix(claims.IssuedAt, 0)) {
		return ErrInvalidToken
	}
	return nil
}

func validatePrivateKey(key *rsa.PrivateKey) error {
	if key == nil || key.N == nil || key.N.BitLen() < minimumRSAKeyBits || key.E < 3 {
		return errors.New("Registry RSA private key must be at least 2048 bits")
	}
	if err := key.Validate(); err != nil {
		return errors.New("Registry RSA private key is invalid")
	}
	return nil
}

func validateClaimsShape(claims Claims) error {
	if claims.Issuer == "" || claims.Audience == "" || claims.ID == "" ||
		claims.ExpiresAt <= claims.IssuedAt || claims.NotBefore > claims.IssuedAt ||
		claims.Access == nil {
		return ErrInvalidToken
	}
	seenNames := make(map[string]struct{}, len(claims.Access))
	for _, access := range claims.Access {
		if access.Type != ResourceRepository || access.Name == "" || access.Actions == nil {
			return ErrInvalidToken
		}
		if _, exists := seenNames[access.Name]; exists {
			return ErrInvalidToken
		}
		seenNames[access.Name] = struct{}{}
		if len(access.Actions) == 0 {
			scope, err := ParseScope(access.Type + ":" + access.Name + ":pull")
			if err != nil || scope.Name != access.Name {
				return ErrInvalidToken
			}
			continue
		}
		scope, err := ParseScope(access.Type + ":" + access.Name + ":" + actionsString(access.Actions))
		if err != nil || scope.Name != access.Name || !equalActions(scope.Actions, access.Actions) {
			return ErrInvalidToken
		}
	}
	return nil
}

func actionsString(actions []Action) string {
	values := make([]string, len(actions))
	for index, action := range actions {
		values[index] = string(action)
	}
	return strings.Join(values, ",")
}

func equalActions(left, right []Action) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
