## Core

scalecp: the Control Plane managing global state, the public API, and multi-tenant scheduling.   
scaled: the system daemon that manages lifecycle of its node, tenant workloads on vms and telemetry

### Dependencies
- Go 1.26
- protobuf
- docker compose
- goose

### Quick start
> [!WARNING]   
> Some endpoints may not work due to limitations on macOS for firecracker. A dev environment will be created for this.
```shell
make run # starts api server
```       
The entry point each program is found in `cmd` and the `makefile` exposes other build targets.
