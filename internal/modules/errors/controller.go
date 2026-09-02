package errors

import (
	"net/http"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/httpserver"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

// HandleList is registered behind authentication and the errors:read
// permission by cmd/api.
func (c *Controller) HandleList(w http.ResponseWriter, r *http.Request) {
	response, err := c.service.List(r.Context(), ListRequest{
		Endpoint: r.URL.Query().Get("endpoint"),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, response)
}
