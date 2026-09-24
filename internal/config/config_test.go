package config_test

import (
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/config"
)

var allKeys = []string{
	"OPENFORMS_HTTP_ADDR", "OPENFORMS_DATABASE_URL", "OPENFORMS_BASE_URL",
	"OPENFORMS_SMTP_HOST", "OPENFORMS_SMTP_PORT", "OPENFORMS_SMTP_USERNAME",
	"OPENFORMS_SMTP_PASSWORD", "OPENFORMS_SMTP_FROM", "OPENFORMS_WEBHOOK_SECRET",
	"OPENFORMS_WORKER_CONCURRENCY", "OPENFORMS_DEMO", "OPENFORMS_COOKIE_SECURE",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if c.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.SMTPPort != 1025 {
		t.Errorf("SMTPPort = %d", c.SMTPPort)
	}
	if c.SMTPFrom != "openforms@localhost" {
		t.Errorf("SMTPFrom = %q", c.SMTPFrom)
	}
	if c.WorkerConcurrency != 4 {
		t.Errorf("WorkerConcurrency = %d", c.WorkerConcurrency)
	}
	if c.DemoMode || c.CookieSecure {
		t.Errorf("DemoMode=%v CookieSecure=%v, want false/false", c.DemoMode, c.CookieSecure)
	}
	if c.DatabaseURL != "" || c.SMTPHost != "" || c.WebhookSecret != "" {
		t.Errorf("expected empty optional values, got %+v", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPENFORMS_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("OPENFORMS_DATABASE_URL", "postgres://x")
	t.Setenv("OPENFORMS_BASE_URL", "https://forms.example.com/")
	t.Setenv("OPENFORMS_SMTP_HOST", "smtp.example.com")
	t.Setenv("OPENFORMS_SMTP_PORT", "2525")
	t.Setenv("OPENFORMS_SMTP_USERNAME", "u")
	t.Setenv("OPENFORMS_SMTP_PASSWORD", "p")
	t.Setenv("OPENFORMS_SMTP_FROM", "noreply@example.com")
	t.Setenv("OPENFORMS_WEBHOOK_SECRET", "s3cret")
	t.Setenv("OPENFORMS_WORKER_CONCURRENCY", "8")
	t.Setenv("OPENFORMS_DEMO", "1")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{
		HTTPAddr: "127.0.0.1:9000", DatabaseURL: "postgres://x", BaseURL: "https://forms.example.com",
		SMTPHost: "smtp.example.com", SMTPPort: 2525, SMTPUsername: "u", SMTPPassword: "p",
		SMTPFrom: "noreply@example.com", WebhookSecret: "s3cret", WorkerConcurrency: 8,
		DemoMode: true, CookieSecure: true,
	}
	if c != want {
		t.Errorf("got %+v\nwant %+v", c, want)
	}
}

func TestCookieSecureExplicitOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPENFORMS_BASE_URL", "https://forms.example.com")
	t.Setenv("OPENFORMS_COOKIE_SECURE", "false")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.CookieSecure {
		t.Error("CookieSecure should be false when explicitly disabled")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]string{
		"OPENFORMS_SMTP_PORT":          "abc",
		"OPENFORMS_WORKER_CONCURRENCY": "0",
		"OPENFORMS_DEMO":               "maybe",
		"OPENFORMS_COOKIE_SECURE":      "yes please",
	}
	for key, val := range cases {
		t.Run(key, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(key, val)
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("err = %v, want error mentioning %s", err, key)
			}
		})
	}
}

func TestRequireDatabase(t *testing.T) {
	if err := (config.Config{}).RequireDatabase(); err == nil || !strings.Contains(err.Error(), "OPENFORMS_DATABASE_URL") {
		t.Fatalf("err = %v", err)
	}
	if err := (config.Config{DatabaseURL: "postgres://x"}).RequireDatabase(); err != nil {
		t.Fatalf("err = %v", err)
	}
}
