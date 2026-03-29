-- name: CreateQueuedDeployment :one
INSERT INTO deployments (app_id, status, image_ref, runtime_port, public_url, created_at, updated_at)
VALUES ($1, 'queued', $2, $3, NULL, now(), now())
RETURNING *;

-- name: MarkDeploymentDeploying :one
UPDATE deployments
SET status = 'deploying',
    error_message = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkDeploymentRunning :one
UPDATE deployments
SET status = 'running',
    error_message = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkDeploymentFailed :one
UPDATE deployments
SET status = 'failed',
    error_message = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
