# paas

[![CI](https://github.com/t0gun/paas/actions/workflows/ci.yml/badge.svg)](https://github.com/t0gun/paas/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/t0gun/paas/graph/badge.svg?token=A444L7NNC1)](https://codecov.io/gh/t0gun/paas)
![Go Version](https://img.shields.io/badge/go-1.25.5-blue)

A tiny container as a service platform

## Architecture

```
internal/
├── domain/      # Business entities and rules
├── contracts/   # Interface definitions
├── service/     # Business logic orchestration
└── adapters/    # Interface implementations
```

## Design Patterns Used

- **Domain-Driven Design** - Business logic isolated in domain layer
- **Repository Pattern** - Data access abstracted via Store interface
- **Dependency Injection** - Services receive dependencies through constructors

### Prerequisites

- Go 1.25.5 or higher
- Make

### Installation

```bash
git clone https://github.com/t0gun/paas.git
cd paas
go mod download
```

## Building and Development

A make file has been provided to make testing and building easy.Read the make file to see all commands. make is
available
by default on all UNIX/Linux OS. To run a quick
test outside a container. You can use

```bash
make test
```
