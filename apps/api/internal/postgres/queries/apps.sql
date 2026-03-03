-- name: CreateApp :one
INSERT INTO apps (project_id, name, slug, subdomain, image_ref, runtime_port, status, is_public, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, 'queued', $7, now(), now())
RETURNING *;

-- name: CreateQueuedDeployment :exec
INSERT INTO deployments (app_id, status, image_ref, runtime_port, public_url, created_at, updated_at)
VALUES ($1, 'queued', $2, $3, NULL, now(), now());

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

-- name: ClaimAppEnvVarsByKeyID :many
WITH candidates AS (
    SELECT id, value_encrypted
    FROM app_env_vars
    WHERE split_part(value_encrypted, ':', 3) = sqlc.arg(key_id)
    ORDER BY created_at ASC
        FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(limit_rows)::int
)
SELECT id, value_encrypted
FROM candidates;


-- name: UpdateAppEnvVarCiphertextCAS :execrows
UPDATE app_env_vars
SET value_encrypted = sqlc.arg(new_ciphertext),
    updated_at = now()
WHERE id = sqlc.arg(env_var_id)
  AND value_encrypted = sqlc.arg(previous_ciphertext);