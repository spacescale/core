-- name: CreateQueuedMicroVM :one
INSERT INTO microvms (workspace_id,
                      resource_type,
                      resource_id,
                      region,
                      vcpu,
                      ram_mb,
                      cpu_mode,
                      root_disk_mb,
                      volume_mb,
                      created_at,
                      updated_at)
VALUES ($1,
        $2,
        $3,
        $4,
        $5,
        $6,
        $7,
        $8,
        $9,
        NOW(),
        NOW())
RETURNING *;

-- name: GetMicroVMByID :one
SELECT *
FROM microvms
WHERE id = $1;


-- name: ListMicroVMsByResource :many
SELECT *
FROM microvms
WHERE resource_type = $1
  AND resource_id = $2
ORDER BY created_at ASC, id ASC;




-- name: UpdateMicroVMRegion :one
UPDATE microvms
SET region     = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;


-- name: MarkMicroVMStarting :one
UPDATE microvms
SET status        = 'starting',
    error_message = NULL,
    updated_at    = NOW()
WHERE id = $1
RETURNING *;


-- name: MarkMicroVMRunning :one
UPDATE microvms
SET status        = 'running',
    error_message = NULL,
    updated_at    = NOW()
WHERE id = $1
RETURNING *;


-- name: MarkMicroVMFailed :one
UPDATE microvms
SET status        = 'failed',
    error_message = $2,
    updated_at    = NOW()
WHERE id = $1
RETURNING *;
