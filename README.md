# SpaceScale

[![API](https://github.com/t0gun/spacescale/actions/workflows/api.yml/badge.svg?)](https://github.com/t0gun/spacescale/actions/workflows/api.yml)
[![Web](https://github.com/t0gun/spacescale/actions/workflows/web.yml/badge.svg?)](https://github.com/t0gun/spacescale/actions/workflows/web.yml)
[![api-test](https://codecov.io/gh/t0gun/spacescale/graph/badge.svg?token=A444L7NNC1&flag=api&label=api-test)](https://codecov.io/gh/t0gun/spacescale)

> [!IMPORTANT]
> SpaceScale is in early development and may have breaking changes

A deployment platform built as a monorepo with [Turborepo](https://turbo.build/) and [pnpm](https://pnpm.io/) workspaces.

## Project Structure

```
apps/
  api/          Go API server (Chi router)
  web/          Dashboard (Next.js 15 + React 19)
  marketing/    Marketing site (Next.js 15)
  db/           Postgres migrations & sqlc queries
packages/
  ui/           Shared component library (@spacescale/ui) with Storybook
```

## Prerequisites

- **Go** 1.26+
- **Node.js** 22+
- **pnpm** 9+
- **Docker**
- **sqlc** for query codegen

## Getting Started

### 1. Install dependencies

```bash
pnpm install
```

### 2. Run API + DB quickly

```bash
make run
```

This builds the DB image from `apps/db/Dockerfile`, starts Postgres on `localhost:5432`, runs migrations inside the DB container, and then runs the API natively.
`make test` reuses the same container and runs against the `spacescale_test` database.

### 3. Run all services

```bash
pnpm dev
```

This starts every app in parallel:

| App         | URL                   | Description    |
| ----------- | --------------------- | -------------- |
| `web`       | http://localhost:3000 | Dashboard      |
| `marketing` | http://localhost:3001 | Marketing site |
| `api`       | http://localhost:8080 | Go API server  |

### Run a single app

```bash
pnpm dev:web          # dashboard only
pnpm dev:marketing    # marketing site only
pnpm dev:api          # API only
```

## Storybook

The shared UI library (`@spacescale/ui`) ships with Storybook for browsing components in isolation.

```bash
pnpm storybook
```

## Common Commands

| Command          | Description                               |
| ---------------- | ----------------------------------------- |
| `pnpm dev`       | Start all apps in dev mode                |
| `pnpm build`     | Build all apps                            |
| `pnpm typecheck` | Run TypeScript checks across all packages |
| `pnpm lint`      | Lint all packages                         |
| `pnpm test`      | Run all tests                             |
| `pnpm format`    | Format code with Prettier                 |
| `pnpm storybook` | Launch Storybook for `@spacescale/ui`     |
| `pnpm clean`     | Remove build artifacts and `node_modules` |

## Database (Makefile)

Local infrastructure and migration workflows are handled from the root `Makefile`.

| Target           | Description                                                       |
| ---------------- | ----------------------------------------------------------------- |
| `make run`       | Build/start local DB and run API                                   |
| `make test`      | Run API tests against `spacescale_test` on the same local DB       |
| `make stop`      | Stop/remove the local DB container (`spacescale-db`)               |
| `make mint`      | Mint a BFF JWT for local API testing (reads `.env.local`)          |
| `make db-build`  | Build the local DB image (`spacescale-db:local`)                   |
| `make db-start`  | Start local DB container and wait for container-side migrations     |
