package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings loaded from environment variables.
type Config struct {
	Port string

	UpstreamBaseURL  string 
	UpstreamAPIKey   string
	UpstreamProvider string 

	FallbackBaseURL  string
	FallbackAPIKey   string
	FallbackModel    string 
	FallbackProvider string 

	DatabaseURL string 
	ReadHeaderTimeout time.Duration 
	ReadTimeout       time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration

	LogLevel string
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		UpstreamBaseURL: os.Getenv("UPSTREAM_BASE_URL"),
		UpstreamAPIKey:  os.Getenv("UPSTREAM_API_KEY"),
		FallbackBaseURL: os.Getenv("FALLBACK_BASE_URL"),
		FallbackAPIKey:  os.Getenv("FALLBACK_API_KEY"),
		FallbackModel:   os.Getenv("FALLBACK_MODEL"),
		DatabaseURL:     resolveDatabaseURL(),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
	cfg.UpstreamProvider = inferProvider(cfg.UpstreamBaseURL, os.Getenv("UPSTREAM_PROVIDER"), "openai")
	cfg.FallbackProvider = inferProvider(cfg.FallbackBaseURL, os.Getenv("FALLBACK_PROVIDER"), "gemini")

	timeouts := []struct {
		key    string
		target *time.Duration
		def    time.Duration
	}{
		{"READ_HEADER_TIMEOUT", &cfg.ReadHeaderTimeout, 5 * time.Second},
		{"READ_TIMEOUT", &cfg.ReadTimeout, 30 * time.Second},
		{"IDLE_TIMEOUT", &cfg.IdleTimeout, 120 * time.Second},
		{"SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout, 10 * time.Second},
	}
	for _, t := range timeouts {
		d, err := getDuration(t.key, t.def)
		if err != nil {
			return nil, err
		}
		*t.target = d
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate validates the required fields are present
func (c *Config) Validate() error {
	if c.Port == "" {
		return errors.New("PORT must not be empty")
	}
	p, err := strconv.Atoi(c.Port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid PORT %q: must be an integer between 1 and 65535", c.Port)
	}

	if c.UpstreamBaseURL == "" {
		return errors.New("UPSTREAM_BASE_URL is required")
	}
	if err := validateBaseURL("UPSTREAM_BASE_URL", c.UpstreamBaseURL); err != nil {
		return err
	}
	if c.FallbackBaseURL != "" {
		if err := validateBaseURL("FALLBACK_BASE_URL", c.FallbackBaseURL); err != nil {
			return err
		}
	}

	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	if c.ReadHeaderTimeout <= 0 {
		return errors.New("READ_HEADER_TIMEOUT must be positive")
	}
	if c.ReadTimeout <= 0 {
		return errors.New("READ_TIMEOUT must be positive")
	}
	if c.IdleTimeout <= 0 {
		return errors.New("IDLE_TIMEOUT must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("SHUTDOWN_TIMEOUT must be positive")
	}

	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid LOG_LEVEL %q: must be debug, info, warn, or error", c.LogLevel)
	}
	return nil
}

// ListenAddr returns the TCP address to bind the listener to
func (c *Config) ListenAddr() string {
	return ":" + c.Port
}

// SafeDatabaseURL returns the database URL with the password hidden, safe for logs.
func (c *Config) SafeDatabaseURL() string{
	u, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return "[unparseable database URL]"
	}
	return u.Redacted()
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getDuration reads a positive duration from env var key
// returns def if unset.
func getDuration(key string, def time.Duration) (time.Duration, error) {
	val := os.Getenv(key)
	if val == "" {
		return def, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}

// validateBaseURL checks that raw is an absolute http(s) URL without embedded credentials.
func validateBaseURL(name, raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid %s %q: must be an absolute http or https URL", name, raw)
	}
	if u.User != nil {
		return fmt.Errorf("invalid %s: must not contain user credentials", name)
	}
	return nil
}

// inferProvider returns the explicit provider name if set, otherwise guesses it from the URL.
func inferProvider(baseURL, explicit, def string) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return strings.ToLower(explicit)
	}
	u := strings.ToLower(baseURL)
	for _, p := range []struct{ match, name string }{
		{"groq.com", "groq"},
		{"googleapis.com", "gemini"},
		{"gemini", "gemini"},
		{"openai.com", "openai"},
	} {
		if strings.Contains(u, p.match) {
			return p.name
		}
	}
	return def
}

// resolveDatabaseURL uses DATABASE_URL if set, otherwise builds one from POSTGRES_* variables.
func resolveDatabaseURL() string {
	rawURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if rawURL == "" {
		user := os.Getenv("POSTGRES_USER")
		if user == "" {
			return ""
		}

		u := url.URL{
			Scheme:   "postgres",
			User:     url.User(user),
			Host:     net.JoinHostPort(getEnv("POSTGRES_HOST", "localhost"), getEnv("POSTGRES_PORT", "5432")),
			Path:     "/" + getEnv("POSTGRES_DB", "postgres"),
			RawQuery: "sslmode=" + getEnv("POSTGRES_SSLMODE", "disable"),
		}
		if pass := os.Getenv("POSTGRES_PASSWORD"); pass != "" {
			u.User = url.UserPassword(user, pass)
		}
		rawURL = u.String()
	}

	if rawURL != "" && isRunningInDocker() {
		rawURL = strings.Replace(rawURL, "@localhost:", "@host.docker.internal:", 1)
		rawURL = strings.Replace(rawURL, "@127.0.0.1:", "@host.docker.internal:", 1)
	}
	return rawURL
}

func isRunningInDocker() bool {
	if os.Getenv("RUNNING_IN_DOCKER") == "true" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}