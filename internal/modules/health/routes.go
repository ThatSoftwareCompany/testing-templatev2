package health

import (
	"net/http"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/httpserver"
)

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	// This route is template-managed operational infrastructure. Register
	// application-specific routes through internal/app/routes.go instead.
	controller := NewController(service)
	mux.Handle("/api/v1/health", httpserver.OnlyMethods(http.MethodGet)(http.HandlerFunc(controller.Handle)))
}
