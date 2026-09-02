package example

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControllerHandleGet(t *testing.T) {
	controller := NewController(NewService())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/example", nil)
	response := httptest.NewRecorder()

	controller.HandleGet(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), `"status":"example route works"`) {
		t.Fatalf("unexpected response body: %s", response.Body.String())
	}
}
