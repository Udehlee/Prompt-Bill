package config

import (
	"strings"
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	validConfig := func() *Config {
		return &Config{
			Port:              "8080",
			UpstreamBaseURL:   "https://api.openai.com",
			DatabaseURL:       "postgres://user:secret@localhost:5432/tracker_dev?sslmode=disable",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
			ShutdownTimeout:   10 * time.Second,
			LogLevel:          "info",
		}
	}

	tests := []struct {
		name        string
		modify      func(*Config)
		expectError string
	}{
		{
			name:        "valid configuration",
			modify:      func(c *Config) {},
			expectError: "",
		},
		{
			name: "empty port",
			modify: func(c *Config) {
				c.Port = ""
			},
			expectError: "PORT must not be empty",
		},
		{
			name: "invalid port non-numeric",
			modify: func(c *Config) {
				c.Port = "not-a-port"
			},
			expectError: "invalid PORT",
		},
		{
			name: "invalid port out of range",
			modify: func(c *Config) {
				c.Port = "70000"
			},
			expectError: "invalid PORT",
		},
		{
			name: "missing upstream URL",
			modify: func(c *Config) {
				c.UpstreamBaseURL = ""
			},
			expectError: "UPSTREAM_BASE_URL is required",
		},
		{
			name: "invalid upstream URL scheme",
			modify: func(c *Config) {
				c.UpstreamBaseURL = "ftp://api.openai.com"
			},
			expectError: "must be an absolute http or https URL",
		},
		{
			name: "invalid upstream URL missing host",
			modify: func(c *Config) {
				c.UpstreamBaseURL = "http://"
			},
			expectError: "must be an absolute http or https URL",
		},
		{
			name: "invalid upstream URL with user credentials",
			modify: func(c *Config) {
				c.UpstreamBaseURL = "https://user:pass@api.openai.com"
			},
			expectError: "must not contain user credentials",
		},
		{
			name: "missing database URL",
			modify: func(c *Config) {
				c.DatabaseURL = ""
			},
			expectError: "DATABASE_URL is required",
		},
		{
			name: "non-positive read header timeout",
			modify: func(c *Config) {
				c.ReadHeaderTimeout = 0
			},
			expectError: "READ_HEADER_TIMEOUT must be positive",
		},
		{
			name: "non-positive read timeout",
			modify: func(c *Config) {
				c.ReadTimeout = -1 * time.Second
			},
			expectError: "READ_TIMEOUT must be positive",
		},
		{
			name: "non-positive idle timeout",
			modify: func(c *Config) {
				c.IdleTimeout = 0
			},
			expectError: "IDLE_TIMEOUT must be positive",
		},
		{
			name: "non-positive shutdown timeout",
			modify: func(c *Config) {
				c.ShutdownTimeout = 0
			},
			expectError: "SHUTDOWN_TIMEOUT must be positive",
		},
		{
			name: "invalid log level",
			modify: func(c *Config) {
				c.LogLevel = "verbose"
			},
			expectError: "invalid LOG_LEVEL",
		},
		{
			name: "valid fallback URL",
			modify: func(c *Config) {
				c.FallbackBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
			},
			expectError: "",
		},
		{
			name: "invalid fallback URL scheme",
			modify: func(c *Config) {
				c.FallbackBaseURL = "ftp://generativelanguage.googleapis.com"
			},
			expectError: "must be an absolute http or https URL",
		},
		{
			name: "invalid fallback URL with user credentials",
			modify: func(c *Config) {
				c.FallbackBaseURL = "https://user:pass@generativelanguage.googleapis.com"
			},
			expectError: "must not contain user credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.expectError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectError)
				}
				if !strings.Contains(err.Error(), tt.expectError) {
					t.Fatalf("expected error containing %q, got %q", tt.expectError, err.Error())
				}
			}
		})
	}
}

