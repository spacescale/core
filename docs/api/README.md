# Yaak API Workspace

This folder contains the exported Yaak workspace for the local SpaceScale API.

## What This Is
- A hand-driven API workspace for the control plane.
- No frontend is required.
- Auth is cookie-based through WorkOS.
- The current resource name is `workloads`, not `apps`.

## Before Using It
1. Start the local stack.
2. Open `http://localhost:8080/auth/login` in a browser.
3. Copy the `spacescale_session` cookie value.
4. Put that value into the Yaak `WORKOS_SESSION` variable.
5. Refresh `WORKSPACE_ID`, `PROJECT_ID`, and `PRIMARY_REGION` when the DB is reset or bootstrap state changes.

## Current Routes
- `GET /healthz`
- `GET /auth/login`
- `GET /auth/callback`
- `GET /auth/logout`
- `POST /v1/bootstrap-defaults`
- `GET /v1/workspaces`
- `POST /v1/workspaces`
- `GET /v1/workspaces/{workspaceId}/projects`
- `POST /v1/workspaces/{workspaceId}/projects`
- `GET /v1/workspaces/{workspaceId}/projects/{projectId}/workloads`
- `POST /v1/workspaces/{workspaceId}/projects/{projectId}/workloads`

## Recommended Yaak Order
1. Health Check
2. Open WorkOS Login
3. Bootstrap Defaults
4. List Workspaces
5. List Projects In Workspace
6. Create Workload
7. List Workloads
8. Open WorkOS Logout

## Notes
- `POST /v1/bootstrap-defaults` is idempotent.
- `Create Workload` launches one workload by default with `targetReplicas = 1`.
- For live placement tests, use `PRIMARY_REGION=eu-central`.
- `WORKOS_LOGOUT_REDIRECT_URI` currently points back to `http://localhost:8080/healthz`.

## Removed
- The old `Mint Process` info request is no longer used.
- This README is the top-level docs entry instead.
