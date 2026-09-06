package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	errorsmodule "github.com/ThatSoftwareCompany/testing-templatev2/internal/modules/errors"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/errstore"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/httpserver"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

func TestPasswordHashingUsesArgon2id(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatalf("unexpected password hash format: %q", hash)
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("VerifyPassword() rejected the original password")
	}
	if VerifyPassword(hash, password+"!") {
		t.Fatal("VerifyPassword() accepted an incorrect password")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("too-short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
	if err := ValidatePassword(strings.Repeat("a", 129)); err == nil {
		t.Fatal("expected long password to be rejected")
	}
	if err := ValidatePassword("áéíóú password seguro"); err != nil {
		t.Fatalf("expected valid UTF-8 password: %v", err)
	}
}

func TestCSRFTokenIsSignedAndTamperResistant(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	token, err := NewCSRFToken(secret)
	if err != nil {
		t.Fatalf("NewCSRFToken() error = %v", err)
	}
	if !ValidateCSRFToken(secret, token) {
		t.Fatal("valid CSRF token was rejected")
	}
	if ValidateCSRFToken(secret, token+"x") {
		t.Fatal("tampered CSRF token was accepted")
	}
	if ValidateCSRFToken([]byte(strings.Repeat("x", 32)), token) {
		t.Fatal("CSRF token signed with another secret was accepted")
	}
}

func TestAccessTokenValidatesClaimsAndKeyID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	manager := &TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key-1", issuer: "test-issuer", audience: "test-audience"}
	now := time.Now().UTC().Truncate(time.Second)
	raw, err := manager.IssueAccessToken(User{ID: 7, Email: "user@example.com", Roles: []string{"internal_admin"}, Permissions: []string{"errors:read"}}, now, 15*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken() error = %v", err)
	}
	principal, err := manager.ParseAccessToken(raw, now.Add(time.Second))
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}
	if principal.ID != 7 || principal.Email != "user@example.com" || !hasPermission(principal.Permissions, "errors:read") {
		t.Fatalf("unexpected principal: %#v", principal)
	}

	wrongKeyID := *manager
	wrongKeyID.keyID = "key-2"
	if _, err := wrongKeyID.ParseAccessToken(raw, now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected key ID mismatch to fail authentication, got %v", err)
	}
	wrongIssuer := *manager
	wrongIssuer.issuer = "other-issuer"
	if _, err := wrongIssuer.ParseAccessToken(raw, now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected issuer mismatch to fail authentication, got %v", err)
	}
	wrongAudience := *manager
	wrongAudience.audience = "other-audience"
	if _, err := wrongAudience.ParseAccessToken(raw, now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected audience mismatch to fail authentication, got %v", err)
	}
	expired, err := manager.IssueAccessToken(User{ID: 7, Email: "user@example.com"}, now.Add(-2*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken() expired error = %v", err)
	}
	if _, err := manager.ParseAccessToken(expired, now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected expired token to fail authentication, got %v", err)
	}
	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{TokenType: accessTokenType})
	hmacRaw, err := hmacToken.SignedString([]byte("not-an-ed25519-key"))
	if err != nil {
		t.Fatalf("sign invalid algorithm token: %v", err)
	}
	if _, err := manager.ParseAccessToken(hmacRaw, now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected invalid signing algorithm to fail authentication, got %v", err)
	}
}

func TestLoadTokenManagerValidatesPEMKeyPair(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	directory := t.TempDir()
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	privatePath := directory + "/private.pem"
	publicPath := directory + "/public.pem"
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	if _, err := LoadTokenManager(privatePath, publicPath, "key", "issuer", "audience"); err != nil {
		t.Fatalf("LoadTokenManager() error = %v", err)
	}
	otherPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	otherDER, _ := x509.MarshalPKIXPublicKey(otherPublic)
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: otherDER}), 0o600); err != nil {
		t.Fatalf("rewrite public key: %v", err)
	}
	if _, err := LoadTokenManager(privatePath, publicPath, "key", "issuer", "audience"); err == nil {
		t.Fatal("expected mismatched key pair to fail")
	}
}

