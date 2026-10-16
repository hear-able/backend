package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	apperrors "hearable/backend/package/errors"
)

const (
	minPasswordLength = 8
	maxPasswordBytes  = 72 // bcrypt ignores anything beyond 72 bytes
	maxNameLength     = 100
)

type repository interface {
	CreateUser(ctx context.Context, name, email, passwordHash, role string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	GetUserByID(ctx context.Context, id string) (User, error)
	InsertRefreshToken(ctx context.Context, userID string, tokenHash []byte, expiresAt time.Time) error
	RotateRefreshToken(ctx context.Context, oldHash, newHash []byte, newExpiresAt, now time.Time) (userID, role string, err error)
	RevokeRefreshToken(ctx context.Context, userID string, tokenHash []byte) error
}

type accessTokenIssuer interface {
	Issue(userID, role string) (string, error)
	TTL() time.Duration
}

type Service struct {
	repo       repository
	tokens     accessTokenIssuer
	refreshTTL time.Duration
	bcryptCost int
	now        func() time.Time

	// dummyHash is compared against when the email is unknown so that login
	// takes the same time whether or not the account exists.
	dummyHash []byte
}

func NewService(repo repository, tokens accessTokenIssuer, refreshTTL time.Duration) *Service {
	return newService(repo, tokens, refreshTTL, bcrypt.DefaultCost)
}

func newService(repo repository, tokens accessTokenIssuer, refreshTTL time.Duration, cost int) *Service {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("hearable-dummy-password"), cost)
	return &Service{
		repo:       repo,
		tokens:     tokens,
		refreshTTL: refreshTTL,
		bcryptCost: cost,
		now:        time.Now,
		dummyHash:  dummy,
	}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (AuthResponse, error) {
	name := strings.TrimSpace(req.Name)
	email := normalizeEmail(req.Email)
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = RoleUser
	}

	switch {
	case role != RoleUser && role != RoleVolunteer:
		return AuthResponse{}, apperrors.Unprocessable("role harus bernilai user atau volunteer.")
	case name == "":
		return AuthResponse{}, apperrors.Unprocessable("Nama panggilan wajib diisi.")
	case utf8.RuneCountInString(name) > maxNameLength:
		return AuthResponse{}, apperrors.Unprocessable("Nama panggilan maksimal 100 karakter.")
	case !validEmail(email):
		return AuthResponse{}, apperrors.Unprocessable("Format email tidak valid.")
	case utf8.RuneCountInString(req.Password) < minPasswordLength:
		return AuthResponse{}, apperrors.Unprocessable("Kata sandi minimal 8 karakter.")
	case len(req.Password) > maxPasswordBytes:
		return AuthResponse{}, apperrors.Unprocessable("Kata sandi terlalu panjang (maksimal 72 byte).")
	case !req.AcceptTerms:
		return AuthResponse{}, apperrors.Unprocessable("Anda harus menyetujui Ketentuan Layanan dan Kebijakan Privasi.")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, name, email, string(hash), role)
	if errors.Is(err, ErrEmailTaken) {
		return AuthResponse{}, apperrors.Conflict("Email sudah terdaftar.")
	}
	if err != nil {
		return AuthResponse{}, err
	}

	return s.issueTokens(ctx, user)
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (AuthResponse, error) {
	invalid := apperrors.Unauthorized("Email atau kata sandi salah.")
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		return AuthResponse{}, invalid
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(req.Password))
		return AuthResponse{}, invalid
	}
	if err != nil {
		return AuthResponse{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return AuthResponse{}, invalid
	}

	return s.issueTokens(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (AuthResponse, error) {
	if rawRefreshToken == "" {
		return AuthResponse{}, apperrors.Unauthorized("Refresh token tidak valid.")
	}

	newRaw, newHash, err := newRefreshToken()
	if err != nil {
		return AuthResponse{}, err
	}

	now := s.now()
	userID, _, err := s.repo.RotateRefreshToken(ctx, hashToken(rawRefreshToken), newHash, now.Add(s.refreshTTL), now)
	if errors.Is(err, ErrRefreshTokenInvalid) || errors.Is(err, ErrRefreshTokenReused) {
		return AuthResponse{}, apperrors.Unauthorized("Refresh token tidak valid.")
	}
	if err != nil {
		return AuthResponse{}, err
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return AuthResponse{}, err
	}

	access, err := s.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return AuthResponse{}, err
	}
	return s.response(user, access, newRaw), nil
}

// Logout revokes the given refresh token if it belongs to userID. Unknown
// tokens are ignored so logout is idempotent.
func (s *Service) Logout(ctx context.Context, userID, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return apperrors.Unprocessable("refresh_token wajib diisi.")
	}
	return s.repo.RevokeRefreshToken(ctx, userID, hashToken(rawRefreshToken))
}

func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return User{}, apperrors.Unauthorized("Akun tidak ditemukan.")
	}
	return user, err
}

func (s *Service) issueTokens(ctx context.Context, user User) (AuthResponse, error) {
	access, err := s.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return AuthResponse{}, err
	}
	raw, hash, err := newRefreshToken()
	if err != nil {
		return AuthResponse{}, err
	}
	if err := s.repo.InsertRefreshToken(ctx, user.ID, hash, s.now().Add(s.refreshTTL)); err != nil {
		return AuthResponse{}, err
	}
	return s.response(user, access, raw), nil
}

func (s *Service) response(user User, access, refresh string) AuthResponse {
	return AuthResponse{
		User:         user,
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.tokens.TTL().Seconds()),
	}
}

// newRefreshToken returns a random opaque token and its SHA-256 hash. Only
// the hash is stored; the token has 256 bits of entropy so a fast hash is
// sufficient.
func newRefreshToken() (raw string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	if len(email) > 255 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && strings.Contains(email[strings.LastIndex(email, "@"):], ".")
}
