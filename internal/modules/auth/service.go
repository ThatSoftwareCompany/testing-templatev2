package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Service struct {
	repository      Repository
	tokens          *TokenManager
	csrfSecret      []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	cookieSecure    bool
	cookieSameSite  http.SameSite
}

type AuthResult struct {
	User         User
	AccessToken  string
	RefreshToken string
}

func NewService(repository Repository, tokens *TokenManager, csrfSecret string, accessTokenTTL, refreshTokenTTL time.Duration, cookieSecure bool, cookieSameSite http.SameSite) *Service {
	return &Service{
		repository:      repository,
		tokens:          tokens,
		csrfSecret:      []byte(csrfSecret),
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
		cookieSecure:    cookieSecure,
		cookieSameSite:  cookieSameSite,
	}
}

func NewProvisioner(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Available() bool {
	return s != nil && s.repository != nil && s.tokens != nil && len(s.csrfSecret) >= 32
}

func (s *Service) Login(ctx context.Context, request LoginRequest, remoteAddr string, now time.Time) (AuthResult, error) {
	if !s.Available() {
		return AuthResult{}, ErrUnavailable
	}
	email := NormalizeEmail(request.Email)
	if email == "" {
		return AuthResult{}, ErrInvalidCredentials
	}
	rateKey := loginRateKey(remoteAddr, email)
	rateLimited, err := s.repository.IsLoginRateLimited(ctx, rateKey, now)
	if err != nil {
		return AuthResult{}, ErrUnavailable
	}
	if rateLimited {
		return AuthResult{}, ErrRateLimited
	}

	user, err := s.repository.FindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!user.Active || !VerifyPassword(user.PasswordHash, request.Password))) {
		rateLimited, recordErr := s.repository.RecordLoginFailure(ctx, rateKey, now)
		if recordErr != nil {
			return AuthResult{}, ErrUnavailable
		}
		if rateLimited {
			return AuthResult{}, ErrRateLimited
		}
		return AuthResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthResult{}, ErrUnavailable
	}
	if err := s.repository.ClearLoginAttempts(ctx, rateKey); err != nil {
		return AuthResult{}, ErrUnavailable
	}

	return s.issueTokens(ctx, user.User, now)
}

func (s *Service) Refresh(ctx context.Context, rawToken string, now time.Time) (AuthResult, error) {
	if !s.Available() || rawToken == "" {
		if !s.Available() {
			return AuthResult{}, ErrUnavailable
		}
		return AuthResult{}, ErrInvalidRefreshToken
	}
	newRawToken, err := randomOpaqueToken()
	if err != nil {
		return AuthResult{}, ErrUnavailable
	}
	user, err := s.repository.RotateRefreshToken(ctx, hashRefreshToken(rawToken), refreshToken{
		Hash:      hashRefreshToken(newRawToken),
		ExpiresAt: now.Add(s.refreshTokenTTL),
	}, now)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) || errors.Is(err, ErrRefreshReuse) {
			return AuthResult{}, err
		}
		return AuthResult{}, ErrUnavailable
	}
	return s.issueTokensWithRefresh(user.User, newRawToken, now)
}

func (s *Service) Logout(ctx context.Context, rawToken string, now time.Time) error {
	if !s.Available() {
		return ErrUnavailable
	}
	if rawToken == "" {
		return nil
	}
	if err := s.repository.RevokeRefreshFamilyByToken(ctx, hashRefreshToken(rawToken), now); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) Authenticate(rawToken string, now time.Time) (Principal, error) {
	if !s.Available() {
		return Principal{}, ErrUnavailable
	}
	return s.tokens.ParseAccessToken(rawToken, now)
}

func (s *Service) NewCSRFToken() (string, error) {
	if !s.Available() {
		return "", ErrUnavailable
	}
	return NewCSRFToken(s.csrfSecret)
}

func (s *Service) ValidateCSRFToken(token string) bool {
	return s != nil && ValidateCSRFToken(s.csrfSecret, token)
}

func (s *Service) CreateAdmin(ctx context.Context, email, password string) (User, error) {
	if s == nil || s.repository == nil {
		return User{}, ErrUnavailable
	}
	normalizedEmail := NormalizeEmail(email)
	if !validEmail(normalizedEmail) {
		return User{}, fmt.Errorf("invalid email")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	return s.repository.CreateAdmin(ctx, normalizedEmail, hash)
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Service) issueTokens(ctx context.Context, user User, now time.Time) (AuthResult, error) {
	rawRefreshToken, err := randomOpaqueToken()
	if err != nil {
		return AuthResult{}, ErrUnavailable
	}
	familyID, err := randomOpaqueToken()
	if err != nil {
		return AuthResult{}, ErrUnavailable
	}
	if err := s.repository.CreateRefreshToken(ctx, refreshToken{
		Hash:      hashRefreshToken(rawRefreshToken),
		FamilyID:  familyID,
		UserID:    user.ID,
		ExpiresAt: now.Add(s.refreshTokenTTL),
	}); err != nil {
		return AuthResult{}, ErrUnavailable
	}
	return s.issueTokensWithRefresh(user, rawRefreshToken, now)
}

func (s *Service) issueTokensWithRefresh(user User, rawRefreshToken string, now time.Time) (AuthResult, error) {
	accessToken, err := s.tokens.IssueAccessToken(user, now, s.accessTokenTTL)
	if err != nil {
		return AuthResult{}, ErrUnavailable
	}
	return AuthResult{User: user, AccessToken: accessToken, RefreshToken: rawRefreshToken}, nil
}

func randomOpaqueToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashRefreshToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func loginRateKey(remoteAddr, email string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil || host == "" {
		host = remoteAddr
	}
	// Store only a stable digest instead of the raw IP/email pair. This avoids
	// retaining login identifiers and produces a PostgreSQL-safe advisory key.
	digest := sha256.Sum256([]byte(host + "\x00" + email))
	return hex.EncodeToString(digest[:])
}

func validEmail(email string) bool {
	if email == "" || len(email) > 320 || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	parts := strings.Split(email, "@")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && strings.Contains(parts[1], ".")
}
