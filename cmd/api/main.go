package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThatSoftwareCompany/template-go-api/internal/app"
	"github.com/ThatSoftwareCompany/template-go-api/internal/modules/health"
	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/config"
	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/db"
	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/errstore"
	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/httpserver"
	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/logging"
	platformmigrate "github.com/ThatSoftwareCompany/template-go-api/internal/platform/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("invalid_configuration")
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.AppEnv).With("app_name", cfg.AppName)
	slog.SetDefault(logger)
	if err := run(cfg, logger); err != nil {
		logger.Error("api_stopped", "reason", "startup_or_shutdown_failure")
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.MigrationsRunOnStartup {
		if err := platformmigrate.Up(platformmigrate.Config{
			DatabaseURL:   cfg.Database.URL,
			MigrationsDir: cfg.MigrationsDir,
		}); err != nil {
			logger.Error("migrations_failed", "reason", "startup_migration_failed")
			return err
		}
		logger.Info("migrations_completed")
	}

	var pool *pgxpool.Pool
	if cfg.Database.Enabled {
		database, err := db.Open(rootContext, cfg.Database)
		if err != nil {
			logger.Error("database_initialization_failed", "reason", "database_unavailable")
			return err
		}
		pool = database
		defer pool.Close()
	}

	store := errstore.NewNoopStore()
	if pool != nil {
		store = errstore.NewPostgresStore(pool)
	}

	server := httpserver.NewServer(cfg, logger, store)
	server.Mux.Handle("/__ping", httpserver.OnlyMethods(http.MethodGet)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))
	var healthPinger health.Pinger
	if pool != nil {
		healthPinger = pool
	}
	health.RegisterRoutes(server.Mux, health.NewService(healthPinger, cfg.Database.HealthTimeout))
	// Template-managed operational routes are registered above. Application-owned
	// routes belong in internal/app/routes.go to keep template updates isolated.
	app.RegisterRoutes(server.Mux, app.Dependencies{
		Database:   pool,
		ErrorStore: store,
		Logger:     logger,
	})

	serverErrors := make(chan error, 1)
	go func() {
		if err := server.HTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-rootContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
		defer cancel()
		if err := server.HTTP.Shutdown(shutdownContext); err != nil {
			logger.Error("http_shutdown_failed", "reason", "shutdown_timeout")
			return err
		}
		return nil
	}
}
