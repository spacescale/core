[![CI](https://github.com/spacescale/core/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/spacescale/core/actions/workflows/pipeline.yaml)
# Core
> ReadMe is 100% Human Written 

This repository owns the `controlplane` and `edge daemon` used by [SpaceScale](https://spacescale.io) Systems, and it is organized into two primary functional layers we will see later. This document is written using an architectural-style explanation over implementation to break down boundaries and important concepts, which makes the implementation easy to follow and understand.

## Control
The controlplane, also called `control` in this repo, owns the orchestration layer of this system and most of the product-facing entities, such as the tenant structure, identity, records, Baremetal Host Provisioning, Patching and Management, alongside partial scheduling intent. `scheduling` responsibility is shared with both the control plane and mostly managed by the edge daemon using a decentralized, auction-based model over the [NATS](https://nats.io) messaging fabric. External SpaceScale clients will talk to this compute system through control. control exposes `Layer 7 API` that clients can consume. Application layer contracts might change often as product evolves, which means breaking changes are expected until this platform stabilizes.

if you have docker compose installed, you can bring up control API server locally using the make target below
```sh
	docker compose up controlp
```
The  folder `control` owns all of its sub system folders and the root `Makefile` has other useful targets to get development up and running easily.


## ScaleD

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
