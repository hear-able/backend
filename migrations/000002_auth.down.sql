DROP TABLE IF EXISTS refresh_tokens;

ALTER TABLE users
    DROP COLUMN IF EXISTS terms_accepted_at,
    DROP COLUMN IF EXISTS role;
