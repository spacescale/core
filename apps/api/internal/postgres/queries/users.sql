-- name: UpsertUserByIdentityKey :one
INSERT INTO users (identity_key, email, name, avatar_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now()) ON CONFLICT (identity_key) DO
UPDATE
    SET email = EXCLUDED.email,
    name = EXCLUDED.name,
    avatar_url = EXCLUDED.avatar_url,
    updated_at = now()
    RETURNING *;

-- name: GetUserByIdentityKey :one
SELECT *
FROM users
WHERE identity_key = $1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;
