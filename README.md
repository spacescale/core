# paas

[![CI](https://github.com/t0gun/paas/actions/workflows/ci.yml/badge.svg)](https://github.com/t0gun/paas/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/t0gun/paas/graph/badge.svg?token=A444L7NNC1)](https://codecov.io/gh/t0gun/paas)
![Go Version](https://img.shields.io/badge/go-1.25.5-blue)

A tiny container as a service platform

## Monorepo Structure

This project uses [Turborepo](https://turbo.build/) for monorepo management.

```
apps/
├── api/         # Go backend API
│   ├── cmd/api/     # Main entrypoint (HTTP server)
│   └── internal/
│       ├── domain/      # Business entities and rules
│       ├── contracts/   # Interface definitions
│       ├── service/     # Business logic orchestration
│       ├── http_api/    # HTTP transport (handlers, DTOs)
│       └── adapters/    # Interface implementations (store, runtime)
└── web/         # Next.js frontend
    └── src/app/     # App router pages

packages/        # Shared packages (future use)
```

## Request Flow (API)

```
cmd/api -> http_api -> service -> contracts -> adapters
```

## Design Patterns Used

- **Domain-Driven Design** - Business logic isolated in domain layer
- **Repository Pattern** - Data access abstracted via Store interface
- **Dependency Injection** - Services receive dependencies through constructors

## Prerequisites

- Go 1.25.5 or higher
- Node.js 22+
- pnpm 9+

## Getting Started

### Installation

```bash
git clone https://github.com/t0gun/paas.git
cd paas
pnpm install
```

### Development

Run all apps in development mode:

```bash
pnpm dev
```

Or run individually:

```bash
# API only
pnpm dev:api

# Web only
pnpm dev:web
```

### Building

```bash
pnpm build
```

### Testing

```bash
pnpm test
```

### Linting

```bash
pnpm lint
```

## Apps

### API (`apps/api`)

Go backend providing REST API for container orchestration.

```bash
cd apps/api
make test      # Run tests
make run       # Start server
```

### Web (`apps/web`)

Next.js frontend dashboard.

```bash
cd apps/web
pnpm dev       # Start dev server
pnpm build     # Production build
```
