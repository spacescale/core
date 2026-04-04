-- name: UpdateProvisioningMetalFromBootstrap :one
UPDATE metals
SET total_cores = sqlc.arg(total_cores),
    total_ram_mb = sqlc.arg(total_ram_mb),
    total_disk_mb = sqlc.arg(total_disk_mb),
    status = 'active',
    bootstrap_token_hash = NULL,
    updated_at = NOW()
WHERE bootstrap_token_hash = sqlc.arg(bootstrap_token_hash)
  AND status = 'provisioning'
RETURNING *;