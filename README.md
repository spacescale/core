## High performance baremetal MicroVM Orchestration

#### The core repo contains two go programs:       
scalecp: An authoritative Control Plane managing global state, the public API, and multi-tenant scheduling.   
scaled: the low level system daemon that manages lifecycle of its node, tenant workloads on vms and telemetry

#### Architecture     
The architecture is asynchronous and event driven. NATS  is used as a
message bus to decouple the Control Plane from the physical execution layer.This design allows the scalecp brain to
issue non-blocking commands and receive telemetry while the distributed scaled daemons manage microVM lifecycles
independently on bare-metal nodes. By shifting to this star-topology, the system eliminates fragile point-to-point
dependencies, ensuring that the platform remains resilient to network partitions and can scale horizontally across
global regions without manual intervention.