package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const csrfRandomLength = 32

func NewCSRFToken(secret []byte) (string, error) {
	if len(secret) < 32 {
		return "", ErrUnavailable
	}
	randomPart := make([]byte, csrfRandomLength)
	if _, err := rand.Read(randomPart); err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(randomPart)
	return value + "." + signCSRF(secret, value), nil
}

func ValidateCSRFToken(secret []byte, token string) bool {
	if len(secret) < 32 {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	randomPart, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(randomPart) != csrfRandomLength {
		return false
	}
	expected := signCSRF(secret, parts[0])
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

func signCSRF(secret []byte, value string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
