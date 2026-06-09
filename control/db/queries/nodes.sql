-- name: UpdateProvisioningNodeFromBootstrap :one
UPDATE nodes
SET bootstrap_token_hash = NULL,
    updated_at = NOW()
WHERE bootstrap_token_hash = sqlc.arg(bootstrap_token_hash)
RETURNING *;
