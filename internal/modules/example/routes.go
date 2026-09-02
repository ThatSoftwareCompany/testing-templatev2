package example

import (
	"net/http"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/modules/auth"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/httpserver"
)

const (
	role       = "example_reader"
	permission = "example:read"
)

func RegisterRoutes(mux *http.ServeMux, authService *auth.Service) {
	controller := NewController(NewService())
	handler := http.Handler(http.HandlerFunc(controller.HandleGet))
	handler = auth.RequirePermission(authService, permission, handler)
	handler = auth.RequireRole(authService, role, handler)
	mux.Handle("/api/v1/example", httpserver.OnlyMethods(http.MethodGet)(handler))
}
