// Package microvm owns the host-side Firecracker boot building blocks for
// SpaceScale workloads.
//
// The package has one job: prepare enough local host state to boot a minimal
// scoutd guest and prove that it reached user space by receiving scoutd's hello
// frame over virtio-vsock. It does not own placement policy or NATS routing.
// Those live above this package in the workload and placement layers.
//
// The mental model has three filesystem views:
//
//   - scaled runs on the normal host filesystem.
//   - Firecracker runs after jailer chroot, so its / is the jail root.
//   - scoutd runs inside the guest VM and talks back over vsock.
//
// There is one real microVM identity: the control-plane microVM ID. The same ID
// is used in both the outer SpaceScale workspace and the jailer directory. We do
// not create a second local VM ID.
//
// The outer workspace stores host-managed VM files:
//
//	/var/lib/spacescale/microvms/<microvm-id>
//
// The copied scoutd boot disk lives there:
//
//	/var/lib/spacescale/microvms/<microvm-id>/rootfs.ext4
//
// The rootfs is a platform-managed boot image. The launcher copies the template
// as-is and does not expose root disk sizing as a launch request field. Later
// OCI workload scratch space or durable data should be modeled separately from
// this scoutd boot disk.
//
// The jailer root is separate so Firecracker runtime socket paths stay short:
//
//	/var/lib/spacescale/j/<firecracker-binary-name>/<microvm-id>/root
//
// After jailer chroot, Firecracker sees that final root directory as /. This is
// why runtime files that Firecracker opens, such as api.sock, v.sock, and fc.log,
// live inside the jail root.
//
// Files inside the jail root have two path views. For example, the vsock base is
// one file but has two names:
//
//	scaled host path:
//	  /var/lib/spacescale/j/<firecracker-binary-name>/<microvm-id>/root/v.sock
//
//	Firecracker chroot path:
//	  v.sock
//
// Workspace stores only the identity and root directories. Methods such as
// VSockHostPath and VSockPathInJail derive the two views from those roots. That
// keeps a single source of truth while still making the chroot boundary explicit.
//
// The package pieces fit together like this:
//
//   - workspace.go calculates and cleans per-VM host paths.
//   - rootfs.go copies the shared scoutd rootfs template into the VM workspace.
//   - cid.go allocates the guest vsock Context ID; CID 2 is the host, guests
//     start at CID 3.
//   - vsock.go opens host Unix listeners for scoutd's control and log channels.
//   - protocol.go validates scoutd's binary hello frame.
//
// launcher.go glues those pieces together in that order: create workspace, copy
// rootfs, allocate CID, open vsock listeners, start Firecracker, then wait for
// scoutd hello.
package microvm
