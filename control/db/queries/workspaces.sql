-- name: CreateWorkspace :one
INSERT INTO workspaces (owner_user_id, name, created_at, updated_at)
VALUES ($1, $2, now(), now()) RETURNING *;

-- name: ListWorkspacesByOwnerUserID :many
SELECT *
FROM workspaces
WHERE owner_user_id = $1
ORDER BY created_at ASC, id ASC;

-- name: GetWorkspaceByIDAndOwnerUserID :one
SELECT *
FROM workspaces
WHERE id = $1 AND owner_user_id = $2;

-- name: UpdateWorkspaceByIDAndOwnerUserID :one
UPDATE workspaces
SET name = $3, updated_at = now()
WHERE id = $1 AND owner_user_id = $2
RETURNING *;

-- name: DeleteWorkspaceByIDAndOwnerUserID :execrows
DELETE FROM workspaces
WHERE id = $1 AND owner_user_id = $2;
