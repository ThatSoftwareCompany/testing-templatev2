package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	Development = "development"
	Test        = "test"
	Production  = "production"
)

type Config struct {
	AppName                string
	AppEnv                 string
	HTTPAddr               string
	HTTPReadTimeout        time.Duration
	HTTPWriteTimeout       time.Duration
	HTTPIdleTimeout        time.Duration
	HTTPShutdownTimeout    time.Duration
	LogLevel               string
	Database               DatabaseConfig
	MigrationsDir          string
	MigrationsRunOnStartup bool
	CORSAllowedOrigins     []string
}

type DatabaseConfig struct {
	Enabled       bool
	URL           string
	MaxConns      int32
	MinConns      int32
	HealthTimeout time.Duration
}

func Load() (Config, error) {
	var err error
	cfg := Config{}

	cfg.AppName = envOrDefault("APP_NAME", "template-go-api")
	cfg.AppEnv = strings.ToLower(envOrDefault("APP_ENV", Development))
	cfg.HTTPAddr = envOrDefault("HTTP_ADDR", ":8080")
	cfg.LogLevel = strings.ToLower(envOrDefault("LOG_LEVEL", "info"))
	cfg.MigrationsDir = envOrDefault("MIGRATIONS_DIR", "file://migrations")

	if cfg.HTTPReadTimeout, err = durationEnv("HTTP_READ_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = durationEnv("HTTP_WRITE_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = durationEnv("HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPShutdownTimeout, err = durationEnv("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}

	if cfg.Database.Enabled, err = boolEnv("DATABASE_ENABLED", true); err != nil {
		return Config{}, err
	}
	cfg.Database.URL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if cfg.Database.MaxConns, err = int32Env("DATABASE_MAX_CONNS", 10); err != nil {
		return Config{}, err
	}
	if cfg.Database.MinConns, err = int32Env("DATABASE_MIN_CONNS", 1); err != nil {
		return Config{}, err
	}
	if cfg.Database.HealthTimeout, err = durationEnv("DATABASE_HEALTH_TIMEOUT", 3*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MigrationsRunOnStartup, err = boolEnv("MIGRATIONS_RUN_ON_STARTUP", false); err != nil {
		return Config{}, err
	}
	if cfg.CORSAllowedOrigins, err = originsEnv("CORS_ALLOWED_ORIGINS"); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.AppName) == "" {
		return fmt.Errorf("APP_NAME must not be empty")
	}
	switch c.AppEnv {
	case Development, Test, Production:
	default:
		return fmt.Errorf("APP_ENV must be one of development, test, or production")
	}
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, or error")
	}
	if c.HTTPReadTimeout <= 0 || c.HTTPWriteTimeout <= 0 || c.HTTPIdleTimeout <= 0 || c.HTTPShutdownTimeout <= 0 {
		return fmt.Errorf("HTTP timeouts must be greater than zero")
	}
	if c.Database.MaxConns <= 0 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be greater than zero")
	}
	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("DATABASE_MIN_CONNS must be between zero and DATABASE_MAX_CONNS")
	}
	if c.Database.HealthTimeout <= 0 {
		return fmt.Errorf("DATABASE_HEALTH_TIMEOUT must be greater than zero")
	}
	if c.Database.Enabled && strings.TrimSpace(c.Database.URL) == "" {
		return fmt.Errorf("DATABASE_URL is required when DATABASE_ENABLED=true")
	}
	if c.MigrationsRunOnStartup && !c.Database.Enabled {
		return fmt.Errorf("MIGRATIONS_RUN_ON_STARTUP requires DATABASE_ENABLED=true")
	}
	if c.MigrationsRunOnStartup && c.AppEnv == Production {
		return fmt.Errorf("MIGRATIONS_RUN_ON_STARTUP must be false in production")
	}
	if strings.TrimSpace(c.MigrationsDir) == "" {
		return fmt.Errorf("MIGRATIONS_DIR must not be empty")
	}
	return nil
}

func (c Config) MigrationURLRequired() error {
	if strings.TrimSpace(c.Database.URL) == "" {
		return fmt.Errorf("DATABASE_URL is required for migrations")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func int32Env(key string, fallback int32) (int32, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer", key)
	}
	return int32(parsed), nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func originsEnv(key string) ([]string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS cannot contain * when credentials are enabled")
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS contains an invalid origin")
		}
		origins = append(origins, strings.TrimRight(origin, "/"))
	}
	return origins, nil
}
