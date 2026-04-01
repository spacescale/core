-- name: CreateQueuedMachine :one
INSERT INTO machines (
    app_id,
    deployment_id,
    region,
    tier,
    created_at,
    updated_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    now(),
    now()
)
RETURNING *;

-- name: GetMachineByID :one
SELECT *
FROM machines
WHERE id = $1;

-- name: ListMachinesByAppID :many
SELECT *
FROM machines
WHERE app_id = $1
ORDER BY created_at ASC, id ASC;

-- name: AssignMachineToNode :one
UPDATE machines
SET node_id = $2,
    status = 'assigned',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkMachineStarting :one
UPDATE machines
SET status = 'starting',
    error_message = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkMachineRunning :one
UPDATE machines
SET status = 'running',
    error_message = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkMachineFailed :one
UPDATE machines
SET status = 'failed',
    error_message = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
