# Guest Rootfs Requirements

The guest rootfs is a read-only EROFS image booted by Firecracker. guestd runs as PID 1 and requires a minimal Linux root with only the directories it directly uses.

## Init Binary

- `guestd` static musl binary at `/sbin/init`
- No shared libraries required (statically linked)
- No dynamic linker needed

## Pre-Created Directories

Only these directories must exist in the EROFS image:

```text
/sbin   (contains the guestd init binary)
/proc   (mount point for guestd's proc mount)
/dev    (mount point for the kernel's automatic devtmpfs mount)
```

## Filesystem Mounts

| Path | Type | Who mounts it | Why |
|------|------|---------------|-----|
| /dev | devtmpfs | Kernel (DEVTMPFS_MOUNT=y) | Auto-mounted before init runs, populates device nodes |
| /proc | proc | guestd | Required to read /proc/cmdline for bootstrap settings |

All other filesystem mounts (/sys, /dev/pts, /dev/mqueue, /dev/shm, /run, /tmp) are
workload concerns. guestd sets up the workload environment when launching a workload,
not at init time.

## Kernel Cmdline Contract

guestd parses these from `/proc/cmdline`:

```text
guestd.ipv4=<ip>/<prefix>
guestd.gateway=<ip>
guestd.mmds=<ip>
```

Other args (`ro`, `quiet`, `console=ttyS0`, etc.) are ignored by guestd.

## Read-Only Root

The root filesystem is mounted read-only (`ro` in kernel args). All writable state
is provided by the workload environment, not the base rootfs.
