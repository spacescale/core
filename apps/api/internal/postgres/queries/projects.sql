-- name: CreateProject :one
INSERT INTO projects (owner_user_id, name, slug, region, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now()) RETURNING *;