func TestAuthHTTPFlowUsesCookiesAndCSRF(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	repository := &memoryRepository{user: storedUser{
		User:         User{ID: 1, Email: "user@example.com", Roles: []string{"internal_admin"}, Permissions: []string{"errors:read"}},
		PasswordHash: hash,
		Active:       true,
	}, tokens: make(map[string]refreshToken)}
	service := NewService(
		repository,
		&TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key-1", issuer: "test-issuer", audience: "test-audience"},
		strings.Repeat("c", 32),
		15*time.Minute,
		30*24*time.Hour,
		false,
		http.SameSiteLaxMode,
	)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)
	handler := httpserver.CorrelationID(mux)

	csrfRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	csrfResponse := httptest.NewRecorder()
	handler.ServeHTTP(csrfResponse, csrfRequest)
	if csrfResponse.Code != http.StatusOK {
		t.Fatalf("csrf status = %d", csrfResponse.Code)
	}
	var csrfBody CSRFResponse
	if err := json.Unmarshal(csrfResponse.Body.Bytes(), &csrfBody); err != nil {
		t.Fatalf("decode csrf response: %v", err)
	}
	csrfCookie := csrfResponse.Result().Cookies()[0]
	if !csrfCookie.HttpOnly || csrfCookie.Path != "/" || csrfCookie.Secure || csrfCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("CSRF cookie does not follow the development policy: %#v", csrfCookie)
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"correct horse battery staple"}`))
	loginRequest.Header.Set("X-CSRF-Token", csrfBody.CSRFToken)
	loginRequest.AddCookie(csrfCookie)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	if strings.Contains(loginResponse.Body.String(), "eyJ") {
		t.Fatal("login response exposed a JWT")
	}
	var accessCookie, refreshCookie *http.Cookie
	for _, cookie := range loginResponse.Result().Cookies() {
		switch cookie.Name {
		case accessCookieName:
			accessCookie = cookie
		case refreshCookieName:
			refreshCookie = cookie
		}
	}
	if accessCookie == nil || refreshCookie == nil || !accessCookie.HttpOnly || !refreshCookie.HttpOnly {
		t.Fatalf("session cookies were not issued securely: access=%#v refresh=%#v", accessCookie, refreshCookie)
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(accessCookie)
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK || !strings.Contains(meResponse.Body.String(), "user@example.com") {
		t.Fatalf("unexpected me response: status=%d body=%s", meResponse.Code, meResponse.Body.String())
	}

	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshRequest.AddCookie(refreshCookie)
	refreshRequest.AddCookie(csrfCookie)
	refreshRequest.Header.Set("X-CSRF-Token", csrfBody.CSRFToken)
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", refreshResponse.Code, refreshResponse.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.AddCookie(refreshCookie)
	logoutRequest.AddCookie(csrfCookie)
	logoutRequest.Header.Set("X-CSRF-Token", csrfBody.CSRFToken)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}
	if len(logoutResponse.Result().Cookies()) != 3 {
		t.Fatalf("logout did not clear all session cookies: %#v", logoutResponse.Result().Cookies())
	}
}

func TestAuthCookiesUseProductionPolicy(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(&memoryRepository{}, &TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key", issuer: "issuer", audience: "audience"}, strings.Repeat("c", 32), 15*time.Minute, 30*24*time.Hour, true, http.SameSiteStrictMode)
	cookie := NewController(service).cookie(csrfCookieName, "token", "/", 0, true)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" {
		t.Fatalf("unexpected production cookie policy: %#v", cookie)
	}
}

func TestAuthReturnsServiceUnavailableWithoutDatabase(t *testing.T) {
	service := NewService(nil, nil, "", 15*time.Minute, 30*24*time.Hour, false, http.SameSiteLaxMode)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "service_unavailable") {
		t.Fatalf("unexpected disabled auth response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAuthRejectsMissingCSRF(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	service := NewService(&memoryRepository{}, &TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key", issuer: "issuer", audience: "audience"}, strings.Repeat("c", 32), time.Minute, 30*24*time.Hour, false, http.SameSiteLaxMode)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"correct horse battery staple"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestLoginRateLimitAndEmailNormalization(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	repository := &memoryRepository{user: storedUser{
		User:         User{ID: 1, Email: "user@example.com"},
		PasswordHash: hash,
		Active:       true,
	}}
	service := NewService(repository, &TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key", issuer: "issuer", audience: "audience"}, strings.Repeat("c", 32), time.Minute, 30*24*time.Hour, false, http.SameSiteLaxMode)
	for attempt := 0; attempt < loginLimit; attempt++ {
		_, err := service.Login(context.Background(), LoginRequest{Email: " USER@EXAMPLE.COM ", Password: "wrong password that is long enough"}, "192.0.2.10:1234", time.Now().UTC())
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d error = %v, want invalid credentials", attempt+1, err)
		}
	}
	if _, err := service.Login(context.Background(), LoginRequest{Email: "user@example.com", Password: "wrong password that is long enough"}, "192.0.2.10:1234", time.Now().UTC()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("sixth attempt error = %v, want rate limited", err)
	}
	if NormalizeEmail(" User@Example.COM ") != "user@example.com" {
		t.Fatal("email normalization did not trim and lowercase the address")
	}
}

func TestRefreshRotationReuseAndFamilyRevocation(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	repository := &memoryRepository{user: storedUser{
		User:         User{ID: 1, Email: "user@example.com"},
		PasswordHash: hash,
		Active:       true,
	}, tokens: make(map[string]refreshToken)}
	service := NewService(repository, &TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key", issuer: "issuer", audience: "audience"}, strings.Repeat("c", 32), time.Minute, 30*24*time.Hour, false, http.SameSiteLaxMode)
	now := time.Now().UTC()
	first, err := service.Login(context.Background(), LoginRequest{Email: "user@example.com", Password: "correct horse battery staple"}, "192.0.2.11:1234", now)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	second, err := service.Refresh(context.Background(), first.RefreshToken, now.Add(time.Second))
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if _, err := service.Refresh(context.Background(), first.RefreshToken, now.Add(2*time.Second)); !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("reused refresh error = %v, want reuse detection", err)
	}
	if _, err := service.Refresh(context.Background(), second.RefreshToken, now.Add(3*time.Second)); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("family refresh after reuse error = %v, want invalid token", err)
	}
}

func TestProtectedErrorsRequireExplicitPermission(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(&memoryRepository{}, &TokenManager{privateKey: privateKey, publicKey: publicKey, keyID: "key", issuer: "issuer", audience: "audience"}, strings.Repeat("c", 32), time.Minute, 30*24*time.Hour, false, http.SameSiteLaxMode)
	controller := errorsmodule.NewController(errorsmodule.NewService(errstore.NewNoopStore()))
	handler := RequirePermission(service, "errors:read", http.HandlerFunc(controller.HandleList))

	withoutPermission, err := service.tokens.IssueAccessToken(User{ID: 1, Email: "user@example.com"}, time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatalf("issue token without permission: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/internal/errors", nil)
	request.AddCookie(&http.Cookie{Name: accessCookieName, Value: withoutPermission})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("without permission status = %d, want %d", response.Code, http.StatusForbidden)
	}

	withPermission, err := service.tokens.IssueAccessToken(User{ID: 1, Email: "admin@example.com", Permissions: []string{"errors:read"}}, time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatalf("issue token with permission: %v", err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/errors", nil)
	request.AddCookie(&http.Cookie{Name: accessCookieName, Value: withPermission})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items"`) {
		t.Fatalf("with permission response = %d %s", response.Code, response.Body.String())
	}
}

