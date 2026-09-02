//go:build integration

package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	platformmigrate "github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAuthenticationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	migrationConfig := platformmigrate.Config{DatabaseURL: databaseURL, MigrationsDir: "file://../../../migrations"}
	if err := platformmigrate.Up(migrationConfig); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := platformmigrate.Up(migrationConfig); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	version, dirty, err := platformmigrate.Version(migrationConfig)
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 2 || dirty {
		t.Fatalf("unexpected migration state: version=%d dirty=%t", version, dirty)
	}
	t.Cleanup(func() {
		if err := platformmigrate.Down(migrationConfig, 1); err != nil {
			t.Errorf("rollback authentication migration: %v", err)
		}
	})

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()
	repository := NewPostgresRepository(pool)
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := repository.CreateAdmin(context.Background(), "admin@example.com", hash)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if user.Roles[0] != adminRole || user.Permissions[0] != errorReadGrant {
		t.Fatalf("unexpected admin authorization: %#v", user)
	}
	stored, err := repository.FindUserByEmail(context.Background(), user.Email)
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}
	if !stored.Active || !VerifyPassword(stored.PasswordHash, "correct horse battery staple") || !hasPermission(stored.Permissions, errorReadGrant) {
		t.Fatalf("stored user was not persisted correctly: %#v", stored)
	}

	now := time.Now().UTC()
	key := loginRateKey("127.0.0.1:1234", "admin@example.com")
	for attempt := 0; attempt < loginLimit; attempt++ {
		rateLimited, err := repository.RecordLoginFailure(context.Background(), key, now)
		if err != nil {
			t.Fatalf("record login failure: %v", err)
		}
		if rateLimited {
			t.Fatalf("attempt %d was unexpectedly rate limited", attempt+1)
		}
	}
	rateLimited, err := repository.IsLoginRateLimited(context.Background(), key, now)
	if err != nil || !rateLimited {
		t.Fatalf("expected login rate limit, limited=%t err=%v", rateLimited, err)
	}
	if err := repository.ClearLoginAttempts(context.Background(), key); err != nil {
		t.Fatalf("clear login attempts: %v", err)
	}

	firstRaw, err := randomOpaqueToken()
	if err != nil {
		t.Fatalf("generate first refresh token: %v", err)
	}
	first := refreshToken{Hash: hashRefreshToken(firstRaw), FamilyID: "integration-family", UserID: user.ID, ExpiresAt: now.Add(24 * time.Hour)}
	if err := repository.CreateRefreshToken(context.Background(), first); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	var rawTokenRows int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = $1`, []byte(firstRaw)).Scan(&rawTokenRows); err != nil {
		t.Fatalf("check raw refresh token storage: %v", err)
	}
	if rawTokenRows != 0 {
		t.Fatal("raw refresh token was stored instead of its hash")
	}
	secondRaw, err := randomOpaqueToken()
	if err != nil {
		t.Fatalf("generate second refresh token: %v", err)
	}
	rotated, err := repository.RotateRefreshToken(context.Background(), first.Hash, refreshToken{Hash: hashRefreshToken(secondRaw), ExpiresAt: now.Add(24 * time.Hour)}, now)
	if err != nil || rotated.ID != user.ID {
		t.Fatalf("rotate refresh token: user=%#v err=%v", rotated, err)
	}
	if _, err := repository.RotateRefreshToken(context.Background(), first.Hash, refreshToken{Hash: hashRefreshToken("unused"), ExpiresAt: now.Add(24 * time.Hour)}, now); !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("expected refresh reuse detection, got %v", err)
	}
	if err := repository.RevokeRefreshFamilyByToken(context.Background(), hashRefreshToken(secondRaw), now); err != nil {
		t.Fatalf("revoke refresh family: %v", err)
	}
}
