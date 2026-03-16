-- name: UpsertScaled :exec
INSERT INTO scaled (id, name, region, status, last_seen_at, memory_available, cpu_usage, disk_available, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())
ON CONFLICT (id) DO UPDATE
    SET name = EXCLUDED.name,
        region = EXCLUDED.region,
        status = EXCLUDED.status,
        last_seen_at = EXCLUDED.last_seen_at,
        memory_available = EXCLUDED.memory_available,
        cpu_usage = EXCLUDED.cpu_usage,
        disk_available = EXCLUDED.disk_available,
        updated_at = now();


-- name: UpdateScaledLastSeenAndStatus :execrows
UPDATE scaled
SET region = $2,
    status = $3,
    last_seen_at = $4,
    memory_available = $5,
    cpu_usage = $6,
    disk_available = $7,
    updated_at = now()
WHERE id = $1;


-- name: MarkScaledOffline :execrows
UPDATE scaled
SET status = 'offline',
    last_seen_at = $2,
    updated_at = now()
WHERE id = $1;


-- name: MarkStaleScaledOffline :execrows
UPDATE scaled
SET status = 'offline',
    updated_at = now()
WHERE status <> 'offline'
  AND last_seen_at <= $1;
