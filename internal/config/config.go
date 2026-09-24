// Package config loads openforms runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr          string // OPENFORMS_HTTP_ADDR, default ":8080"
	DatabaseURL       string // OPENFORMS_DATABASE_URL, required for serve/migrate/seed/admin
	BaseURL           string // OPENFORMS_BASE_URL, default "http://localhost:8080"
	SMTPHost          string // OPENFORMS_SMTP_HOST; empty → log mailer
	SMTPPort          int    // OPENFORMS_SMTP_PORT, default 1025
	SMTPUsername      string // OPENFORMS_SMTP_USERNAME
	SMTPPassword      string // OPENFORMS_SMTP_PASSWORD
	SMTPFrom          string // OPENFORMS_SMTP_FROM, default "openforms@localhost"
	WebhookSecret     string // OPENFORMS_WEBHOOK_SECRET; empty → no signature header
	WorkerConcurrency int    // OPENFORMS_WORKER_CONCURRENCY, default 4
	DemoMode          bool   // OPENFORMS_DEMO ("true"/"1")
	CookieSecure      bool   // OPENFORMS_COOKIE_SECURE, default: BaseURL starts with https
}

// Load reads configuration from the environment. Empty variables count as unset.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:      getenv("OPENFORMS_HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("OPENFORMS_DATABASE_URL"),
		BaseURL:       strings.TrimRight(getenv("OPENFORMS_BASE_URL", "http://localhost:8080"), "/"),
		SMTPHost:      os.Getenv("OPENFORMS_SMTP_HOST"),
		SMTPUsername:  os.Getenv("OPENFORMS_SMTP_USERNAME"),
		SMTPPassword:  os.Getenv("OPENFORMS_SMTP_PASSWORD"),
		SMTPFrom:      getenv("OPENFORMS_SMTP_FROM", "openforms@localhost"),
		WebhookSecret: os.Getenv("OPENFORMS_WEBHOOK_SECRET"),
	}
	var err error
	if c.SMTPPort, err = intEnv("OPENFORMS_SMTP_PORT", 1025, 1); err != nil {
		return Config{}, err
	}
	if c.WorkerConcurrency, err = intEnv("OPENFORMS_WORKER_CONCURRENCY", 4, 1); err != nil {
		return Config{}, err
	}
	if c.DemoMode, err = boolEnv("OPENFORMS_DEMO", false); err != nil {
		return Config{}, err
	}
	if c.CookieSecure, err = boolEnv("OPENFORMS_COOKIE_SECURE", strings.HasPrefix(c.BaseURL, "https://")); err != nil {
		return Config{}, err
	}
	return c, nil
}

// RequireDatabase reports an error when no database URL is configured.
func (c Config) RequireDatabase() error {
	if c.DatabaseURL == "" {
		return errors.New("OPENFORMS_DATABASE_URL is required")
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def, min int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return 0, fmt.Errorf("%s must be an integer >= %d, got %q", key, min, v)
	}
	return n, nil
}

func boolEnv(key string, def bool) (bool, error) {
	switch strings.ToLower(os.Getenv(key)) {
	case "":
		return def, nil
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true/false/1/0, got %q", key, os.Getenv(key))
	}
}
