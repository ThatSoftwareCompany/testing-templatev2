// Package app contains application-owned composition extension points.
package app

import (
	"log/slog"
	"net/http"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/errstore"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/httpserver"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies contains shared runtime dependencies for application modules.
// Database is nil when DATABASE_ENABLED=false.
type Dependencies struct {
	Database   *pgxpool.Pool
	ErrorStore errstore.Store
	Logger     *slog.Logger
}

// RegisterRoutes is the application-owned route composition point.
//
// Derived repositories may add registrations for business modules here. Keep
// template-provided operational routes in cmd/api and internal/modules/health.
// The template updater preserves this file so application routes remain owned
// by the generated repository.
func RegisterRoutes(mux *http.ServeMux, _ Dependencies) {
	// This route exists only to validate application-owned route preservation
	// across template updates. Product routes should live in their own module.
	mux.HandleFunc("GET /api/v1/example", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]string{"status": "example route works"})
	})
}
