-- name: CreateApp :one
INSERT INTO apps (project_id, name, slug, subdomain, image_ref, tier, primary_region, runtime_port, status, is_public, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'queued', $9, now(), now())
RETURNING *;

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

-- name: CreateAppEnvVar :exec
INSERT INTO app_env_vars (app_id, key, value_encrypted, is_secret, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now());

-- name: GetRegistryCredentialByIDAndProjectID :one
SELECT *
FROM registry_credentials
WHERE id = $1
  AND project_id = $2;

-- name: UpsertAppRegistryCredential :exec
INSERT INTO app_registry_credentials (app_id, registry_credential_id, created_at, last_used)
VALUES ($1, $2, now(), NULL)
ON CONFLICT (app_id, registry_credential_id) DO NOTHING;
