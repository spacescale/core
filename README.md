[![CI](https://github.com/spacescale/core/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/spacescale/core/actions/workflows/pipeline.yaml)
# Core
> ReadMe is 100% Human Written 

This repository owns the `controlplane` and `edge daemon` used by [SpaceScale](https://spacescale.io) Systems, and it is organized into two primary functional layers we will see later. This document is written using an architectural-style explanation over implementation to break down boundaries and important concepts, which makes the implementation easy to follow and understand.


The SpaceScale API is documented and testable via **Yaak** workspaces. The collection files are located in `docs/api/`.you can download [Yaak](https://yaak.app/) and import the YAML workspace files to view the complete API specification and request examples.


## Control
The controlplane, also called `control` in this repo, owns the orchestration layer of this system and most of the product-facing entities, such as the tenant structure, identity, records, Baremetal Host Provisioning, Command and Control, alongside partial scheduling intent. `scheduling` responsibility is shared with both the control plane and mostly managed by the edge daemon using a decentralized, auction-based model over the [NATS](https://nats.io) messaging fabric. External SpaceScale clients will talk to this compute system through control. control exposes `Layer 7 API` that clients can consume. Application layer contracts might change often as product evolves, which means breaking changes are expected until this platform stabilizes.

if you have docker compose installed, you can bring up control API server locally using the make target below
```sh
	make controlp
```
The  folder `control` owns all of its sub system folders and the root `Makefile` has other useful targets to get development up and running easily.


## ScaleD    
The `Scale Daemon`, also known in short as `ScaleD`, is the autonomous edge daemon on every baremetal server that is provisioned by `control`. The controlplane's `provisioner` actually provisions each `server(Node)` with a golden image that has ScaleD baked inside with its dependencies and its own tiny `NATS` leaf node that it uses for communication with the upstream messaging fabric. The security boundary is MutualTLS. In fact, before a Node joins, the daemon is smart enough to run its preflight and actually know it is ready to start accepting workloads. Scheduling is intentionally pushed to the edge because the daemon can bid for workloads auctioned through a regional NATS subject if it has enough space to fit the workload. Although the current scheduling intent is built around `spacescale ignite`, `spacescale machines` is still conceptual and not worked on, but we hope to adopt this model as well. `scaled` is a Linux-only daemon, and its requirements require its native host with preflight requirements. Read the `node` package to learn more about its preflight system.

You can build a native AMD64 Linux binary using this make target:

```sh
make build-scaled
```
It is useful to read more of some important parts of the source code because this is only an intro.


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
