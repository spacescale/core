[![CI](https://github.com/spacescale/core/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/spacescale/core/actions/workflows/pipeline.yaml)
# Core
> ReadMe is 100% Human Written 

This repository owns the `controlplane` and `edge daemon` used by [SpaceScale](https://spacescale.io) Systems, and it is organized into two primary functional layers we will see later. This document is written using an architectural-style explanation over implementation to break down boundaries and important concepts, which makes the implementation easy to follow and understand.

## Reference

- **API**: Yaak workspaces in `docs/api/`. Download [Yaak](https://yaak.app/).
- **Runtime**: [Guest kernel profile](docs/runtime/kernel-profile.md).

## Local Development

The normal control plane development flow runs inside Docker Compose. Read the compose to get the overview of dev setup. `. Start the full environment stack in the background.

```bash
make compose-start
```

## Control
The controlplane, also called `control` in this repo, owns the orchestration layer of this system and most of the product-facing entities, such as the tenant structure, identity, records, Baremetal Host Provisioning, Command and Control, alongside partial scheduling intent. `scheduling` responsibility is shared with both the control plane and mostly managed by the edge daemon using a decentralized, auction-based model over the [NATS](https://nats.io) messaging fabric. External SpaceScale clients will talk to this compute system through control. control exposes `Layer 7 API` that clients can consume. Application layer contracts might change often as product evolves, which means breaking changes are expected until this platform stabilizes.

you can bring up control API server locally using the make target below
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

