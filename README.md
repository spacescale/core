## Core

SpaceScale currently has two binaries:

- `scalecp`: the control plane that serves the HTTP API, owns durable state in Postgres, and consumes node events over
  NATS
- `scaled`: the node daemon that bootstraps itself with `scalecp`, reports node state, and will eventually manage VM
  lifecycle on a host

### Communication

NATS keeps node-to-control-plane communication simple across multiple servers by handling routing, reconnects, and
cluster membership at the transport layer

- JetStream KV stores live node state
- Postgres stores durable state

### Requirements

- Go 1.26.x
- protoc
- docker compose
- goose
- sqlc

### Platform Notes

- `scalecp` runs fine on macOS for local development
- `scaled` is currently Linux-only because the host probing code reads Linux system information directly
- Firecracker and real node lifecycle work should be treated as Linux-host development

```shell
make run # starts api server
```       

The entry point to each program is found in `cmd` and the `makefile` exposes other build and run targets. Platform is
still evolving very fast.