func TestAuthorizationMiddlewareRequiresAuthenticationRoleAndPermission(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(&memoryRepository{}, &TokenManager{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      "key",
		issuer:     "issuer",
		audience:   "audience",
	}, strings.Repeat("c", 32), time.Minute, 30*24*time.Hour, false, http.SameSiteLaxMode)
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequireRole(service, "example_reader", RequirePermission(service, "example:read", endpoint))

	tests := []struct {
		name        string
		roles       []string
		permissions []string
		wantStatus  int
	}{
		{name: "missing authentication", wantStatus: http.StatusUnauthorized},
		{name: "wrong role", roles: []string{"other_role"}, permissions: []string{"example:read"}, wantStatus: http.StatusForbidden},
		{name: "missing permission", roles: []string{"example_reader"}, wantStatus: http.StatusForbidden},
		{name: "authorized", roles: []string{"example_reader"}, permissions: []string{"example:read"}, wantStatus: http.StatusNoContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/example", nil)
			if test.name != "missing authentication" {
				raw, issueErr := service.tokens.IssueAccessToken(User{
					ID:          1,
					Email:       "user@example.com",
					Roles:       test.roles,
					Permissions: test.permissions,
				}, time.Now().UTC(), time.Minute)
				if issueErr != nil {
					t.Fatalf("issue access token: %v", issueErr)
				}
				request.AddCookie(&http.Cookie{Name: accessCookieName, Value: raw})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

type memoryRepository struct {
	user     storedUser
	failures int
	tokens   map[string]refreshToken
}

func (r *memoryRepository) FindUserByEmail(_ context.Context, email string) (storedUser, error) {
	if r.user.Email != email {
		return storedUser{}, pgx.ErrNoRows
	}
	return r.user, nil
}

func (r *memoryRepository) CreateAdmin(_ context.Context, email, passwordHash string) (User, error) {
	if r.user.Email != "" {
		return User{}, ErrUserExists
	}
	r.user = storedUser{User: User{ID: 1, Email: email}, PasswordHash: passwordHash, Active: true}
	return r.user.User, nil
}

func (r *memoryRepository) IsLoginRateLimited(context.Context, string, time.Time) (bool, error) {
	return r.failures >= loginLimit, nil
}

func (r *memoryRepository) RecordLoginFailure(context.Context, string, time.Time) (bool, error) {
	if r.failures >= loginLimit {
		return true, nil
	}
	r.failures++
	return false, nil
}

func (r *memoryRepository) ClearLoginAttempts(context.Context, string) error {
	r.failures = 0
	return nil
}

func (r *memoryRepository) CreateRefreshToken(_ context.Context, token refreshToken) error {
	if r.tokens == nil {
		r.tokens = make(map[string]refreshToken)
	}
	r.tokens[string(token.Hash)] = token
	return nil
}

func (r *memoryRepository) RotateRefreshToken(_ context.Context, hash []byte, replacement refreshToken, now time.Time) (storedUser, error) {
	token, ok := r.tokens[string(hash)]
	if !ok {
		return storedUser{}, ErrInvalidRefreshToken
	}
	if token.Used {
		for key, candidate := range r.tokens {
			if candidate.FamilyID == token.FamilyID {
				candidate.Revoked = true
				r.tokens[key] = candidate
			}
		}
		return storedUser{}, ErrRefreshReuse
	}
	if token.Revoked || !token.ExpiresAt.After(now) || token.UserID != r.user.ID {
		return storedUser{}, ErrInvalidRefreshToken
	}
	token.Used = true
	token.Revoked = true
	r.tokens[string(hash)] = token
	replacement.FamilyID = token.FamilyID
	replacement.UserID = token.UserID
	r.tokens[string(replacement.Hash)] = replacement
	return r.user, nil
}

func (r *memoryRepository) RevokeRefreshFamilyByToken(_ context.Context, hash []byte, _ time.Time) error {
	token, ok := r.tokens[string(hash)]
	if !ok {
		return nil
	}
	for key, candidate := range r.tokens {
		if candidate.FamilyID == token.FamilyID {
			candidate.Revoked = true
			r.tokens[key] = candidate
		}
	}
	return nil
}

var _ Repository = (*memoryRepository)(nil)
