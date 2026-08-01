package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type API struct {
	Address         string
	ShutdownTimeout time.Duration
	Database        Database
	Authentication  Authentication
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

	return API{
		Address:         stringValue("HUBCR_API_ADDRESS", ":8080"),
		ShutdownTimeout: shutdownTimeout,
		Database:        database,
		Authentication:  authentication,
	}, nil
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
