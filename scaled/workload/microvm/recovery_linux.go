//go:build linux

package microvm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spacescale/core/scaled/node"
	"github.com/vishvananda/netlink"
)

const (
	spacescaleCgroupName = "spacescale"
	cgroupFSRoot         = "/sys/fs/cgroup"
	procFSRoot           = "/proc"

	orphanExitTimeout  = 5 * time.Second
	orphanPollInterval = 25 * time.Millisecond
)

// CleanupOrphanedIgniteState removes processes and host resources left by a
// previous scaled process. Ignite workloads are stateless, so the control plane
// will place them elsewhere after the old node heartbeat expires.
//
// Cleanup is deliberately best effort: every failure is logged and the
// remaining cleanup stages still run.
func CleanupOrphanedIgniteState(ctx context.Context, logger *slog.Logger, jailer node.FirecrackerJailerIdentity) {
	log := logger.With("component", "ignite-recovery")

	pids, discoveryErrs := discoverOrphanPIDs(
		filepath.Join(cgroupFSRoot, spacescaleCgroupName),
		procFSRoot,
		jailer.UID,
	)
	for _, err := range discoveryErrs {
		log.Warn("orphan process discovery failed", "error", err)
	}
	killOrphanPIDs(ctx, log, pids)

	if err := cleanupStaleTAPs(log); err != nil {
		log.Warn("stale tap cleanup failed", "error", err)
	}

	cleanupStatePath(log, microVMStateDir)
	cleanupStatePath(log, microVMJailerStateDir)
}



// discoverOrphanPIDs discovers the orphan PIDs.
func discoverOrphanPIDs(cgroupRoot, procRoot string, jailerUID int) ([]int, []error) {
	pids := make(map[int]struct{})
	var errs []error

	cgroupPIDs, err := pidsFromCgroupTree(cgroupRoot)
	if err != nil {
		errs = append(errs, err)
	}
	for _, pid := range cgroupPIDs {
		pids[pid] = struct{}{}
	}

	uidPIDs, err := pidsOwnedByUID(procRoot, jailerUID)
	if err != nil {
		errs = append(errs, err)
	}
	for _, pid := range uidPIDs {
		pids[pid] = struct{}{}
	}

	delete(pids, os.Getpid())

	result := make([]int, 0, len(pids))
	for pid := range pids {
		result = append(result, pid)
	}
	slices.Sort(result)

	return result, errs
}

// pidsFromCgroupTree returns the PIDs from a cgroup tree.
func pidsFromCgroupTree(root string) ([]int, error) {
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect cgroup root %s: %w", root, err)
	}

	pids := make(map[int]struct{})
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "cgroup.procs" {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, field := range strings.Fields(string(raw)) {
			pid, err := strconv.Atoi(field)
			if err != nil || pid <= 0 {
				return fmt.Errorf("parse pid %q from %s", field, path)
			}
			pids[pid] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk cgroup tree %s: %w", root, err)
	}

	result := make([]int, 0, len(pids))
	for pid := range pids {
		result = append(result, pid)
	}
	slices.Sort(result)
	return result, nil
}


// pidsOwnedByUID returns the PIDs owned by a UID.
func pidsOwnedByUID(procRoot string, uid int) ([]int, error) {
	if uid <= 0 {
		return nil, fmt.Errorf("refuse to scan unsafe jailer uid %d", uid)
	}

	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read proc root %s: %w", procRoot, err)
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}

		owned, err := processOwnedByUID(filepath.Join(procRoot, entry.Name(), "status"), uid)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
				continue
			}
			return nil, err
		}
		if owned {
			pids = append(pids, pid)
		}
	}

	slices.Sort(pids)
	return pids, nil
}


// processOwnedByUID checks if a process is owned by a UID.
func processOwnedByUID(statusPath string, uid int) (bool, error) {
	file, err := os.Open(statusPath)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "Uid:" {
			continue
		}
		realUID, err := strconv.Atoi(fields[1])
		if err != nil {
			return false, fmt.Errorf("parse uid from %s: %w", statusPath, err)
		}
		return realUID == uid, nil
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("scan %s: %w", statusPath, err)
	}
	return false, nil
}


// killOrphanPIDs kills the orphan PIDs.
func killOrphanPIDs(ctx context.Context, logger *slog.Logger, pids []int) {
	signaled := make([]int, 0, len(pids))
	for _, pid := range pids {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			if !errors.Is(err, syscall.ESRCH) {
				logger.Warn("orphan firecracker kill failed", "pid", pid, "error", err)
			}
			continue
		}
		signaled = append(signaled, pid)
		logger.Info("killed orphan firecracker process", "pid", pid)
	}

	if len(signaled) == 0 {
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, orphanExitTimeout)
	defer cancel()
	ticker := time.NewTicker(orphanPollInterval)
	defer ticker.Stop()

	for {
		remaining := livePIDs(signaled)
		if len(remaining) == 0 {
			return
		}

		select {
		case <-waitCtx.Done():
			logger.Warn("orphan firecracker processes did not exit",
				"pids", remaining,
				"error", waitCtx.Err(),
			)
			return
		case <-ticker.C:
		}
	}
}

// livePIDs returns the PIDs that are still alive.
func livePIDs(pids []int) []int {
	remaining := make([]int, 0, len(pids))
	for _, pid := range pids {
		err := syscall.Kill(pid, 0)
		if err == nil || errors.Is(err, syscall.EPERM) {
			remaining = append(remaining, pid)
		}
	}
	return remaining
}

// cleanupStaleTAPs removes stale TAPs from the host network namespace.
func cleanupStaleTAPs(logger *slog.Logger) error {
	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("list host links: %w", err)
	}

	var errs []error
	for _, link := range links {
		if !isSpaceScaleTAP(link) {
			continue
		}
		name := link.Attrs().Name
		if err := netlink.LinkDel(link); err != nil {
			errs = append(errs, fmt.Errorf("delete tap %s: %w", name, err))
			continue
		}
		logger.Info("removed stale tap", "tap", name)
	}
	return errors.Join(errs...)
}

// isSpaceScaleTAP checks if a link is a SpaceScale TAP.
func isSpaceScaleTAP(link netlink.Link) bool {
	if link == nil || link.Attrs() == nil {
		return false
	}
	attrs := link.Attrs()
	return isSpaceScaleTAPName(attrs.Name) ||
		(strings.HasPrefix(attrs.Name, "tap") && hasSpaceScaleMAC(attrs.HardwareAddr))
}

// isSpaceScaleTAPName checks if a link name is a SpaceScale TAP name.
func isSpaceScaleTAPName(name string) bool {
	const suffixLength = 11
	if len(name) != len("tap")+suffixLength || !strings.HasPrefix(name, "tap") {
		return false
	}
	for _, char := range name[len("tap"):] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// hasSpaceScaleMAC checks if a MAC address is a SpaceScale MAC address.
func hasSpaceScaleMAC(mac net.HardwareAddr) bool {
	return len(mac) >= 2 && mac[0] == 0x02 && mac[1] == 0x53
}

// cleanupStatePath removes a stale state directory if it exists.
func cleanupStatePath(logger *slog.Logger, path string) {
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Warn("inspect stale state path failed", "path", path, "error", err)
		}
		return
	}
	if err := os.RemoveAll(path); err != nil {
		logger.Warn("stale state cleanup failed", "path", path, "error", err)
		return
	}
	logger.Info("removed stale state", "path", path)
}
