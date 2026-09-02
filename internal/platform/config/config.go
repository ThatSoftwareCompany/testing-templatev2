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
	Auth                   AuthConfig
}

type DatabaseConfig struct {
	Enabled       bool
	URL           string
	MaxConns      int32
	MinConns      int32
	HealthTimeout time.Duration
}

type AuthConfig struct {
	PrivateKeyFile  string
	PublicKeyFile   string
	KeyID           string
	JWTIssuer       string
	JWTAudience     string
	CSRFSecret      string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
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
	cfg.Auth.PrivateKeyFile = strings.TrimSpace(os.Getenv("AUTH_PRIVATE_KEY_FILE"))
	cfg.Auth.PublicKeyFile = strings.TrimSpace(os.Getenv("AUTH_PUBLIC_KEY_FILE"))
	cfg.Auth.KeyID = strings.TrimSpace(os.Getenv("AUTH_KEY_ID"))
	cfg.Auth.JWTIssuer = strings.TrimSpace(os.Getenv("AUTH_JWT_ISSUER"))
	cfg.Auth.JWTAudience = strings.TrimSpace(os.Getenv("AUTH_JWT_AUDIENCE"))
	cfg.Auth.CSRFSecret = os.Getenv("AUTH_CSRF_SECRET")
	if cfg.Auth.AccessTokenTTL, err = durationEnv("AUTH_ACCESS_TOKEN_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.Auth.RefreshTokenTTL, err = durationEnv("AUTH_REFRESH_TOKEN_TTL", 30*24*time.Hour); err != nil {
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
	if c.Auth.AccessTokenTTL <= 0 {
		return fmt.Errorf("AUTH_ACCESS_TOKEN_TTL must be greater than zero")
	}
	if c.Auth.RefreshTokenTTL < 7*24*time.Hour || c.Auth.RefreshTokenTTL > 30*24*time.Hour {
		return fmt.Errorf("AUTH_REFRESH_TOKEN_TTL must be between 7 and 30 days")
	}
	return nil
}

func (c Config) ValidateAuthRuntime() error {
	if !c.Database.Enabled {
		return nil
	}
	for _, field := range []struct {
		name     string
		value    string
		metadata bool
	}{
		{name: "AUTH_PRIVATE_KEY_FILE", value: c.Auth.PrivateKeyFile},
		{name: "AUTH_PUBLIC_KEY_FILE", value: c.Auth.PublicKeyFile},
		{name: "AUTH_KEY_ID", value: c.Auth.KeyID, metadata: true},
		{name: "AUTH_JWT_ISSUER", value: c.Auth.JWTIssuer, metadata: true},
		{name: "AUTH_JWT_AUDIENCE", value: c.Auth.JWTAudience, metadata: true},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required when DATABASE_ENABLED=true", field.name)
		}
		if field.metadata && !validAuthMetadata(field.value) {
			return fmt.Errorf("%s contains invalid characters", field.name)
		}
	}
	if len([]byte(c.Auth.CSRFSecret)) < 32 {
		return fmt.Errorf("AUTH_CSRF_SECRET must contain at least 32 bytes when DATABASE_ENABLED=true")
	}
	return nil
}

func validAuthMetadata(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character == '\u0000' || character == '\u007f' || character < ' ' || character == ' ' {
			return false
		}
	}
	return true
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
