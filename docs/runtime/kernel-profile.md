# SpaceScale Guest Kernel Profile

SpaceScale guests are Firecracker microVMs that boot into `scoutd` as PID 1. The current production model is one customer app supervised by one `scoutd` inside one VM. We do not run Docker, containerd, or a full container host inside the guest.

This kernel profile is for a production PaaS app guest. It should boot quickly, support normal Linux application runtimes, databases, brokers, message queues, CPU inference workloads, and basic `scoutd` workload control without adding container-host, guest eBPF, tracing, debug, or GPU stacks before we need them.

## Rules

- Build required features into the kernel as `=y`; do not rely on loadable modules.
- Keep `CONFIG_MODULES` disabled while the guest cmdline includes `nomodule` and the rootfs does not ship `/lib/modules`.
- Keep the guest model narrow: Firecracker device model, `scoutd`, and app processes.
- Do not enable features just because a general-purpose Linux distribution enables them.
- Add kernel surface only when the guest actually owns that responsibility.
- Keep the default guest lean. Put platform eBPF, deep network observability, and routing acceleration on the host where `scaled` owns the machine.

## Baseline

Use Linux `6.1.x` LTS for the current Firecracker path. Kernel artifacts should be named with a SpaceScale revision instead of silently replacing an existing runtime object:

```text
vmlinux-v6.1.169-spacescale4-x86_64
```

## Workload Coverage

The default guest kernel is intentionally lean. It supports normal app execution inside the VM and leaves platform networking, routing, observability, and image preparation to the host.

| Workload | Supported? | Notes |
|---|---:|---|
| Normal language runtimes | Yes | Go, Node.js, Python, JVM, Rust, Ruby, PHP, and C/C++ apps should work when the rootfs has the right userspace. |
| Realtime APIs | Yes | HTTP, WebSocket, and raw TCP apps are fine; latency depends more on host scheduling, networking, and app design. |
| Headless multiplayer/game servers | Yes | UDP works through `INET`; host routing and firewall policy must support it. |
| CPU inference | Yes | Kernel support is fine; CPU flags, memory, model files, and userspace libraries matter more. |
| Databases | Yes | Kernel is okay for Postgres-like and Redis-like workloads; durable volumes, fsync behavior, backups, and IOPS isolation are the real production work. |
| Brokers/queues | Yes | NATS, Redis, RabbitMQ, JVM brokers, and similar services should be fine with proper memory and storage sizing. |
| Docker/containerd inside guest | No | Intentionally unsupported; SpaceScale should prepare OCI/rootfs outside the guest. |
| FUSE apps | No | `FUSE_FS` is disabled in the default guest. |
| VPN/TUN workloads | No | `TUN` is disabled; guest-side VPNs are not part of the default model. |
| Guest firewall/iptables | No | Netfilter, nftables, and iptables are disabled by design; host policy owns this. |
| Guest eBPF/deep tracing | No | Host eBPF owns platform monitoring, routing, and policy. `BPF_SYSCALL`, BPF JIT, kprobes, uprobes, ftrace, tracefs, and BTF stay off in the default guest. |
| GPU inference | No | GPU support needs a separate architecture decision; it is not a default Firecracker app-guest kernel feature. |

## Boot And Scheduling

Required flags:

```text
HYPERVISOR_GUEST
PARAVIRT
KVM_GUEST
KVM_CLOCK
SMP
HIGH_RES_TIMERS
NO_HZ_IDLE
HZ_1000
PREEMPT_DYNAMIC
```

Why:

- `HYPERVISOR_GUEST`, `PARAVIRT`, `KVM_GUEST`, and `KVM_CLOCK` keep the kernel tuned for KVM/Firecracker instead of bare metal.
- `SMP` supports multi-vCPU shapes.
- `HIGH_RES_TIMERS`, `HZ_1000`, and `PREEMPT_DYNAMIC` are useful for latency-sensitive app runtimes, brokers, trading systems, and inference servers.
- `NO_HZ_IDLE` keeps idle guests cheaper without changing the app-visible model.

