package example

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

func (c *Controller) HandleGet(w http.ResponseWriter, _ *http.Request) {
	httpserver.WriteJSON(w, http.StatusOK, c.service.GetExample())
}
