-- name: UpsertAgent :exec
INSERT INTO agents (agent_key, name, status, capabilities, last_session_id, last_seen_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now(), now())
ON CONFLICT (agent_key) DO UPDATE
    SET name = EXCLUDED.name,
        status = EXCLUDED.status,
        capabilities = EXCLUDED.capabilities,
        last_session_id = EXCLUDED.last_session_id,
        last_seen_at = EXCLUDED.last_seen_at,
        updated_at = now();


-- name: UpdateAgentLastSeenAndStatus :execrows
UPDATE agents
SET status = $2,
    last_seen_at = $3,
    updated_at = now()
WHERE agent_key = $1
  AND last_session_id = $4;


-- name: MarkAgentOffline :execrows
UPDATE agents
SET status = 'offline',
    last_seen_at = $2,
    updated_at = now()
WHERE agent_key = $1
  AND last_session_id = $3;


-- name: MarkStaleAgentsOffline :execrows
UPDATE agents
SET status = 'offline',
    updated_at = now()
WHERE status <> 'offline'
  AND last_seen_at <= $1;
