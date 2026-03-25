## Core
SpaceScale currently has two binaries:

- `scalecp`: control plane that serves the API, owns durable state, and consumes node events over NATS
- `scaled`:  daemon that bootstraps itself with `scalecp`, reports node state,  manage guest VMs and node lifecycle

## Communication                      
NATS keeps node-to-control-plane communication simple across multiple servers by handling routing, reconnects, and
cluster membership at the transport layer .JetStream KV stores live node state and Postgres stores durable state.

## Requirements               
- Go 1.26.x
- protoc
- docker compose
- goose
- sqlc

## Platform Notes                       
- `scalecp` runs fine on macOS for local development
- `scaled` targets Linux, it's built for managing guest vms, and node lifecycle on servers. 
- Firecracker and real node lifecycle work should be treated as Linux-host development

## Quickstart          
```shell            
make run # starts api server
```                 
The entry point to each program is found in `cmd` and the `makefile` exposes other build and run targets. Platform is
still evolving very fast.
