package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/config"
)

func TestOpenRejectsInvalidDatabaseURL(t *testing.T) {
	_, err := Open(context.Background(), config.DatabaseConfig{
		URL:           "not-a-database-url",
		MaxConns:      2,
		MinConns:      1,
		HealthTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "parse database configuration") {
		t.Fatalf("Open() error = %v, want database configuration parse error", err)
	}
}

func TestOpenFailsFastWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Open(ctx, config.DatabaseConfig{
		URL:           "postgres://user:password@localhost:5432/app?sslmode=disable",
		MaxConns:      2,
		MinConns:      1,
		HealthTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "ping database") {
		t.Fatalf("Open() error = %v, want ping failure", err)
	}
}
