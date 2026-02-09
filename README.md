# SpaceScale

[![API](https://github.com/t0gun/spacescale/actions/workflows/api.yml/badge.svg?)](https://github.com/t0gun/spacescale/actions/workflows/api.yml)
[![Web](https://github.com/t0gun/spacescale/actions/workflows/web.yml/badge.svg?)](https://github.com/t0gun/spacescale/actions/workflows/web.yml)
[![api-test](https://codecov.io/gh/t0gun/spacescale/graph/badge.svg?token=A444L7NNC1&flag=api&label=api-test)](https://codecov.io/gh/t0gun/spacescale)


> [!IMPORTANT]
> SpaceScale is in early development and may have breaking changes

This repo uses [Turborepo](https://turbo.build/) for workspace orchestration. Local infrastructure and migration
workflows are handled from the root `Makefile`.

### Prerequisites

- Go `1.25+`
- Node.js `22+`
- pnpm `9+`
- Docker + Docker Compose
- `goose` for migrations
- `sqlc`  for query/codegen workflows


### API and DB - Uses Makefile 
Here are some few Targets
```bash
make compose-up # starts container
make migrate-up
make goose-create <migration_name>
```

#### Turbo repo orchestrates web and API services
```bash
pnpm install
pnpm dev
```