func TestConfig_SafeDatabaseURL(t *testing.T) {
	cfg := &Config{
		DatabaseURL: "postgres://tracker_user:super_secret_password@localhost:5432/tracker_dev?sslmode=disable",
	}

	safe := cfg.SafeDatabaseURL()
	if strings.Contains(safe, "super_secret_password") {
		t.Fatalf("safe URL leaked raw password: %s", safe)
	}
	if !strings.Contains(safe, "tracker_user:xxxxx@localhost:5432") {
		t.Fatalf("expected safe password placeholder, got: %s", safe)
	}
}

func TestConfig_Load(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("UPSTREAM_BASE_URL", "http://127.0.0.1:8081")
	t.Setenv("DATABASE_URL", "postgres://test:pass@127.0.0.1:5432/testdb")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("READ_HEADER_TIMEOUT", "2s")
	t.Setenv("READ_TIMEOUT", "15s")
	t.Setenv("IDLE_TIMEOUT", "60s")
	t.Setenv("SHUTDOWN_TIMEOUT", "5s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected Load error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("expected Port 9090, got %s", cfg.Port)
	}
	if cfg.ListenAddr() != ":9090" {
		t.Errorf("expected ListenAddr :9090, got %s", cfg.ListenAddr())
	}
	if cfg.UpstreamBaseURL != "http://127.0.0.1:8081" {
		t.Errorf("expected UpstreamBaseURL http://127.0.0.1:8081, got %s", cfg.UpstreamBaseURL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %s", cfg.LogLevel)
	}
	if cfg.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("expected ReadHeaderTimeout 2s, got %v", cfg.ReadHeaderTimeout)
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Errorf("expected ReadTimeout 15s, got %v", cfg.ReadTimeout)
	}
	if cfg.IdleTimeout != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", cfg.IdleTimeout)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("expected ShutdownTimeout 5s, got %v", cfg.ShutdownTimeout)
	}
}

func TestConfig_InferProvider(t *testing.T) {
	tests := []struct {
		url      string
		explicit string
		fallback string
		expected string
	}{
		{"https://api.groq.com/openai/v1", "", "openai", "groq"},
		{"https://generativelanguage.googleapis.com/v1beta/openai", "", "gemini", "gemini"},
		{"https://api.openai.com/v1", "", "openai", "openai"},
		{"https://custom-proxy.internal", "custom", "openai", "custom"},
		{"https://custom-proxy.internal", "", "gemini", "gemini"},
	}

	for _, tt := range tests {
		got := inferProvider(tt.url, tt.explicit, tt.fallback)
		if got != tt.expected {
			t.Errorf("inferProvider(%q, %q, %q) = %q, want %q", tt.url, tt.explicit, tt.fallback, got, tt.expected)
		}
	}
}

func TestConfig_ResolveDatabaseURL(t *testing.T) {
	// Test direct DATABASE_URL
	t.Setenv("DATABASE_URL", "postgres://myuser:mypass@localhost:5432/mydb?sslmode=disable")
	t.Setenv("RUNNING_IN_DOCKER", "")
	got := resolveDatabaseURL()
	if got != "postgres://myuser:mypass@localhost:5432/mydb?sslmode=disable" {
		t.Errorf("unexpected URL: %s", got)
	}

	// Test rewrite inside docker
	t.Setenv("RUNNING_IN_DOCKER", "true")
	gotDocker := resolveDatabaseURL()
	if gotDocker != "postgres://myuser:mypass@host.docker.internal:5432/mydb?sslmode=disable" {
		t.Errorf("unexpected docker URL: %s", gotDocker)
	}

	// Test fallback to individual variables
	t.Setenv("DATABASE_URL", "")
	t.Setenv("RUNNING_IN_DOCKER", "")
	t.Setenv("POSTGRES_USER", "realuser")
	t.Setenv("POSTGRES_PASSWORD", "secret#pass")
	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "realdb")
	t.Setenv("POSTGRES_SSLMODE", "require")

	assembled := resolveDatabaseURL()
	expected := "postgres://realuser:secret%23pass@127.0.0.1:5432/realdb?sslmode=require"
	if assembled != expected {
		t.Errorf("assembled URL = %q, want %q", assembled, expected)
	}
}
