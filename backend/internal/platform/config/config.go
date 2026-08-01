package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type API struct {
	Address         string
	ShutdownTimeout time.Duration
	Database        Database
	Authentication  Authentication
	Registry        Registry
}

type Database struct {
	URL                string
	ConnectTimeout     time.Duration
	HealthCheckTimeout time.Duration
	MaxConnections     int32
}

type Authentication struct {
	SessionTTL          time.Duration
	SessionCookieSecure bool
}

type Registry struct {
	Enabled           bool
	ExternalURL       string
	AllowInsecureHTTP bool
	Service           string
	Issuer            string
	TokenTTL          time.Duration
	ClockSkew         time.Duration
	PrivateKeyFile    string
	PublicJWKSFile    string
}

type Worker struct {
	PollInterval time.Duration
}

func LoadAPI() (API, error) {
	shutdownTimeout, err := duration("HUBCR_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return API{}, err
	}
	database, err := LoadDatabase()
	if err != nil {
		return API{}, err
	}
	authentication, err := loadAuthentication()
	if err != nil {
		return API{}, err
	}
	registry, err := loadRegistry()
	if err != nil {
		return API{}, err
	}

	return API{
		Address:         stringValue("HUBCR_API_ADDRESS", ":8080"),
		ShutdownTimeout: shutdownTimeout,
		Database:        database,
		Authentication:  authentication,
		Registry:        registry,
	}, nil
}

func loadRegistry() (Registry, error) {
	enabled, err := boolean("HUBCR_REGISTRY_AUTH_ENABLED", false)
	if err != nil {
		return Registry{}, err
	}
	allowInsecureHTTP, err := boolean("HUBCR_REGISTRY_ALLOW_INSECURE_HTTP", false)
	if err != nil {
		return Registry{}, err
	}
	tokenTTL, err := duration("HUBCR_REGISTRY_TOKEN_TTL", 5*time.Minute)
	if err != nil {
		return Registry{}, err
	}
	if tokenTTL < time.Minute || tokenTTL > 15*time.Minute || tokenTTL%time.Second != 0 {
		return Registry{}, errors.New("HUBCR_REGISTRY_TOKEN_TTL must be a whole-second duration from 1m through 15m")
	}
	registry := Registry{
		Enabled:           enabled,
		ExternalURL:       stringValue("HUBCR_REGISTRY_EXTERNAL_URL", ""),
		AllowInsecureHTTP: allowInsecureHTTP,
		Service:           stringValue("HUBCR_REGISTRY_SERVICE", "hubcr-registry"),
		Issuer:            stringValue("HUBCR_REGISTRY_ISSUER", "hubcr-token-service"),
		TokenTTL:          tokenTTL,
		ClockSkew:         30 * time.Second,
		PrivateKeyFile:    stringValue("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE", ""),
		PublicJWKSFile:    stringValue("HUBCR_REGISTRY_TOKEN_JWKS_FILE", ""),
	}
	if !validRegistryIdentifier(registry.Service) {
		return Registry{}, errors.New("HUBCR_REGISTRY_SERVICE must be a valid protocol identifier")
	}
	if !validRegistryIdentifier(registry.Issuer) {
		return Registry{}, errors.New("HUBCR_REGISTRY_ISSUER must be a valid protocol identifier")
	}
	if !enabled {
		return registry, nil
	}
	if err := validateRegistryExternalURL(registry.ExternalURL, allowInsecureHTTP); err != nil {
		return Registry{}, err
	}
	if registry.PrivateKeyFile == "" || !filepath.IsAbs(registry.PrivateKeyFile) {
		return Registry{}, errors.New("HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE must be an absolute path when Registry authentication is enabled")
	}
	if registry.PublicJWKSFile == "" || !filepath.IsAbs(registry.PublicJWKSFile) {
		return Registry{}, errors.New("HUBCR_REGISTRY_TOKEN_JWKS_FILE must be an absolute path when Registry authentication is enabled")
	}
	return registry, nil
}

func loadAuthentication() (Authentication, error) {
	sessionTTL, err := duration("HUBCR_SESSION_TTL", 24*time.Hour)
	if err != nil {
		return Authentication{}, err
	}
	secure, err := boolean("HUBCR_SESSION_COOKIE_SECURE", false)
	if err != nil {
		return Authentication{}, err
	}
	return Authentication{SessionTTL: sessionTTL, SessionCookieSecure: secure}, nil
}

func LoadDatabase() (Database, error) {
	databaseURL := stringValue(
		"HUBCR_DATABASE_URL",
		"postgres://hubcr:hubcr-dev-only@localhost:5432/hubcr?sslmode=disable",
	)
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Database{}, err
	}
	connectTimeout, err := duration("HUBCR_DATABASE_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Database{}, err
	}
	healthCheckTimeout, err := duration("HUBCR_DATABASE_HEALTH_TIMEOUT", 2*time.Second)
	if err != nil {
		return Database{}, err
	}
	maxConnections, err := positiveInt32("HUBCR_DATABASE_MAX_CONNECTIONS", 10)
	if err != nil {
		return Database{}, err
	}

	return Database{
		URL:                databaseURL,
		ConnectTimeout:     connectTimeout,
		HealthCheckTimeout: healthCheckTimeout,
		MaxConnections:     maxConnections,
	}, nil
}

func LoadWorker() (Worker, error) {
	pollInterval, err := duration("HUBCR_WORKER_POLL_INTERVAL", 5*time.Second)
	if err != nil {
		return Worker{}, err
	}

	return Worker{PollInterval: pollInterval}, nil
}

func stringValue(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}

func positiveInt32(key string, fallback int32) (int32, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return int32(parsed), nil
}

func boolean(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func validateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return errors.New("HUBCR_DATABASE_URL must be a valid PostgreSQL URL")
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return errors.New("HUBCR_DATABASE_URL must use the postgres or postgresql scheme")
	}
	if parsed.Hostname() == "" || parsed.User == nil || parsed.Path == "" || parsed.Path == "/" {
		return errors.New("HUBCR_DATABASE_URL must include user, host, and database name")
	}
	if parsed.Fragment != "" {
		return errors.New("HUBCR_DATABASE_URL must not include a fragment")
	}
	return nil
}

func validateRegistryExternalURL(value string, allowInsecureHTTP bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return errors.New("HUBCR_REGISTRY_EXTERNAL_URL must be an absolute origin without credentials, path, query, or fragment")
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecureHTTP {
			return nil
		}
		return errors.New("HUBCR_REGISTRY_EXTERNAL_URL must use HTTPS unless insecure local HTTP is explicitly enabled")
	default:
		return errors.New("HUBCR_REGISTRY_EXTERNAL_URL must use the http or https scheme")
	}
}

func validRegistryIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for index, character := range []byte(value) {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			(index > 0 && strings.ContainsRune("._:-", rune(character))) {
			continue
		}
		return false
	}
	return true
}
