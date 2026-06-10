// Package node reads Linux host facts and prepares host state for scaled
// startup.
//
// The package is the startup/preflight boundary for KVM, swap, KSM, SMT,
// golden-image runtime files, fixed Firecracker jailer account setup, physical
// core counting, memory, disk, and boot ID discovery. Downstream packages
// receive resolved values instead of repeating host identity or readiness checks.
package node

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	bootIDPath = "/proc/sys/kernel/random/boot_id"

	memInfoPath           = "/proc/meminfo"
	rootMountDir          = "/"
	cpuTopologyCoreIDGlob = "/sys/devices/system/cpu/cpu[0-9]*/topology/core_id"
)

// Snapshot represents the physical and logical boundaries of the edge node at
// the moment of boot. It is used to initialize the deterministic Capacity ledger.
type Snapshot struct {
	BootID          string
	TotalCores      uint32
	TotalRAMMb      uint64
	TotalDiskMb     uint64
	AvailableDiskMb uint64
}

type memoryStats struct {
	TotalMb uint64
}

type diskStats struct {
	TotalMb     uint64
	AvailableMb uint64
}

func readSnapshot(bootIDPath, memInfoPath, cpuTopologyCoreIDGlob, rootMountDir string) (Snapshot, error) {
	bootID, err := readBootID(bootIDPath)
	if err != nil {
		return Snapshot{}, err
	}

	totalCores, err := readPhysicalCoreCount(cpuTopologyCoreIDGlob)
	if err != nil {
		return Snapshot{}, err
	}

	memory, err := readMemoryStats(memInfoPath)
	if err != nil {
		return Snapshot{}, err
	}

	disk, err := readDiskStats(rootMountDir)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		BootID:          bootID,
		TotalCores:      totalCores,
		TotalRAMMb:      memory.TotalMb,
		TotalDiskMb:     disk.TotalMb,
		AvailableDiskMb: disk.AvailableMb,
	}, nil
}

// readBootID retrieves the current kernel boot identifier.
func readBootID(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read boot id: %w", err)
	}

	bootID := strings.TrimSpace(string(raw))
	if bootID == "" {
		return "", errors.New("read boot id: empty value")
	}

	return bootID, nil
}

// readMemoryStats returns installed RAM, not transient available memory.
func readMemoryStats(path string) (memoryStats, error) {
	file, err := os.Open(path)
	if err != nil {
		return memoryStats{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	var stats memoryStats
	var foundTotal bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "MemTotal:") {
			value, err := parseMemInfoKBLine(line, "MemTotal:")
			if err != nil {
				return memoryStats{}, err
			}
			stats.TotalMb = value
			foundTotal = true

			break
		}
	}

	if err := scanner.Err(); err != nil {
		return memoryStats{}, fmt.Errorf("scan %s: %w", path, err)
	}
	if !foundTotal {
		return memoryStats{}, fmt.Errorf("scan %s: MemTotal not found", path)
	}

	return stats, nil
}

func parseMemInfoKBLine(line string, label string) (uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, fmt.Errorf("invalid %s line: %q", strings.TrimSuffix(label, ":"), line)
	}
	if fields[0] != label {
		return 0, fmt.Errorf("unexpected meminfo label %q", fields[0])
	}
	if !strings.EqualFold(fields[2], "kB") {
		return 0, fmt.Errorf("unexpected %s unit %q", strings.TrimSuffix(label, ":"), fields[2])
	}

	kb, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s value: %w", strings.TrimSuffix(label, ":"), err)
	}

	return kb / 1024, nil
}

func readDiskStats(path string) (diskStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskStats{}, fmt.Errorf("statfs %q: %w", path, err)
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availableBytes := stat.Bavail * uint64(stat.Bsize)

	return diskStats{
		TotalMb:     totalBytes / 1024 / 1024,
		AvailableMb: availableBytes / 1024 / 1024,
	}, nil
}

// readPhysicalCoreCount counts physical cores by de-duplicating sysfs topology.
func readPhysicalCoreCount(glob string) (uint32, error) {
	paths, err := filepath.Glob(glob)
	if err != nil {
		return 0, fmt.Errorf("glob cpu topology: %w", err)
	}
	if len(paths) == 0 {
		// If no topology is found, we cannot accurately determine physical
		// boundaries, so we fail the pre-flight check for security.
		return 0, errors.New("glob cpu topology: no cpu topology found (ensure sysfs is mounted)")
	}

	cores := make(map[string]struct{}, len(paths))
	for _, corePath := range paths {
		coreID, err := readTopologyValue(corePath)
		if err != nil {
			return 0, err
		}

		packagePath := strings.TrimSuffix(corePath, "core_id") + "physical_package_id"
		packageID, err := readTopologyValue(packagePath)
		if err != nil {
			return 0, err
		}

		cores[packageID+":"+coreID] = struct{}{}
	}

	if len(cores) == 0 {
		return 0, errors.New("read physical core count: no cores found (potential sysfs corruption)")
	}

	return uint32(len(cores)), nil
}

func readTopologyValue(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read topology %q: %w", path, err)
	}

	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", fmt.Errorf("read topology %q: empty value", path)
	}

	return value, nil
}
