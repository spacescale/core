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
    metal_id,
    created_at,
    updated_at
)
VALUES (
           sqlc.arg(id),
           sqlc.arg(version),
           sqlc.arg(metal_id),
           now(),
           now()
       )
ON CONFLICT (metal_id) DO UPDATE
    SET version = EXCLUDED.version,
        updated_at = now()
RETURNING *;