package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/config"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/errstore"
)

type recordingStore struct {
	events []errstore.ErrorEvent
}

func (s *recordingStore) Persist(_ context.Context, event errstore.ErrorEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordingStore) List(context.Context, errstore.Filter) ([]errstore.ErrorEvent, error) {
	return s.events, nil
}

func testConfig() config.Config {
	return config.Config{
		AppName:             "test-api",
		AppEnv:              config.Test,
		HTTPAddr:            ":8080",
		HTTPReadTimeout:     time.Second,
		HTTPWriteTimeout:    time.Second,
		HTTPIdleTimeout:     time.Second,
		HTTPShutdownTimeout: time.Second,
		LogLevel:            "info",
		CORSAllowedOrigins:  []string{"http://frontend.test"},
		Database:            config.DatabaseConfig{HealthTimeout: time.Second},
	}
}

func TestPingResponseDoesNotNeedDatabase(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	server.Mux.Handle("/__ping", OnlyMethods(http.MethodGet)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))

	request := httptest.NewRequest(http.MethodGet, "/__ping", nil)
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected ping response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", recorder.Header().Get("Content-Type"))
	}
}

func TestNotFoundResponseIncludesStableErrorContract(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	request.Header.Set("X-Correlation-ID", "not-found-correlation")
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", recorder.Header().Get("Content-Type"))
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"code":"not_found"`) || !strings.Contains(body, `"correlation_id":"not-found-correlation"`) {
		t.Fatalf("unexpected not-found response: %s", body)
	}
}

func TestCorrelationIDIsValidatedAndReturned(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	server.Mux.Handle("/test", OnlyMethods(http.MethodGet)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))

	validRequest := httptest.NewRequest(http.MethodGet, "/test", nil)
	validRequest.Header.Set("X-Correlation-ID", "request-123")
	validRecorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(validRecorder, validRequest)
	if validRecorder.Header().Get("X-Correlation-ID") != "request-123" {
		t.Fatalf("valid correlation ID was not preserved")
	}

	invalidRequest := httptest.NewRequest(http.MethodGet, "/test", nil)
	invalidRequest.Header.Set("X-Correlation-ID", "bad value with spaces")
	invalidRecorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(invalidRecorder, invalidRequest)
	if invalidRecorder.Header().Get("X-Correlation-ID") == "bad value with spaces" || invalidRecorder.Header().Get("X-Correlation-ID") == "" {
		t.Fatalf("invalid correlation ID was not replaced")
	}
}

func TestSecurityHeadersAndCors(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	server.Mux.Handle("/test", OnlyMethods(http.MethodGet)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set("Origin", "http://frontend.test")
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	for header, expected := range map[string]string{
		"X-Content-Type-Options":           "nosniff",
		"X-Frame-Options":                  "DENY",
		"Referrer-Policy":                  "no-referrer",
		"Permissions-Policy":               "camera=(), microphone=(), geolocation=(), payment=()",
		"Access-Control-Allow-Origin":      "http://frontend.test",
		"Access-Control-Allow-Credentials": "true",
	} {
		if recorder.Header().Get(header) != expected {
			t.Fatalf("header %s = %q, want %q", header, recorder.Header().Get(header), expected)
		}
	}
	if strings.Contains(recorder.Header().Get("Access-Control-Allow-Origin"), "*") {
		t.Fatal("CORS wildcard must not be emitted")
	}
}

func TestCorsRejectsUnlistedOrigin(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	server.Mux.Handle("/test-unlisted", OnlyMethods(http.MethodGet)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))

	request := httptest.NewRequest(http.MethodGet, "/test-unlisted", nil)
	request.Header.Set("Origin", "https://not-allowed.example")
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS must not allow an unlisted origin")
	}
}

func TestCorsPreflightReturnsNoContentForAllowedOrigin(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	request.Header.Set("Origin", "http://frontend.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "X-CSRF-Token")
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "http://frontend.test" {
		t.Fatalf("unexpected allowed origin: %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
	if recorder.Header().Get("Access-Control-Allow-Methods") != "GET, OPTIONS, POST" {
		t.Fatalf("unexpected allowed methods: %q", recorder.Header().Get("Access-Control-Allow-Methods"))
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), "X-CSRF-Token") {
		t.Fatalf("CSRF header is not allowed: %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestMethodNotAllowedIncludesAllowHeader(t *testing.T) {
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), errstore.NewNoopStore())
	server.Mux.Handle("/test", OnlyMethods(http.MethodGet)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))

	request := httptest.NewRequest(http.MethodPost, "/test", nil)
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("unexpected method response: status=%d allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestPanicRecoveryPersistsSafeFiveHundredEvent(t *testing.T) {
	store := &recordingStore{}
	server := NewServer(testConfig(), slog.New(slog.NewJSONHandler(testWriter{t}, nil)), store)
	server.Mux.Handle("/panic", OnlyMethods(http.MethodGet)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret token should not be exposed")
	})))

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set("Authorization", "Bearer secret-token")
	recorder := httptest.NewRecorder()
	server.HTTP.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("panic details leaked in response: %s", recorder.Body.String())
	}
	if len(store.events) != 1 || store.events[0].StatusCode != http.StatusInternalServerError || store.events[0].Message != "internal server error" {
		t.Fatalf("unexpected persisted event: %#v", store.events)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
