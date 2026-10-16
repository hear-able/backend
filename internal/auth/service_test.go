package auth

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/token"
)

type fakeRepo struct {
	users       map[string]User
	tokens      map[string][]byte // userID -> latest hash
	rotateErr   error
	revokedHash []byte
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[string]User{}, tokens: map[string][]byte{}}
}

func (f *fakeRepo) CreateUser(_ context.Context, name, email, hash string) (User, error) {
	for _, u := range f.users {
		if u.Email == email {
			return User{}, ErrEmailTaken
		}
	}
	u := User{ID: "aaaaaaaa-0000-0000-0000-00000000000" + string(rune('1'+len(f.users))), Name: name, Email: email, Role: RoleUser, PasswordHash: hash}
	f.users[u.ID] = u
	return u, nil
}
func (f *fakeRepo) GetUserByEmail(_ context.Context, email string) (User, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}
func (f *fakeRepo) GetUserByID(_ context.Context, id string) (User, error) {
	u, ok := f.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (f *fakeRepo) InsertRefreshToken(_ context.Context, userID string, hash []byte, _ time.Time) error {
	f.tokens[userID] = hash
	return nil
}
func (f *fakeRepo) RotateRefreshToken(_ context.Context, oldHash, newHash []byte, _, _ time.Time) (string, string, error) {
	if f.rotateErr != nil {
		return "", "", f.rotateErr
	}
	for uid, h := range f.tokens {
		if bytes.Equal(h, oldHash) {
			f.tokens[uid] = newHash
			return uid, RoleUser, nil
		}
	}
	return "", "", ErrRefreshTokenInvalid
}
func (f *fakeRepo) RevokeRefreshToken(_ context.Context, _ string, hash []byte) error {
	f.revokedHash = hash
	return nil
}

func newTestService(repo *fakeRepo) *Service {
	return newService(repo, token.NewManager("0123456789abcdef0123456789abcdef", time.Minute), time.Hour, bcrypt.MinCost)
}

func validRegister() RegisterRequest {
	return RegisterRequest{Name: " Rian ", Email: " Rian@Example.com ", Password: "rahasia123", AcceptTerms: true}
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if appErr, ok := apperrors.As(err); !ok || appErr.Status != status {
		t.Fatalf("want %d, got %v", status, err)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	resp, err := svc.Register(context.Background(), validRegister())
	if err != nil {
		t.Fatal(err)
	}
	if resp.User.Name != "Rian" || resp.User.Email != "rian@example.com" || resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("resp %+v", resp)
	}
	stored := repo.users[resp.User.ID]
	if stored.PasswordHash == "rahasia123" || bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("rahasia123")) != nil {
		t.Fatal("password must be stored as bcrypt hash")
	}
	if bytes.Equal(repo.tokens[resp.User.ID], []byte(resp.RefreshToken)) {
		t.Fatal("refresh token must be stored hashed")
	}

	_, err = svc.Register(context.Background(), validRegister())
	wantStatus(t, err, http.StatusConflict)

	if _, err := svc.Login(context.Background(), LoginRequest{Email: "RIAN@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	_, err = svc.Login(context.Background(), LoginRequest{Email: "rian@example.com", Password: "salah-sekali"})
	wantStatus(t, err, http.StatusUnauthorized)
	_, err = svc.Login(context.Background(), LoginRequest{Email: "nobody@example.com", Password: "rahasia123"})
	wantStatus(t, err, http.StatusUnauthorized)
}

func TestRegisterValidation(t *testing.T) {
	mutations := map[string]func(*RegisterRequest){
		"no name":      func(r *RegisterRequest) { r.Name = "  " },
		"bad email":    func(r *RegisterRequest) { r.Email = "rian" },
		"no tld":       func(r *RegisterRequest) { r.Email = "rian@localhost" },
		"display name": func(r *RegisterRequest) { r.Email = "Rian <rian@example.com>" },
		"short pw":     func(r *RegisterRequest) { r.Password = "1234567" },
		"long pw":      func(r *RegisterRequest) { r.Password = string(make([]byte, 73)) },
		"no terms":     func(r *RegisterRequest) { r.AcceptTerms = false },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			req := validRegister()
			mutate(&req)
			_, err := newTestService(newFakeRepo()).Register(context.Background(), req)
			wantStatus(t, err, http.StatusUnprocessableEntity)
		})
	}
}

func TestRefreshRotatesAndRejectsReuse(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)
	reg, _ := svc.Register(context.Background(), validRegister())

	refreshed, err := svc.Refresh(context.Background(), reg.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken == reg.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	_, err = svc.Refresh(context.Background(), reg.RefreshToken)
	wantStatus(t, err, http.StatusUnauthorized)

	repo.rotateErr = ErrRefreshTokenReused
	_, err = svc.Refresh(context.Background(), refreshed.RefreshToken)
	wantStatus(t, err, http.StatusUnauthorized)
}

func TestLogoutRevokesHash(t *testing.T) {
	repo := newFakeRepo()
	if err := newTestService(repo).Logout(context.Background(), "u", "raw-token"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repo.revokedHash, hashToken("raw-token")) {
		t.Fatal("logout must revoke by hash")
	}
}
