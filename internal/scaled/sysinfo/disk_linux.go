package sysinfo

import (
	"fmt"
	"strings"
	"syscall"
)

// DiskStats holds the bare-metal node's NVMe storage capacity
type DiskStats struct {
	TotalMB     uint64
	AvailableMB uint64
}

// ReadDiskStats executes a raw syscall.Statfs to query the kernel's filesystem driver
// for the physical storage capacity of a given mount path.
//
// It defaults to the root partition ("/") if no path is provided.
//
// Hardware Mechanics:
// The Linux kernel does not report storage in bytes. It reports in Blocks.
// To calculate true byte capacity, we multiply the total number of blocks (stat.Blocks)
// by the physical size of each block (stat.Bsize, typically 4096 bytes).
//
// Safety Constraint:
// We explicitly use stat.Bavail (Blocks Available to unprivileged users) rather
// than stat.Bfree (Total Free Blocks). Linux reserves roughly 5% of disk space
// exclusively for the root user to prevent catastrophic system lockouts during
// 100% disk-full events. Using Bavail ensures our orchestrator respects this boundary.
func ReadDiskStats(path string) (DiskStats, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/" // the path / is usually the main linux partition
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskStats{}, fmt.Errorf("statfs %q: %w", path, err)
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availableBytes := stat.Bavail * uint64(stat.Bsize)

	return DiskStats{
		TotalMB:     totalBytes / 1024 / 1024,
		AvailableMB: availableBytes / 1024 / 1024,
	}, nil
}
