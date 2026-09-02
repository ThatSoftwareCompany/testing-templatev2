package auth

import "time"

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type User struct {
	ID          int64    `json:"id"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type AuthResponse struct {
	User User `json:"user"`
}

type CSRFResponse struct {
	CSRFToken string `json:"csrf_token"`
}

type Principal struct {
	User
	TokenID string
}

type storedUser struct {
	User
	PasswordHash string
	Active       bool
}

type refreshToken struct {
	Hash      []byte
	FamilyID  string
	UserID    int64
	ExpiresAt time.Time
	Used      bool
	Revoked   bool
}

type loginAttempt struct {
	Key         string
	AttemptedAt time.Time
}
