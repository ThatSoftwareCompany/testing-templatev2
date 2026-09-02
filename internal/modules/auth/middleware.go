package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/httpserver"
)

type principalContextKey struct{}

func RequireAuthentication(service *Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil || !service.Available() {
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
			return
		}
		cookie, err := r.Cookie(accessCookieName)
		if err != nil || cookie.Value == "" {
			httpserver.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
			return
		}
		principal, err := service.Authenticate(cookie.Value, time.Now().UTC())
		if err != nil {
			if errors.Is(err, ErrUnavailable) {
				httpserver.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
				return
			}
			httpserver.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequirePermission(service *Service, permission string, next http.Handler) http.Handler {
	return RequireAuthentication(service, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || !hasPermission(principal.Permissions, permission) {
			httpserver.WriteError(w, r, http.StatusForbidden, "forbidden", "permission denied")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireRole enforces an explicit role after authenticating the request.
// Roles do not grant access implicitly; product routes should normally pair
// this check with RequirePermission when both dimensions are part of the
// authorization contract.
func RequireRole(service *Service, role string, next http.Handler) http.Handler {
	return RequireAuthentication(service, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || !hasRole(principal.Roles, role) {
			httpserver.WriteError(w, r, http.StatusForbidden, "forbidden", "permission denied")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func hasPermission(permissions []string, wanted string) bool {
	for _, permission := range permissions {
		if permission == wanted {
			return true
		}
	}
	return false
}

func hasRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
