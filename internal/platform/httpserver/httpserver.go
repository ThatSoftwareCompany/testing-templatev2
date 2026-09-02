package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/config"
	"github.com/ThatSoftwareCompany/template-go-api/internal/platform/errstore"
)

type contextKey string

const correlationIDKey contextKey = "correlation_id"

type Server struct {
	HTTP *http.Server
	Mux  *http.ServeMux
}

func NewServer(cfg config.Config, logger *slog.Logger, store errstore.Store) *Server {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, "not_found", "resource not found")
	}))

	root := CorrelationID(
		SecurityHeaders(
			CORS(cfg.CORSAllowedOrigins,
				RequestLogger(logger,
					ErrorPersistence(logger, store,
						Recovery(logger, mux))))))

	return &Server{
		Mux: mux,
		HTTP: &http.Server{
			Addr:         cfg.HTTPAddr,
			Handler:      root,
			ReadTimeout:  cfg.HTTPReadTimeout,
			WriteTimeout: cfg.HTTPWriteTimeout,
			IdleTimeout:  cfg.HTTPIdleTimeout,
		},
	}
}

func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if !validCorrelationID(correlationID) {
			correlationID = newCorrelationID()
		}
		w.Header().Set("X-Correlation-ID", correlationID)
		ctx := context.WithValue(r.Context(), correlationIDKey, correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CorrelationIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDKey).(string)
	return value
}

func OnlyMethods(methods ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		allowed[method] = struct{}{}
	}
	allowHeader := strings.Join(methods, ", ")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowed[r.Method]; !ok {
				w.Header().Set("Allow", allowHeader)
				WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	WriteJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":           code,
			"message":        message,
			"correlation_id": CorrelationIDFromContext(r.Context()),
		},
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		logger.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"correlation_id", CorrelationIDFromContext(r.Context()),
		)
	})
}

func ErrorPersistence(logger *slog.Logger, store errstore.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.Status()
		if status < http.StatusInternalServerError {
			return
		}

		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
		defer cancel()
		if err := store.Persist(persistCtx, errstore.NewEvent(
			time.Now().UTC(),
			CorrelationIDFromContext(r.Context()),
			r.Method,
			r.URL.Path,
			r.URL.Path,
			status,
			statusErrorCode(status),
			safeErrorMessage(status),
		)); err != nil {
			logger.Error("error_store_write_failed",
				"correlation_id", CorrelationIDFromContext(r.Context()),
				"path", r.URL.Path,
			)
		}
	})
}

func Recovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic_recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"correlation_id", CorrelationIDFromContext(r.Context()),
				)
				WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		next.ServeHTTP(w, r)
	})
}

func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[strings.TrimRight(origin, "/")] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
		if _, ok := origins[origin]; ok && origin != "" {
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, X-Correlation-ID")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func statusErrorCode(status int) string {
	if status == http.StatusServiceUnavailable {
		return "service_unavailable"
	}
	return "internal_error"
}

func safeErrorMessage(status int) string {
	if status == http.StatusServiceUnavailable {
		return "service unavailable"
	}
	return "internal server error"
}

func validCorrelationID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func newCorrelationID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "generated-correlation-id"
	}
	return hex.EncodeToString(bytes[:])
}