## Core Filesystems

Required flags:

```text
PROC_FS
PROC_SYSCTL
SYSFS
TMPFS
TMPFS_POSIX_ACL
TMPFS_XATTR
DEVTMPFS
DEVTMPFS_MOUNT
UNIX98_PTYS
EXT4_FS
EXT4_FS_POSIX_ACL
EXT4_FS_SECURITY
FS_POSIX_ACL
```

Why:

- `PROC_FS` and `SYSFS` provide the normal Linux runtime view expected by `scoutd`, language runtimes, databases, and diagnostics.
- `TMPFS` backs `/dev/shm`, `/run`, and `/tmp`; this is required before `scoutd` can treat the guest as ready.
- `UNIX98_PTYS` builds the Linux 6.1 `devpts` filesystem (`fs/devpts`) and supports pseudo-terminals for process supervision, shells, debuggers, and runtimes that expect ptys.
- `DEVTMPFS` and `DEVTMPFS_MOUNT` provide `/dev` without requiring userspace device management.
- `EXT4_FS` mounts the scoutd rootfs and later app root filesystems.
- ACL and security xattr support preserve normal Linux filesystem metadata for app runtimes and future policy enforcement.

## Firecracker Devices

Required flags:

```text
VIRTIO
VIRTIO_MMIO
VIRTIO_MMIO_CMDLINE_DEVICES
VIRTIO_BLK
VIRTIO_NET
VSOCKETS
VIRTIO_VSOCKETS
VIRTIO_VSOCKETS_COMMON
SERIAL_8250
SERIAL_8250_CONSOLE
HW_RANDOM
HW_RANDOM_VIRTIO
```

Why:

- `VIRTIO_MMIO` is the Firecracker device transport used by the current `pci=off` boot path.
- `VIRTIO_BLK` exposes `/dev/vda` for the root disk.
- `VIRTIO_NET` supports the guest network device used by app traffic.
- `VSOCKETS` and `VIRTIO_VSOCKETS` provide host/guest control and log channels for `scoutd`.
- `SERIAL_8250_CONSOLE` keeps panic and error diagnostics available through `console=ttyS0`; `quiet loglevel=3` keeps normal boot enumeration out of `jailer.log`.
- `HW_RANDOM_VIRTIO` improves entropy availability for TLS, session tokens, brokers, databases, and language runtimes.

## Networking

Required flags:

```text
NET
UNIX
INET
IPV6
PACKET
INET_DIAG
INET_TCP_DIAG
INET_UDP_DIAG
UNIX_DIAG
TCP_CONG_CUBIC
TCP_CONG_BBR
```

Why:

- `NET`, `UNIX`, `INET`, and `PACKET` are the baseline for app networking and local IPC.
- `IPV6` stays enabled because modern libraries often probe IPv6 even when the platform does not route it yet.
- `*_DIAG` options support `ss`-style socket inspection and future `scoutd` diagnostics.
- `TCP_CONG_CUBIC` is the normal baseline congestion control.
- `TCP_CONG_BBR` gives us a future tuning option for latency and throughput without making guest routing or firewalling part of the model.

## Runtime Compatibility

Required flags:

```text
BINFMT_ELF
BINFMT_SCRIPT
COREDUMP
ELF_CORE
EPOLL
EVENTFD
TIMERFD
SIGNALFD
MEMFD_CREATE
INOTIFY_USER
FANOTIFY
SYSVIPC
POSIX_MQUEUE
AIO
IO_URING
```

Why:

