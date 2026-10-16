package auth

const (
	queryInsertUser = `
		INSERT INTO users (name, email, password_hash, terms_accepted_at)
		VALUES ($1, $2, $3, now())
		RETURNING id, name, email, role, created_at`

	queryGetUserByEmail = `
		SELECT id, name, email, role, created_at, password_hash
		FROM users
		WHERE email = $1`

	queryGetUserByID = `
		SELECT id, name, email, role, created_at, password_hash
		FROM users
		WHERE id = $1`

	queryInsertRefreshToken = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`

	querySelectRefreshTokenForUpdate = `
		SELECT rt.id, rt.user_id, u.role, rt.expires_at, rt.revoked_at
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1
		FOR UPDATE OF rt`

	queryRevokeRefreshTokenByID = `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`

	queryRevokeAllUserRefreshTokens = `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`

	queryRevokeUserRefreshToken = `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND token_hash = $2 AND revoked_at IS NULL`
)
