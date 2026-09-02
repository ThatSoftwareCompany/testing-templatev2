package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	adminRole      = "internal_admin"
	errorReadGrant = "errors:read"
	loginLimit     = 5
	loginWindow    = 15 * time.Minute
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) FindUserByEmail(ctx context.Context, email string) (storedUser, error) {
	return findUser(ctx, r.pool, email)
}

func (r *PostgresRepository) CreateAdmin(ctx context.Context, email, passwordHash string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin admin creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var roleID, permissionID, userID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO roles (slug) VALUES ($1)
		ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
		RETURNING id`, adminRole).Scan(&roleID); err != nil {
		return User{}, fmt.Errorf("ensure admin role: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO permissions (name) VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, errorReadGrant).Scan(&permissionID); err != nil {
		return User{}, fmt.Errorf("ensure error permission: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, active, created_at, updated_at)
		VALUES ($1, $2, true, $3, $3)
		ON CONFLICT (email) DO NOTHING
		RETURNING id`, email, passwordHash, time.Now().UTC()).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserExists
		}
		return User{}, fmt.Errorf("create admin user: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
		return User{}, fmt.Errorf("assign admin role: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, roleID, permissionID); err != nil {
		return User{}, fmt.Errorf("assign admin permission: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit admin creation: %w", err)
	}
	return User{ID: userID, Email: email, Roles: []string{adminRole}, Permissions: []string{errorReadGrant}}, nil
}

func (r *PostgresRepository) IsLoginRateLimited(ctx context.Context, key string, now time.Time) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM login_attempts
		WHERE rate_key = $1 AND succeeded = false AND attempted_at > $2`, key, now.Add(-loginWindow)).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check login rate limit: %w", err)
	}
	return count >= loginLimit, nil
}

func (r *PostgresRepository) RecordLoginFailure(ctx context.Context, key string, now time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin login rate check: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize failure recording for the same IP/email pair so concurrent API
	// instances cannot insert more than the configured number of failures.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return false, fmt.Errorf("lock login rate key: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM login_attempts WHERE rate_key = $1 AND attempted_at <= $2`, key, now.Add(-loginWindow)); err != nil {
		return false, fmt.Errorf("purge expired login attempts: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM login_attempts
		WHERE rate_key = $1 AND succeeded = false AND attempted_at > $2`, key, now.Add(-loginWindow)).Scan(&count); err != nil {
		return false, fmt.Errorf("check login failure limit: %w", err)
	}
	if count >= loginLimit {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit login rate limit: %w", err)
		}
		return true, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO login_attempts (rate_key, attempted_at, succeeded)
		VALUES ($1, $2, false)`, key, now); err != nil {
		return false, fmt.Errorf("record login failure: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit login attempt: %w", err)
	}
	return false, nil
}

func (r *PostgresRepository) ClearLoginAttempts(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM login_attempts WHERE rate_key = $1`, key)
	if err != nil {
		return fmt.Errorf("clear login attempts: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateRefreshToken(ctx context.Context, token refreshToken) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`, token.UserID, token.Hash, token.FamilyID, token.ExpiresAt, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RotateRefreshToken(ctx context.Context, hash []byte, replacement refreshToken, now time.Time) (storedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return storedUser{}, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		id, userID int64
		familyID   string
		expiresAt  time.Time
		usedAt     pgtype.Timestamptz
		revokedAt  pgtype.Timestamptz
		active     bool
	)
	err = tx.QueryRow(ctx, `
		SELECT rt.id, rt.user_id, rt.family_id, rt.expires_at, rt.used_at, rt.revoked_at, u.active
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1
		FOR UPDATE`, hash).Scan(&id, &userID, &familyID, &expiresAt, &usedAt, &revokedAt, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedUser{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return storedUser{}, fmt.Errorf("read refresh token: %w", err)
	}
	if usedAt.Valid {
		if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, $2) WHERE family_id = $1`, familyID, now); err != nil {
			return storedUser{}, fmt.Errorf("revoke reused refresh family: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return storedUser{}, fmt.Errorf("commit refresh reuse revocation: %w", err)
		}
		return storedUser{}, ErrRefreshReuse
	}
	if revokedAt.Valid || !expiresAt.After(now) || !active {
		return storedUser{}, ErrInvalidRefreshToken
	}
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens SET used_at = $2, revoked_at = $2 WHERE id = $1`, id, now); err != nil {
		return storedUser{}, fmt.Errorf("mark refresh token used: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`, userID, replacement.Hash, familyID, replacement.ExpiresAt, now); err != nil {
		return storedUser{}, fmt.Errorf("create rotated refresh token: %w", err)
	}
	user, err := findUserByID(ctx, tx, userID)
	if err != nil {
		return storedUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return storedUser{}, fmt.Errorf("commit refresh rotation: %w", err)
	}
	return user, nil
}

func (r *PostgresRepository) RevokeRefreshFamilyByToken(ctx context.Context, hash []byte, now time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE family_id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1)`, hash, now)
	if err != nil {
		return fmt.Errorf("revoke refresh family: %w", err)
	}
	return nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func findUser(ctx context.Context, db queryer, email string) (storedUser, error) {
	var user storedUser
	err := db.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.active,
		       COALESCE(array_agg(DISTINCT r.slug) FILTER (WHERE r.slug IS NOT NULL), '{}'),
		       COALESCE(array_agg(DISTINCT p.name) FILTER (WHERE p.name IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN permissions p ON p.id = rp.permission_id
		WHERE u.email = $1
		GROUP BY u.id, u.email, u.password_hash, u.active`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Active, &user.Roles, &user.Permissions)
	return user, err
}

func findUserByID(ctx context.Context, db queryer, id int64) (storedUser, error) {
	var user storedUser
	err := db.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.active,
		       COALESCE(array_agg(DISTINCT r.slug) FILTER (WHERE r.slug IS NOT NULL), '{}'),
		       COALESCE(array_agg(DISTINCT p.name) FILTER (WHERE p.name IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN permissions p ON p.id = rp.permission_id
		WHERE u.id = $1
		GROUP BY u.id, u.email, u.password_hash, u.active`, id).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Active, &user.Roles, &user.Permissions)
	return user, err
}