- `BINFMT_ELF` and `BINFMT_SCRIPT` run normal Linux binaries and scripts.
- `BINFMT_MISC` stays off in the default guest. We are not registering in-guest binary handlers for QEMU user-mode emulation, container runtimes, or custom interpreter dispatch.
- `EPOLL`, `EVENTFD`, `TIMERFD`, and `SIGNALFD` are standard primitives used by Go, Node.js, Python, JVM, Rust, databases, brokers, and high-concurrency servers.
- `MEMFD_CREATE` is used by modern runtimes and sandboxing patterns.
- `INOTIFY_USER` and `FANOTIFY` support file watchers, runtime reloads, and app diagnostics.
- `SYSVIPC` and `POSIX_MQUEUE` avoid compatibility surprises in databases, brokers, queues, and older libraries.
- `AIO` and `IO_URING` support high-performance file and network I/O paths used by modern databases and servers.

## Memory Pressure Signals

Required flags:

```text
PSI
```

Why:

- `PSI` gives `scoutd` and future control loops visibility into CPU, memory, and I/O pressure inside the guest.
- Huge pages and transparent huge pages are not required for the default guest. Add them only when a measured database, JVM, or CPU inference workload needs them.

## Workload Control

Required flags:

```text
CGROUPS
MEMCG
CGROUP_PIDS
PID_NS
UTS_NS
IPC_NS
NET_NS
```

Why:

- `CGROUPS`, `MEMCG`, and `CGROUP_PIDS` give `scoutd` enough baseline primitives to track memory and process count for one supervised app.
- `PID_NS`, `UTS_NS`, `IPC_NS`, and `NET_NS` are useful isolation primitives even when we are not running a container runtime inside the guest.
- `USER_NS` is intentionally excluded for now. SpaceScale runs one app under `scoutd` as normal guest users, not rootless containers. User namespaces add attack surface and should be enabled only when a real rootless-container or sandbox requirement appears.

Enable these later only when `scoutd` actively uses them for in-guest workload limits:

```text
CGROUP_SCHED
FAIR_GROUP_SCHED
CFS_BANDWIDTH
CPUSETS
CGROUP_CPUACCT
MEMCG_SWAP
CGROUP_FREEZER
BLK_CGROUP
CGROUP_WRITEBACK
```

## Host-Side eBPF

Network monitoring, policy, and faster routing belong on the host side of the VM boundary, not inside the default guest kernel.

The intended path is:

```text
customer app inside microVM
        |
virtio-net
        |
tap device on host
        |
host eBPF / tc / XDP / routing / telemetry
        |
internet or private network
```

Why:

- Host eBPF can attach to each VM TAP device, tc ingress/egress, bridge/veth if introduced later, and eventually physical NIC XDP.
- Host eBPF can collect per-VM bytes, packets, drops, connection metadata, and billing signals without giving guests kernel programmability.
- Host eBPF can enforce SpaceScale-owned L4 policy and routing decisions around the VM boundary.
- Guest eBPF would increase default guest attack surface and should be reserved for a future privileged observability tier, not the standard PaaS app guest.

## Security Primitives

Required flags:

```text
SECCOMP
SECCOMP_FILTER
KEYS
SECURITY
SECURITYFS
```

Why:

- `SECCOMP` and `SECCOMP_FILTER` support syscall filtering for `scoutd` and app sandboxes.
- `KEYS` preserves normal Linux keyring compatibility.
- `SECURITY` and `SECURITYFS` keep the basic Linux security framework available without enabling a full policy stack.
- `SECURITY_LANDLOCK` stays off until `scoutd` has a concrete Landlock sandbox design.

## Keep Disabled For Now

Do not enable these in the default PaaS app guest profile:

