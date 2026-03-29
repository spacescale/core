-- name: CreateProject :one
INSERT INTO projects (workspace_id, name, slug, created_at, updated_at)
VALUES ($1, $2, $3, now(), now()) RETURNING *;


-- name: ListProjectsByWorkspaceIDAndOwnerUserID :many
SELECT p.*
FROM projects AS p
         JOIN workspaces AS w ON w.id = p.workspace_id
WHERE p.workspace_id = $1
  AND w.owner_user_id = $2
ORDER BY p.created_at ASC, p.id ASC;


-- name: ListProjectsByOwnerUserID :many
SELECT p.*
FROM projects AS p
         JOIN workspaces AS w ON w.id = p.workspace_id
WHERE w.owner_user_id = $1
ORDER BY p.created_at ASC, p.id ASC;


-- name: GetProjectByIDAndOwnerUserID :one
SELECT p.*
FROM projects AS p
         JOIN workspaces AS w ON w.id = p.workspace_id
WHERE p.id = $1
  AND w.owner_user_id = $2;


-- name: GetProjectBySlug :one
SELECT *
FROM projects
WHERE slug = $1;

-- name: UpdateProjectByIDAndOwnerUserID :one
UPDATE projects AS p
SET name       = $3,
    updated_at = now() FROM workspaces AS w
WHERE p.workspace_id = w.id
  AND p.id = $1
  AND w.owner_user_id = $2
    RETURNING p.*;


-- name: DeleteProjectByIDAndOwnerUserID :execrows
DELETE
FROM projects AS p USING workspaces AS w
WHERE p.workspace_id = w.id
  AND p.id = $1
  AND w.owner_user_id = $2;
