package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const pgUniqueViolation = "23505"

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, name, email, passwordHash, role string) (User, error) {
	var u User
	err := r.db.QueryRowContext(ctx, queryInsertUser, name, email, passwordHash, role).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	return r.getUser(ctx, queryGetUserByEmail, email)
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (User, error) {
	return r.getUser(ctx, queryGetUserByID, id)
}

func (r *Repository) getUser(ctx context.Context, query string, arg any) (User, error) {
	var u User
	err := r.db.QueryRowContext(ctx, query, arg).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (r *Repository) InsertRefreshToken(ctx context.Context, userID string, tokenHash []byte, expiresAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, queryInsertRefreshToken, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically revokes the token identified by oldHash and
// stores newHash for the same user. Presenting an already revoked token is
// treated as theft: every token of that user is revoked.
func (r *Repository) RotateRefreshToken(ctx context.Context, oldHash, newHash []byte, newExpiresAt, now time.Time) (userID, role string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var (
		tokenID   string
		expiresAt time.Time
		revokedAt sql.NullTime
	)
	err = tx.QueryRowContext(ctx, querySelectRefreshTokenForUpdate, oldHash).
		Scan(&tokenID, &userID, &role, &expiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrRefreshTokenInvalid
	}
	if err != nil {
		return "", "", fmt.Errorf("select refresh token: %w", err)
	}

	if revokedAt.Valid {
		if _, err := tx.ExecContext(ctx, queryRevokeAllUserRefreshTokens, userID); err != nil {
			return "", "", fmt.Errorf("revoke all refresh tokens: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return "", "", fmt.Errorf("commit: %w", err)
		}
		return "", "", ErrRefreshTokenReused
	}
	if !expiresAt.After(now) {
		return "", "", ErrRefreshTokenInvalid
	}

	if _, err := tx.ExecContext(ctx, queryRevokeRefreshTokenByID, tokenID); err != nil {
		return "", "", fmt.Errorf("revoke refresh token: %w", err)
	}
	if _, err := tx.ExecContext(ctx, queryInsertRefreshToken, userID, newHash, newExpiresAt); err != nil {
		return "", "", fmt.Errorf("insert refresh token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("commit: %w", err)
	}
	return userID, role, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, userID string, tokenHash []byte) error {
	if _, err := r.db.ExecContext(ctx, queryRevokeUserRefreshToken, userID, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
