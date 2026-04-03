# Firecracker Best practices

There are some important security rules we are considering to integrate with our microVM. The below info captures it

- Disable SMT (Hyperthreading): we are doing this to prevent cross tenant leakage. We are guarantee a single tenant will
  use a single core at a time.
- Disable KSM (Kernel SamePage Merging): we are running multi tenant vms with identical OS. we don't want the kernel to
  merge identical memory pages of tenants memory
- Turn off Swap: we don't want the host to deduplicate memory across VMs.

## Jailer is important

to run firecracker binary safely in production, we must wrap it in the jailer binary that comes with firecracker.jailer
provides :

- Unique identity: Every single customer VM must run under its own dynamically generated, unprivileged Linux UID/GID.
- Cgroups: Jailer uses cgroups to put a hard cage around the VM. If a user pays for 512MB of RAM, Jailer ensures the
  Linux host kills the VM if it tries to use 513MB

The firecracker-go-sdk handles a lot, and we don't need to write C code or shell out.

## Preventing Noisy Neighbors

Noisy Neighbors are inevitable on Shared Hardware:

Rate limit the Disk: Use Firecracker's API to set IOPS and Bandwidth limits on the ext4 block device i attach.
Rate limit the network: configure Firecracker's internal network rate limiters so one VM cannot saturate your vRack
connection. we can set dedicated quota of cpu time via cpu subsystem and also do cpu pinning for dedicated vms.

# Linux 6.1 Boot speed fix
Remounts the cgroup filesystem to favor dynamic mods, dropping VM boot time to milliseconds.

# Killing Serial Console

Firecracker tires to output virtual serial console to the host system. stdout logs output to this console, and it will
eventually fill up ram or disk space. we need to stop this b passing the 8250.nr_uarts=0 boot argument in the go code.