# Guest Rootfs Requirements

The guest rootfs is a read-only EROFS image booted by Firecracker. guestd runs as PID 1 and expects a minimal Linux root with pre-created mount points.

## Init Binary

- `guestd` static musl binary at `/init`
- No shared libraries required (statically linked)
- No dynamic linker needed

## Pre-Created Directories

These directories must exist in the EROFS image:

```text
/proc
/sys
/dev
/dev/pts
/dev/mqueue
/dev/shm
/run
/tmp
```

## Filesystem Mounts (handled by guestd at boot)

| Path | Type | Size | Notes |
|------|------|------|-------|
| /proc | proc | — | Kernel provided |
| /sys | sysfs | — | Read/write for early boot |
| /dev | devtmpfs | — | Auto-populated by kernel |
| /dev/pts | devpts | — | Pseudo-terminals |
| /dev/mqueue | mqueue | — | POSIX message queues |
| /dev/shm | tmpfs | 64M | Shared memory |
| /run | tmpfs | 16M | Runtime state, noexec |
| /tmp | tmpfs | 64M | Scratch space, executable |

## Kernel Cmdline Contract

guestd parses these from `/proc/cmdline`:

```text
guestd.ipv4=<ip>/<prefix>
guestd.gateway=<ip>
guestd.mmds=<ip>
```

Other args (`ro`, `quiet`, `console=ttyS0`, etc.) are ignored by guestd.

## Read-Only Root

The root filesystem is mounted read-only (`ro` in kernel args). All writable state is tmpfs and ephemeral. Nothing survives a VM restart.
