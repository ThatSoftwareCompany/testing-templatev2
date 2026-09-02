package migrate

import (
	"database/sql"
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	DatabaseURL   string
	MigrationsDir string
}

func Up(cfg Config) error {
	return withMigrator(cfg, func(m *migrate.Migrate) error {
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("migrate up: %w", err)
		}
		return nil
	})
}

func Down(cfg Config, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("migration down steps must be greater than zero")
	}
	return withMigrator(cfg, func(m *migrate.Migrate) error {
		if err := m.Steps(-steps); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("migrate down: %w", err)
		}
		return nil
	})
}

func Version(cfg Config) (uint, bool, error) {
	var version uint
	var dirty bool
	err := withMigrator(cfg, func(m *migrate.Migrate) error {
		var err error
		version, dirty, err = m.Version()
		if err == migrate.ErrNilVersion {
			version = 0
			dirty = false
			return nil
		}
		return err
	})
	return version, dirty, err
}

func withMigrator(cfg Config, operation func(*migrate.Migrate) error) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(cfg.MigrationsDir, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := operation(m); err != nil {
		return err
	}
	return nil
}
