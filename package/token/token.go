// Package token issues and verifies short-lived JWT access tokens (HS256).
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "hearable"

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	UserID string
	Role   string
}

type Manager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (m *Manager) TTL() time.Duration { return m.ttl }

type jwtClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func (m *Manager) Issue(userID, role string) (string, error) {
	now := m.now()
	claims := jwtClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

func (m *Manager) Verify(raw string) (Claims, error) {
	var claims jwtClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: claims.Subject, Role: claims.Role}, nil
}
