package auth

import "errors"

var (
	ErrUnavailable         = errors.New("authentication unavailable")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrRateLimited         = errors.New("login rate limited")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshReuse        = errors.New("refresh token reuse detected")
	ErrInvalidCSRF         = errors.New("invalid csrf token")
	ErrUnauthenticated     = errors.New("authentication required")
	ErrForbidden           = errors.New("permission denied")
	ErrUserExists          = errors.New("user already exists")
)
