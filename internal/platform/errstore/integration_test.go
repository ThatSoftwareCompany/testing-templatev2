//go:build integration

package errstore

import (
	"context"
	"os"
	"testing"
	"time"

	platformmigrate "github.com/ThatSoftwareCompany/template-go-api/internal/platform/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStorePersistsAndListsSafeEvents(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	if err := platformmigrate.Up(platformmigrate.Config{
		DatabaseURL:   databaseURL,
		MigrationsDir: "file://../../../migrations",
	}); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := platformmigrate.Up(platformmigrate.Config{
		DatabaseURL:   databaseURL,
		MigrationsDir: "file://../../../migrations",
	}); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	version, dirty, err := platformmigrate.Version(platformmigrate.Config{
		DatabaseURL:   databaseURL,
		MigrationsDir: "file://../../../migrations",
	})
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 1 || dirty {
		t.Fatalf("unexpected migration state: version=%d dirty=%t", version, dirty)
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()
	store := NewPostgresStore(pool)

	event := ErrorEvent{
		OccurredAt:    time.Now().UTC(),
		CorrelationID: "integration-correlation",
		Method:        "GET",
		Path:          "/api/v1/health",
		Endpoint:      "/api/v1/health",
		StatusCode:    503,
		ErrorCode:     "service_unavailable",
		Message:       "service unavailable",
	}
	if err := store.Persist(context.Background(), event); err != nil {
		t.Fatalf("persist event: %v", err)
	}

	items, err := store.List(context.Background(), Filter{Endpoint: event.Endpoint, Limit: 10})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(items) == 0 || items[0].CorrelationID != event.CorrelationID {
		t.Fatalf("unexpected events: %#v", items)
	}

	filtered, err := store.List(context.Background(), Filter{Endpoint: "/not-found", Limit: 1})
	if err != nil {
		t.Fatalf("list filtered events: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("unexpected filtered events: %#v", filtered)
	}

	if err := platformmigrate.Down(platformmigrate.Config{
		DatabaseURL:   databaseURL,
		MigrationsDir: "file://../../../migrations",
	}, 1); err != nil {
		t.Fatalf("rollback migration: %v", err)
	}
}
