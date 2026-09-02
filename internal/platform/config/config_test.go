package config

import (
	"testing"
)

func TestLoadUsesPostgresByDefault(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost:5432/app?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Database.Enabled {
		t.Fatal("expected PostgreSQL to be enabled by default")
	}
}

func TestLoadAllowsDatabaseDisabled(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("DATABASE_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.Enabled {
		t.Fatal("expected database to be disabled")
	}
}

func TestLoadRejectsMissingDatabaseURLWhenEnabled(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL error")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "environment", key: "APP_ENV", value: "staging"},
		{name: "boolean", key: "DATABASE_ENABLED", value: "sometimes"},
		{name: "duration", key: "HTTP_READ_TIMEOUT", value: "not-a-duration"},
		{name: "integer", key: "DATABASE_MAX_CONNS", value: "many"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBaseEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatalf("expected invalid %s to fail", test.key)
			}
		})
	}
}

func TestLoadRejectsProductionAutomaticMigrations(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("MIGRATIONS_RUN_ON_STARTUP", "true")

	if _, err := Load(); err == nil {
		t.Fatal("expected production automatic migrations to fail validation")
	}
}

func TestLoadRejectsWildcardCors(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	if _, err := Load(); err == nil {
		t.Fatal("expected wildcard CORS origin to fail validation")
	}
}

func TestLoadRejectsInvalidConnectionBounds(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("DATABASE_MIN_CONNS", "11")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid connection bounds to fail")
	}
}

func TestLoadRejectsMigrationsWithoutDatabase(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("MIGRATIONS_RUN_ON_STARTUP", "true")

	if _, err := Load(); err == nil {
		t.Fatal("expected migrations without database to fail")
	}
}

func setBaseEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_NAME", "test-api")
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("HTTP_READ_TIMEOUT", "5s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "10s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "1m")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("DATABASE_ENABLED", "true")
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost:5432/app?sslmode=disable")
	t.Setenv("DATABASE_MAX_CONNS", "10")
	t.Setenv("DATABASE_MIN_CONNS", "1")
	t.Setenv("DATABASE_HEALTH_TIMEOUT", "3s")
	t.Setenv("MIGRATIONS_DIR", "file://migrations")
	t.Setenv("MIGRATIONS_RUN_ON_STARTUP", "false")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
}