```text
USER_NS
OVERLAY_FS
FUSE_FS
SQUASHFS
BLK_DEV_LOOP
TUN
VETH
DUMMY
BRIDGE
VLAN_8021Q
NETFILTER
NF_TABLES
NETFILTER_XTABLES
IP_NF_IPTABLES
IP_NF_FILTER
IP_NF_NAT
IP_NF_MANGLE
BRIDGE_NETFILTER
BPF_SYSCALL
BPF_JIT
BPF_JIT_ALWAYS_ON
BPF_UNPRIV_DEFAULT_OFF
BPF_EVENTS
BPF_LSM
CGROUP_BPF
KPROBES
UPROBES
KPROBE_EVENTS
UPROBE_EVENTS
TRACEPOINTS
TRACING
FTRACE
FUNCTION_TRACER
TRACEFS
KALLSYMS_ALL
DEBUG_INFO_BTF
HUGETLBFS
HUGETLB_PAGE
TRANSPARENT_HUGEPAGE
TRANSPARENT_HUGEPAGE_MADVISE
CGROUP_SCHED
FAIR_GROUP_SCHED
CFS_BANDWIDTH
CPUSETS
CGROUP_CPUACCT
MEMCG_SWAP
CGROUP_FREEZER
BLK_CGROUP
CGROUP_WRITEBACK
SECURITY_LANDLOCK
IKCONFIG
IKCONFIG_PROC
DRM
DRM_VIRTIO_GPU
VFIO
VFIO_PCI
SELINUX
APPARMOR
AUDIT

BINFMT_MISC
VIRTIO_CONSOLE
PPS
PTP_1588_CLOCK
RTC_CLASS
RTC_DRV_CMOS
KEXEC
KEXEC_FILE
HIBERNATION
SUSPEND
PCCARD
PCMCIA
NVRAM
SERIAL_8250_DMA
SERIAL_8250_EXTENDED
SERIAL_8250_MANY_PORTS
SERIAL_8250_SHARE_IRQ
SERIAL_8250_DETECT_IRQ
SERIAL_8250_RSA

USB
HID
INPUT
SERIO
SERIO_I8042
KEYBOARD_ATKBD
MOUSE_PS2

SOUND
SND

WIRELESS
CFG80211
RFKILL
WLAN

E100
E1000
E1000E
SKY2
R8169
USB_NET_DRIVERS

SCSI
SCSI_VIRTIO
ATA
MD
MD_AUTODETECT
BLK_DEV_DM

NFS_FS
SUNRPC
NET_9P
9P_FS
QUOTA

ACPI
PCI
PNP
VIRTIO_PCI
VIRTIO_INPUT
VGA_ARB
VGA_CONSOLE
VT
VT_CONSOLE
VT_HW_CONSOLE_BINDING
HW_CONSOLE
DUMMY_CONSOLE
AGP
IOMMU_SUPPORT
SERIAL_8250_PCI
SERIAL_8250_PNP

XFRM_USER
NETLABEL
IPV6_SIT
NETCONSOLE
NETPOLL
NET_SCHED
NET_CLS
NET_CLS_CGROUP
NET_CLS_ACT
CGROUP_NET_PRIO
CGROUP_NET_CLASSID

PERF_EVENTS_INTEL_UNCORE
PERF_EVENTS_INTEL_RAPL
PERF_EVENTS_INTEL_CSTATE
PERF_EVENTS_AMD_UNCORE
CGROUP_PERF
CGROUP_DEBUG
```

Why:

