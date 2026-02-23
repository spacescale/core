-- name: BootstrapDefaults :one
-- Use named args here so sqlc generates readable parameter names
-- (OwnerUserID, WorkspaceName, ProjectName, ProjectSlug, ProjectRegion)
-- instead of generic Column1..Column5 in this multi-CTE query.
WITH input AS (
    SELECT
        sqlc.arg(owner_user_id)::uuid AS owner_user_id,
        sqlc.arg(workspace_name)::text AS workspace_name,
        sqlc.arg(project_name)::text AS project_name,
        sqlc.arg(project_slug)::text AS project_slug,
        sqlc.arg(project_region)::text AS project_region
),
     has_workspace AS (
         SELECT EXISTS (
             SELECT 1
             FROM workspaces w
                      JOIN input i ON i.owner_user_id = w.owner_user_id
         ) AS has_any
     ),
     inserted_workspace AS (
         INSERT INTO workspaces (owner_user_id, name, created_at, updated_at)
             SELECT i.owner_user_id, i.workspace_name, now(), now()
             FROM input i
             WHERE NOT (SELECT has_any FROM has_workspace)
             ON CONFLICT (owner_user_id, name) DO NOTHING
             RETURNING id
     ),
     inserted_project AS (
         INSERT INTO projects (workspace_id, name, slug, region, created_at, updated_at)
             SELECT iw.id, i.project_name, i.project_slug, i.project_region, now(), now()
             FROM inserted_workspace iw
                      CROSS JOIN input i
             RETURNING id
     )
SELECT
    EXISTS (SELECT 1 FROM inserted_workspace) AS created,
    (SELECT id FROM inserted_workspace LIMIT 1) AS workspace_id,
    (SELECT id FROM inserted_project LIMIT 1) AS project_id;