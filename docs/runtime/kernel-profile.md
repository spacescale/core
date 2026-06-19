# SpaceScale Guest Kernel Profile

Firecracker microVM guest kernel for SpaceScale. Boots into `guestd` as PID 1.

## Version

Linux `6.1.x` LTS.

## Build Rules

- Build required features as `=y`, not loadable modules
- `CONFIG_MODULES` disabled, `nomodule` on boot cmdline, no `/lib/modules` in rootfs
- Start from `x86_64_defconfig` + `kernel/configs/kvm_guest.config`, then apply this profile

## Boot and Scheduling

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

- `HYPERVISOR_GUEST`, `PARAVIRT`, `KVM_GUEST`, `KVM_CLOCK`: KVM/Firecracker paravirt support
- `SMP`: Multi-vCPU support
- `HIGH_RES_TIMERS`, `HZ_1000`, `PREEMPT_DYNAMIC`: Low-latency scheduling for brokers, APIs, inference
- `NO_HZ_IDLE`: Cheaper idle guests

## Core Filesystems

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
EROFS_FS
```

- `PROC_FS`, `SYSFS`: Standard Linux runtime view for guestd and workloads
- `TMPFS`: Backs `/dev/shm`, `/run`, `/tmp`
- `DEVTMPFS`, `DEVTMPFS_MOUNT`: Auto-populated `/dev`
- `UNIX98_PTYS`: Pseudo-terminals for process supervision and shells
- `EXT4_FS`: guestd rootfs and workload root filesystems
- `EROFS_FS`: Read-only workload image attachment (see issue #36)
- ACL and xattr support: Normal Linux filesystem metadata

## Firecracker Devices

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

- `VIRTIO_MMIO`: Firecracker device transport (`pci=off` boot path)
- `VIRTIO_BLK`: Root disk at `/dev/vda`
- `VIRTIO_NET`: Guest network device
- `VSOCKETS`, `VIRTIO_VSOCKETS`: Host/guest control and log channels for guestd
- `SERIAL_8250_CONSOLE`: Panic/error output via `console=ttyS0`
- `HW_RANDOM_VIRTIO`: Entropy for TLS, session tokens, databases

## Networking

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

- `NET`, `UNIX`, `INET`, `PACKET`: Baseline networking and local IPC
- `IPV6`: Modern libraries probe IPv6 even when unrouted
- `*_DIAG`: `ss`-style socket inspection for guestd diagnostics
- `TCP_CONG_CUBIC`: Default congestion control
- `TCP_CONG_BBR`: Optional tuning for latency-sensitive workloads

## Runtime Compatibility

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

- `BINFMT_ELF`, `BINFMT_SCRIPT`: Run normal Linux binaries and scripts
- `EPOLL`, `EVENTFD`, `TIMERFD`, `SIGNALFD`: Standard async primitives for Go, Node.js, Python, JVM, Rust
- `MEMFD_CREATE`: Modern runtimes and sandboxing patterns
- `INOTIFY_USER`, `FANOTIFY`: File watchers and runtime reloads
- `SYSVIPC`, `POSIX_MQUEUE`: Database and broker compatibility
- `AIO`, `IO_URING`: High-performance I/O for databases and servers

## Memory Pressure

```text
PSI
```

- `PSI`: CPU, memory, and I/O pressure visibility for guestd control loops

## Workload Control

```text
CGROUPS
MEMCG
CGROUP_PIDS
PID_NS
UTS_NS
IPC_NS
NET_NS
```

- `CGROUPS`, `MEMCG`, `CGROUP_PIDS`: Memory and process tracking for supervised workloads
- `PID_NS`, `UTS_NS`, `IPC_NS`, `NET_NS`: Isolation primitives for guestd

## Security

```text
SECCOMP
SECCOMP_FILTER
KEYS
SECURITY
SECURITYFS
```

- `SECCOMP`, `SECCOMP_FILTER`: Syscall filtering for guestd and workload sandboxes
- `KEYS`: Linux keyring compatibility
- `SECURITY`, `SECURITYFS`: Basic security framework availability

## Build

```bash
# Start from defconfig + kvm guest, then apply this profile with scripts/config
make ARCH=x86_64 defconfig
scripts/config -e kvm_guest.config
# Apply all required flags above as -e CONFIG_<FLAG>=y

# Verify critical set
grep -E 'CONFIG_(TMPFS|UNIX98_PTYS|PROC_FS|SYSFS|DEVTMPFS|DEVTMPFS_MOUNT|EXT4_FS|EROFS_FS|VIRTIO_MMIO|VIRTIO_BLK|VIRTIO_NET|VIRTIO_VSOCKETS|AIO|IO_URING|SECCOMP|MEMCG|CGROUP_PIDS|PID_NS|NET_NS)=' .config

# Build
make ARCH=x86_64 -j"$(nproc)" vmlinux
```
