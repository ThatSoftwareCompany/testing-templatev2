package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenType     = "access"
	accessTokenIDLength = 16
)

type TokenManager struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
	issuer     string
	audience   string
}

func LoadTokenManager(privatePath, publicPath, keyID, issuer, audience string) (*TokenManager, error) {
	privatePEM, err := os.ReadFile(privatePath)
	if err != nil {
		return nil, fmt.Errorf("read auth private key: %w", err)
	}
	privateBlock, _ := pem.Decode(privatePEM)
	if privateBlock == nil {
		return nil, fmt.Errorf("decode auth private key PEM")
	}
	privateAny, err := x509.ParsePKCS8PrivateKey(privateBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse auth private key: %w", err)
	}
	privateKey, ok := privateAny.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("auth private key is not Ed25519")
	}

	publicPEM, err := os.ReadFile(publicPath)
	if err != nil {
		return nil, fmt.Errorf("read auth public key: %w", err)
	}
	publicBlock, _ := pem.Decode(publicPEM)
	if publicBlock == nil {
		return nil, fmt.Errorf("decode auth public key PEM")
	}
	publicAny, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse auth public key: %w", err)
	}
	publicKey, ok := publicAny.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("auth public key is not Ed25519")
	}
	if !bytes.Equal(publicKey, privateKey.Public().(ed25519.PublicKey)) {
		return nil, fmt.Errorf("auth public key does not match private key")
	}

	return &TokenManager{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      keyID,
		issuer:     issuer,
		audience:   audience,
	}, nil
}

type accessClaims struct {
	TokenType   string   `json:"token_type"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

func (m *TokenManager) IssueAccessToken(user User, now time.Time, ttl time.Duration) (string, error) {
	tokenID := make([]byte, accessTokenIDLength)
	if _, err := rand.Read(tokenID); err != nil {
		return "", fmt.Errorf("generate access token id: %w", err)
	}
	claims := accessClaims{
		TokenType:   accessTokenType,
		Email:       user.Email,
		Roles:       append([]string(nil), user.Roles...),
		Permissions: append([]string(nil), user.Permissions...),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatInt(user.ID, 10),
			Audience:  jwt.ClaimStrings{m.audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        base64.RawURLEncoding.EncodeToString(tokenID),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = m.keyID
	signed, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

func (m *TokenManager) ParseAccessToken(raw string, now time.Time) (Principal, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method == nil || token.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, ErrUnauthenticated
		}
		if token.Header["kid"] != m.keyID {
			return nil, ErrUnauthenticated
		}
		return m.publicKey, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || token == nil || !token.Valid || claims.TokenType != accessTokenType {
		return Principal{}, ErrUnauthenticated
	}
	if claims.NotBefore == nil || claims.NotBefore.After(now) || claims.Subject == "" || claims.Email == "" {
		return Principal{}, ErrUnauthenticated
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{
		User: User{
			ID:          userID,
			Email:       claims.Email,
			Roles:       append([]string(nil), claims.Roles...),
			Permissions: append([]string(nil), claims.Permissions...),
		},
		TokenID: claims.ID,
	}, nil
}
