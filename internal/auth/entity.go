package auth

import (
	"errors"
	"time"
)

const (
	RoleUser      = "user"
	RoleVolunteer = "volunteer"
)

var (
	ErrEmailTaken          = errors.New("email already registered")
	ErrNotFound            = errors.New("not found")
	ErrRefreshTokenInvalid = errors.New("refresh token invalid")
	ErrRefreshTokenReused  = errors.New("refresh token reused")
)

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`

	PasswordHash string `json:"-"`
}

type RegisterRequest struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	AcceptTerms bool   `json:"accept_terms"`
	// Role is optional; empty means a regular user. Volunteers register from
	// the web app to contribute modules.
	Role string `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	User         User   `json:"user"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}
