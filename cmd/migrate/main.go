package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/config"
	platformmigrate "github.com/ThatSoftwareCompany/template-go-api/internal/platform/migrate"
)

func main() {
	command := flag.String("command", "", "migration command: up, down, or version")
	steps := flag.Int("steps", 1, "number of migrations to revert for down")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration")
		os.Exit(1)
	}
	if err := cfg.MigrationURLRequired(); err != nil {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required for migrations")
		os.Exit(1)
	}
	migrationConfig := platformmigrate.Config{
		DatabaseURL:   cfg.Database.URL,
		MigrationsDir: cfg.MigrationsDir,
	}

	switch *command {
	case "up":
		err = platformmigrate.Up(migrationConfig)
	case "down":
		err = platformmigrate.Down(migrationConfig, *steps)
	case "version":
		var version uint
		var dirty bool
		version, dirty, err = platformmigrate.Version(migrationConfig)
		if err == nil {
			fmt.Printf("version=%d dirty=%t\n", version, dirty)
		}
	default:
		fmt.Fprintln(os.Stderr, "-command must be one of up, down, or version")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration command failed")
		os.Exit(1)
	}
}