- `USER_NS` is for rootless containers and some sandboxes. We do not need it for one app under `scoutd`, and it increases kernel attack surface.
- Overlay, FUSE, squashfs, loop devices, bridges, veth, tun, VLANs, and netfilter are container-host, router, VPN, image-mounting, or guest-firewall features. SpaceScale should prepare OCI/rootfs and network policy outside the guest until product requirements say otherwise.
- eBPF syscall/JIT, tracing, kprobe/uprobe, and BTF features stay off in the default guest. Use host-side eBPF on TAP/tc/XDP for platform traffic monitoring, policy, and routing. Add guest eBPF only for a future explicit privileged tier.
- Linux 6.1 on x86 selects `CONFIG_BPF=y` and `CONFIG_PERF_EVENTS=y` from architecture defaults. That is acceptable only while `BPF_SYSCALL`, `BPF_JIT`, `CGROUP_BPF`, kprobes, uprobes, ftrace, tracefs, and BTF remain disabled.
- Linux 6.1 selects core `XFRM`, `XFRM_AH`, and `XFRM_ESP` when `IPV6=y`. That is acceptable for the default guest while `XFRM_USER`, `NETFILTER`, nftables, iptables, tc classifiers, and guest routing features remain disabled.
- Huge pages, transparent huge pages, expanded cgroup controllers, Landlock, and embedded kernel config stay off until `scoutd` or a measured workload requirement needs them.
- GPU support is not just a kernel-flag problem for this Firecracker model. Current guests boot with `pci=off`, and Firecracker does not expose normal GPU passthrough as a simple default app-guest feature. Future GPU support should be a separate architecture decision, not hidden in this kernel profile.
- SELinux, AppArmor, and audit add policy/runtime complexity that the first `scoutd` app guest does not need.
- `BINFMT_MISC` is for in-guest binary handler registration, commonly QEMU user-mode or custom extension/magic dispatch. Normal ELF binaries and shebang scripts already work without it.
- `VIRTIO_CONSOLE` is unnecessary while the guest uses serial only for early panic/error output and vsock for `scoutd` control/log channels.
- `PPS`, `PTP_1588_CLOCK`, `RTC_CLASS`, and `RTC_DRV_CMOS` are not needed for the default app guest. KVM clock is enough for current guests; low-level x86 CMOS accesses may still appear in Firecracker diagnostics even when the guest-facing RTC class is disabled.
- Kexec, hibernation, suspend, PCMCIA/PCCARD, and NVRAM are host or physical-machine features outside the Firecracker app guest model.
- Keep `SERIAL_8250` and `SERIAL_8250_CONSOLE`, but keep only one runtime UART and leave 8250 extended probing off. That preserves the `ttyS0` early panic/error path without probing extra legacy serial ports.
- USB, HID, keyboard/mouse, sound, wireless, physical NIC, SCSI, ATA, RAID, device mapper, NFS, 9p, ACPI, PCI, PNP, VGA, IOMMU, XFRM, netconsole, and traffic-control classifier features are generic distro guest hardware or routing features. The SpaceScale guest uses `pci=off acpi=off`, virtio-mmio block/network/vsock, and host-owned routing.

## Build Notes

Start from `x86_64_defconfig` plus `kernel/configs/kvm_guest.config`, then apply this profile and the disabled feature list with `scripts/config`. Set `EXPERT=y` so virtual terminal, input, and VGA console can be disabled while keeping `TTY`, `SERIAL_8250`, and `SERIAL_8250_CONSOLE`. Run `make ARCH=x86_64 olddefconfig` and verify the final `.config`; some x86 architecture defaults like `CONFIG_BPF=y`, `CONFIG_PERF_EVENTS=y`, and IPv6-selected core XFRM may remain selected while the guest eBPF syscall/JIT/tracing, netfilter, and routing features stay disabled.

Every required option above should be built in as `=y`. If an option is unavailable in the chosen kernel version, document why before dropping it.

Before building, verify the critical set:

```bash
grep -E 'CONFIG_(TMPFS|UNIX98_PTYS|PROC_FS|SYSFS|DEVTMPFS|DEVTMPFS_MOUNT|EXT4_FS|VIRTIO_MMIO|VIRTIO_MMIO_CMDLINE_DEVICES|VIRTIO_BLK|VIRTIO_NET|VIRTIO_VSOCKETS|AIO|IO_URING|SYSVIPC|POSIX_MQUEUE|SECCOMP|MEMCG|CGROUP_PIDS|PID_NS|NET_NS)=' .config
```

Build the boot artifact:

```bash
make ARCH=x86_64 -j"$(nproc)" vmlinux
```

After booting a VM, verify the running guest config from inside the guest:

```bash
uname -a
```

Keep the exact `.config` file as a sidecar artifact next to each promoted `vmlinux` object. The default guest does not embed `IKCONFIG`, so runtime verification should compare the promoted object name and recorded SHA256 with the saved build config instead of relying on `/proc/config.gz`.
