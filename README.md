# SpaceScale

SpaceScale is a next-generation platform for high-density microVM orchestration. It provides a decentralized architecture for managing compute at scale, combining the hardware-level isolation of virtualization with the operational agility of modern container environments.

The platform is designed to transform distributed hardware into a unified, autonomous compute fabric, enabling high-performance multi-tenant infrastructure across global regions.

---

## Core Components

The repository is organized into two primary functional layers:

### scalecp (Control Plane)
The stateless management layer that serves the public API, manages tenant metadata, and orchestrates workload distribution. It leverages PostgreSQL for durable state and NATS for its decentralized communication fabric.

### scaled (Edge Daemon)
An autonomous agent running on physical hardware. It manages local resource capacity, interacts directly with the KVM/Firecracker subsystem, and maintains workload continuity. It is designed to operate independently, ensuring resilience during network partitions.

---

## Documentation

For technical deep dives into specific platform subsystems:

- **[Dynamic Node Provisioning](docs/dynamic-node-provisioning.md)**: Node lifecycle and onboarding.
- **[Firecracker Best Practices](docs/firecracker-best-practices.md)**: Hardening and optimization.
- **[Linux Kernel Guide](docs/kernel-build.md)**: Custom Linux Kernel Compilation Guide.

## API Reference

The SpaceScale API is documented and testable via **Yaak** workspaces. The collection files are located in `docs/api/`.
1. Download [Yaak](https://yaak.app/).
2. Import the YAML workspace files to view the complete API specification and request examples.

## Development

Instructions for environment setup, technical requirements, and local execution can be found in **[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)**.
