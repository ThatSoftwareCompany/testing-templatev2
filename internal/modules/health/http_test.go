package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/httpserver"
)

func TestHealthControllerReturnsDisabledDatabase(t *testing.T) {
	response := performHealthRequest(t, NewService(nil, time.Second))

	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok","database":"disabled"}
` {
		t.Fatalf("unexpected response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestHealthControllerReturnsDatabaseUp(t *testing.T) {
	response := performHealthRequest(t, NewService(&fakePinger{}, time.Second))

	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok","database":"up"}
` {
		t.Fatalf("unexpected response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestHealthControllerReturnsDatabaseDown(t *testing.T) {
	response := performHealthRequest(t, NewService(&fakePinger{err: errors.New("database unavailable")}, time.Second))

	if response.Code != http.StatusServiceUnavailable || response.Body.String() != `{"status":"not_ready","database":"down"}
` {
		t.Fatalf("unexpected response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func performHealthRequest(t *testing.T, service *Service) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	recorder := httptest.NewRecorder()
	httpserver.CorrelationID(mux).ServeHTTP(recorder, request)
	return recorder
}
