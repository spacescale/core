-- name: GetScaledByID :one
SELECT *
FROM scaled
WHERE id = $1;
-- name: GetScaledByMetalID :one
SELECT *
FROM scaled
WHERE metal_id = $1;
-- name: UpsertScaledBootstrap :one
INSERT INTO scaled (
    id,
    version,
    boot_id,
    status,
    status_reason,
    metal_id,
    created_at,
    updated_at
)
VALUES (
           sqlc.arg(id),
           sqlc.arg(version),
           sqlc.arg(boot_id),
           'offline',
           NULL,
           sqlc.arg(metal_id),
           now(),
           now()
       )
ON CONFLICT (metal_id) DO UPDATE
    SET version = EXCLUDED.version,
        boot_id = EXCLUDED.boot_id,
        updated_at = now()
RETURNING *;


-- name: UpdateScaledPresence :execrows
UPDATE scaled
SET boot_id = sqlc.arg(boot_id),
    status = sqlc.arg(status),
    status_reason = sqlc.narg(status_reason),
    total_running_vms = sqlc.arg(total_running_vms),
    updated_at = now()
WHERE id = sqlc.arg(id);


-- name: MarkScaledOffline :execrows
UPDATE scaled
SET status = 'offline',
    status_reason = sqlc.narg(status_reason),
    updated_at = now()
WHERE id = sqlc.arg(id);