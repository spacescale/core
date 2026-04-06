# Development Guide

This document outlines the requirements and procedures for developing on the SpaceScale platform.

## Requirements

- **Go 1.26.x**
- **protoc** (Protocol Buffers compiler)
- **docker compose** (for NATS and PostgreSQL)
- **goose** (Database migrations)
- **sqlc** (Type-safe SQL generator)

## Platform Constraints

- **scalecp:** Can be developed and run on macOS or Linux.
- **scaled:** Requires a Linux host with **KVM** access for microVM execution. Local development on macOS is limited to logic and control-plane integration.

## Local Environment Setup

1. **Start Infrastructure:**
   Use the provided docker compose file to start NATS and PostgreSQL.
   ```bash
   docker-compose up -d
   ```

2. **Run Migrations:**
   ```bash
   make migrate-up
   ```

3. **Start Control Plane:**
   ```bash
   make run
   ```

## API Testing

The API reference is located in `docs/api/`. These are **Yaak** workspace files. To view and test the API:
1. Open [Yaak](https://yaak.app/).
2. Import the YAML files found in `docs/api/`.
3. Ensure your local `scalecp` is running on the expected port.
