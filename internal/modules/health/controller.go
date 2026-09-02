package health

import (
	"net/http"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/httpserver"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) Handle(w http.ResponseWriter, r *http.Request) {
	response, ready := c.service.Check(r.Context())
	if !ready {
		httpserver.WriteJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, response)
}
