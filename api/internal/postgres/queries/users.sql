-- name: UpsertUserByGithubID :one
INSERT INTO users (github_id, email, name, avatar_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now()) ON CONFLICT (github_id) DO
UPDATE
    SET email = EXCLUDED.email,
    name = EXCLUDED.name,
    avatar_url = EXCLUDED.avatar_url,
    updated_at = now()
    RETURNING *;

-- name: GetUserByGithubID :one
SELECT *
FROM users
WHERE github_id = $1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;
