# How ScaleCP Provisions Nodes on Demand

This document defines how ScaleCP uses a decentralized, stateless workflow for the dynamic provisioning and scaling of
nodes. This architecture enforces strict hardware isolation, utilizes NATS for state and concurrency, and
operates on an exact 86% capacity threshold.

## Core Philosophy: The Stateless Control Plane

The SpaceScale control plane `ScaleCP` is a stateless router. Infrastructure state is never stored in a PostgreSQL
database. The edge node daemon `scaled` acts as the ultimate source of truth. All capacity math, provisioning
locks, and cluster coordination are handled entirely over a secure NATS messaging fabric using mTLS.

## The Provisioning LifeCycle

### Phase 1: Real-Time Capacity Calculation Using Scatter-Gather Pattern

To determine if a region requires more nodes, `ScaleCP` asks active nodes directly using the NATS
Request-Reply pattern.

- The Broadcast: Every 60 seconds, a background worker in scalecp broadcasts a NATS request to a regional subject, for
  example region.us-east.capacity.report
- The Reply: Every active scaled edge daemon in that region reads its internal ledger and replies with its true physical
  boundaries: total cpu cores vs allocated, total ram vs allocated, total disk space vs allocated, and usable ip addresses.
- scalecp aggregates the replies. It runs the utilization math: (Total Allocated / Total Physical)
- If ANY of the four metrics hits >= 86%, the region is critically low on inventory, and scalecp initiates
  provisioning.

### Phase 2: The Concurrency Lock Preventing the thundering Herd Problem

If multiple highly available ScaleCP instances detect the 86% threshold at the same millisecond, they will all attempt
to buy a server. We use the NATS JetStream KeyValue Store to prevent duplicate purchases.

- When a scalecp instance decides to provision, it attempts to atomically create a NATS KV record:
  us-east-provisioning-lock
- Atomic Guarantee: NATS guarantees only one instance will successfully create the key. The winning instance proceeds.
- The Losers: The other scalecp instances receive a "Key Exists" error, silently drop their execution, and go back to
  sleep.
- The 15-Minute TTL: The lock is created with a 15-minute Time-To-Live. This covers the time it takes OVH to prepare
  the node. If the API call fails, the lock expires automatically.

### Phase 3: Node Procurement and SSH bootstrap

The scalecp instance holding the lock calls the OVH API to rent a standard server for the new node. Once the OVH API
reports the server is active and returns the IP address, scalecp connects via SSH to bootstrap the node.

- Once the OVH API reports the server is active and returns the IP address, scalecp connects via SSH to bootstrap the
  node.
- mTLS Provisioning: scalecp generates and injects the cryptographic mTLS certificates (ca.crt, client.crt, client.key)
  required for the node to securely authenticate with the regional NATS cluster.
- Systemd Setup: It writes the /etc/systemd/system/scaled.service file and starts the daemon.

### Phase 4: Edge Pre-Flight and Network Lockdown

The moment the scaled binary starts on the new server, it executes a strict PreFlight sequence. It secures
the node and locks down the network before ever connecting to NATS.

- Kill SMT (Hyperthreading): scaled writes off to /sys/devices/system/cpu/smt/control. 1 vCPU = 1 isolated physical
  core.
- Kill Swap & KSM: Disables swap and Kernel Samepage Merging to prevent memory deduplication attacks.
- Patch KVM Bug: Hot-patches the Linux 6.1 favordynmods bug and disables KVM huge page recovery for millisecond boot
  times.
- Measure Truth: scaled counts the final, secured physical cores and RAM.

Network Firewall iptables nftables

- scaled configures the host firewall to DROP all inbound traffic by default.
- locks down ssh completely. control plane doesnt have business with the node anymore. scaled is now the manager.
- It allows inbound and outbound traffic for the Firecracker microVM bridge network.
- allows traffic to update and patch the server if needed.
- it allows outbound mTLS connections to the Regional NATS cluster.

### Phase 5: Closing the Loop

- scaled uses its injected mTLS certificates to connect to the Regional NATS cluster.
- It publishes a node ready event with its physical cores and RAM.
- scalecp listens for this event. Upon receiving it, scalecp explicitly deletes the us-east-provisioning-lock from the
  NATS KV store.
- During the next 60-second scatter-gather, the new node replies with its empty capacity. The regional utilization
  instantly drops below 86%, and normal operations resume
