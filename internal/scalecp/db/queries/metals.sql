-- name: UpdateProvisioningMetalFromBootstrap :one
UPDATE metals
SET total_threads = sqlc.arg(total_threads),
    total_ram_mb  = sqlc.arg(total_ram_mb),
    total_disk_mb = sqlc.arg(total_disk_mb),
    updated_at    = NOW()
WHERE bootstrap_token_hash = sqlc.arg(bootstrap_token_hash)
  AND status = 'provisioning'
RETURNING *;

-- name: MarkMetalActiveByNodeID :execrows
UPDATE metals m
SET status = 'active',
    bootstrap_token_hash = NULL,
    updated_at = now()
FROM scaled s
WHERE s.id = sqlc.arg(node_id)
  AND s.metal_id = m.id
  AND m.status = 'provisioning';