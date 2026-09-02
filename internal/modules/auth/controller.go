package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/httpserver"
)

const (
	accessCookieName  = "tsc_access_token"
	refreshCookieName = "tsc_refresh_token"
	csrfCookieName    = "tsc_csrf_token"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) HandleCSRF(w http.ResponseWriter, r *http.Request) {
	if !c.service.Available() {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
		return
	}
	token, err := c.service.NewCSRFToken()
	if err != nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
		return
	}
	http.SetCookie(w, c.cookie(csrfCookieName, token, "/", 0, true))
	httpserver.WriteJSON(w, http.StatusOK, CSRFResponse{CSRFToken: token})
}

func (c *Controller) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if !c.service.Available() {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
		return
	}
	if !c.validCSRF(w, r) {
		return
	}
	var request LoginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	result, err := c.service.Login(r.Context(), request, r.RemoteAddr, time.Now().UTC())
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	c.setSessionCookies(w, result)
	httpserver.WriteJSON(w, http.StatusOK, AuthResponse{User: result.User})
}

func (c *Controller) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if !c.service.Available() {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
		return
	}
	if !c.validCSRF(w, r) {
		return
	}
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		writeAuthError(w, r, ErrInvalidRefreshToken)
		return
	}
	result, err := c.service.Refresh(r.Context(), cookie.Value, time.Now().UTC())
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	c.setSessionCookies(w, result)
	httpserver.WriteJSON(w, http.StatusOK, AuthResponse{User: result.User})
}

func (c *Controller) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if !c.service.Available() {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
		return
	}
	if !c.validCSRF(w, r) {
		return
	}
	var logoutErr error
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		logoutErr = c.service.Logout(r.Context(), cookie.Value, time.Now().UTC())
	}
	c.clearCookies(w)
	if logoutErr != nil {
		writeAuthError(w, r, logoutErr)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) HandleMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		httpserver.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, AuthResponse{User: principal.User})
}

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	controller := NewController(service)
	mux.Handle("/api/v1/auth/csrf", httpserver.OnlyMethods(http.MethodGet)(http.HandlerFunc(controller.HandleCSRF)))
	mux.Handle("/api/v1/auth/login", httpserver.OnlyMethods(http.MethodPost)(http.HandlerFunc(controller.HandleLogin)))
	mux.Handle("/api/v1/auth/refresh", httpserver.OnlyMethods(http.MethodPost)(http.HandlerFunc(controller.HandleRefresh)))
	mux.Handle("/api/v1/auth/logout", httpserver.OnlyMethods(http.MethodPost)(http.HandlerFunc(controller.HandleLogout)))
	mux.Handle("/api/v1/auth/me", httpserver.OnlyMethods(http.MethodGet)(RequireAuthentication(service, http.HandlerFunc(controller.HandleMe))))
}

func (c *Controller) validCSRF(w http.ResponseWriter, r *http.Request) bool {
	cookie, cookieErr := r.Cookie(csrfCookieName)
	header := r.Header.Get("X-CSRF-Token")
	if cookieErr != nil || header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 || !c.service.ValidateCSRFToken(header) {
		httpserver.WriteError(w, r, http.StatusForbidden, "csrf_failed", "csrf validation failed")
		return false
	}
	return true
}

func (c *Controller) setSessionCookies(w http.ResponseWriter, result AuthResult) {
	http.SetCookie(w, c.cookie(accessCookieName, result.AccessToken, "/", int(c.service.accessTokenTTL.Seconds()), true))
	http.SetCookie(w, c.cookie(refreshCookieName, result.RefreshToken, "/api/v1/auth", int(c.service.refreshTokenTTL.Seconds()), true))
}

func (c *Controller) clearCookies(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie(accessCookieName, "", "/", -1, true))
	http.SetCookie(w, c.cookie(refreshCookieName, "", "/api/v1/auth", -1, true))
	http.SetCookie(w, c.cookie(csrfCookieName, "", "/", -1, true))
}

func (c *Controller) cookie(name, value, path string, maxAge int, httpOnly bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: httpOnly,
		Secure:   c.service.cookieSecure,
		SameSite: c.service.cookieSameSite,
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request contains multiple JSON values")
	}
	return nil
}

func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		httpserver.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
	case errors.Is(err, ErrRateLimited):
		httpserver.WriteError(w, r, http.StatusTooManyRequests, "too_many_requests", "too many requests")
	case errors.Is(err, ErrInvalidRefreshToken), errors.Is(err, ErrRefreshReuse), errors.Is(err, ErrUnauthenticated):
		httpserver.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
	case errors.Is(err, ErrInvalidCSRF):
		httpserver.WriteError(w, r, http.StatusForbidden, "csrf_failed", "csrf validation failed")
	case errors.Is(err, ErrUnavailable):
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
	default:
		httpserver.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
