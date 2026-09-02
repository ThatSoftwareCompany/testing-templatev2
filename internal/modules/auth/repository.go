package auth

import (
	"context"
	"time"
)

type Repository interface {
	FindUserByEmail(context.Context, string) (storedUser, error)
	CreateAdmin(context.Context, string, string) (User, error)
	IsLoginRateLimited(context.Context, string, time.Time) (bool, error)
	RecordLoginFailure(context.Context, string, time.Time) (bool, error)
	ClearLoginAttempts(context.Context, string) error
	CreateRefreshToken(context.Context, refreshToken) error
	RotateRefreshToken(context.Context, []byte, refreshToken, time.Time) (storedUser, error)
	RevokeRefreshFamilyByToken(context.Context, []byte, time.Time) error
}
