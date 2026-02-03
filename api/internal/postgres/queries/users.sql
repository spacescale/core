-- name: UpsertUserByGithubID :one
INSERT INTO users (github_id, created_at, updated_at)
VALUES ($1, now(), now()) ON CONFLICT (github_id) DO
UPDATE
    SET updated_at = now()
    RETURNING *;