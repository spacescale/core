-- name: CreateWorkload :one
INSERT INTO workloads (project_id, name, slug, subdomain, image_ref, primary_region, runtime_port, status, is_public, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'queued', $8, now(), now())
RETURNING *;

-- name: ListWorkloadsByProjectID :many
SELECT *
FROM workloads
WHERE project_id = $1
ORDER BY created_at ASC, id ASC;

-- name: GetWorkloadByIDAndProjectID :one
SELECT *
FROM workloads
WHERE id = $1
  AND project_id = $2;

-- name: MarkWorkloadDeploying :one
UPDATE workloads
SET status = 'deploying',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkWorkloadRunning :one
UPDATE workloads
SET status = 'running',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkWorkloadFailed :one
UPDATE workloads
SET status = 'failed',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateWorkloadEnvVars :copyfrom
INSERT INTO workload_env_vars (workload_id, key, value_encrypted, is_secret)
VALUES ($1, $2, $3, $4);

-- name: GetRegistryCredentialByIDAndProjectID :one
SELECT *
FROM registry_credentials
WHERE id = $1
  AND project_id = $2;

-- name: UpsertWorkloadRegistryCredential :exec
INSERT INTO workload_registry_credentials (workload_id, registry_credential_id, created_at, last_used)
VALUES ($1, $2, now(), NULL)
ON CONFLICT (workload_id, registry_credential_id) DO NOTHING;
