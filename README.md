## High performance baremetal MicroVM Orchestration

> The core repo contains two go programs:

scalecp: the Control Plane managing global state, the public API, and multi-tenant scheduling.   
scaled: the system daemon that manages lifecycle of its node, tenant workloads on vms and telemetry

> Architecture

The architecture is asynchronous and event driven. NATS is used as a message bus.
.This design allows the scalecp brain to issue non-blocking commands and receive telemetry while the distributed scaled
daemons manage microVM lifecycles independently on bare-metal nodes. 