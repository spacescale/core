# Core

This repository is organized into two primary functional layers:

## scalecp 

The stateless control plane  that serves the public API, manages tenant metadata, and orchestrates workload
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

- [Guest kernel profile](docs/runtime/kernel-profile.md): production Firecracker guest kernel flags, rationale, and exclusions for the `guestd` app-guest model.
