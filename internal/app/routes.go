// Package app contains application-owned composition extension points.
package app

import (
	"log/slog"
	"net/http"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/modules/auth"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/modules/example"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/errstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies contains shared runtime dependencies for application modules.
// Database is nil when DATABASE_ENABLED=false.
type Dependencies struct {
	Database   *pgxpool.Pool
	ErrorStore errstore.Store
	Logger     *slog.Logger
	Auth       *auth.Service
}

// RegisterRoutes is the application-owned route composition point.
//
// Derived repositories may add registrations for business modules here. Keep
// template-provided operational routes in cmd/api and internal/modules/health.
// The template updater preserves this file so application routes remain owned
// by the generated repository.
func RegisterRoutes(mux *http.ServeMux, dependencies Dependencies) {
	// This route simulates a product endpoint. It is intentionally registered
	// only from the application-owned extension point and requires both an
	// application role and permission.
	example.RegisterRoutes(mux, dependencies.Auth)
}
