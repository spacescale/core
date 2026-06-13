[![CI](https://github.com/spacescale/core/actions/workflows/pipeline.yml/badge.svg)](https://github.com/spacescale/core/actions/workflows/pipeline.yml)
# Core

This repository is organized into two primary functional layers:

## control

The stateless control plane that serves the public API, manages tenant metadata, and orchestrates workload
distribution. It leverages PostgreSQL for durable state and NATS for its decentralized communication fabric.

## scaled

An autonomous edge daemon running on physical hardware. It manages local resource capacity, interacts directly with the
KVM/Firecracker subsystem, and maintains workload continuity. It is designed to operate independently, ensuring
resilience during network partitions.

---

## API Reference

The SpaceScale API is documented and testable via **Yaak** workspaces. The collection files are located in `docs/api/`.

1. Download [Yaak](https://yaak.app/).
2. Import the YAML workspace files to view the complete API specification and request examples.

## Runtime Docs

- [Guest kernel profile](docs/runtime/kernel-profile.md): production Firecracker guest kernel flags, rationale, and
  exclusions for the `guestd` app-guest model.

## Local Development

The normal control plane development flow runs inside Docker Compose. Compose provides the development values for
Postgres, NATS, migrations, and `controlp`. Start the full control plane stack in the background.

```bash
make compose-start
```

This brings up Postgres and NATS, waits for Postgres to become healthy, runs the database migrations once, and then
starts `controlp`. The control plane listens on `http://127.0.0.1:8080` but If you want to run the same flow in the
foreground and watch the logs directly, use this command.

```bash
make controlp
```

you can check the full `Makefile` for other important targets to be used during development.
