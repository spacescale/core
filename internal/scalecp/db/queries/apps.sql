-- name: CreateApp :one
INSERT INTO apps (project_id, name, slug, subdomain, image_ref, plan_id, primary_region, runtime_port, status, is_public, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'queued', $9, now(), now())
RETURNING *;

-- name: ListAppsByProjectID :many
SELECT *
FROM apps
WHERE project_id = $1
ORDER BY created_at ASC, id ASC;

-- name: GetAppByIDAndProjectID :one
SELECT *
FROM apps
WHERE id = $1
  AND project_id = $2;

-- name: MarkAppDeploying :one
UPDATE apps
SET status = 'deploying',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkAppRunning :one
UPDATE apps
SET status = 'running',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkAppFailed :one
UPDATE apps
SET status = 'failed',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateAppEnvVars :copyfrom
INSERT INTO app_env_vars (app_id, key, value_encrypted, is_secret)
VALUES ($1, $2, $3, $4);

-- name: GetRegistryCredentialByIDAndProjectID :one
SELECT *
FROM registry_credentials
WHERE id = $1
  AND project_id = $2;

-- name: UpsertAppRegistryCredential :exec
INSERT INTO app_registry_credentials (app_id, registry_credential_id, created_at, last_used)
VALUES ($1, $2, now(), NULL)
ON CONFLICT (app_id, registry_credential_id) DO NOTHING;
