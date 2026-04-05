package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const (
	bootIDPath   = "/proc/sys/kernel/random/boot_id"
	memInfoPath  = "/proc/meminfo"
	rootMountDir = "/"
)

type Snapshot struct {
	BootID          string
	TotalCores      uint32
	TotalRamMb      uint64
	AvailableRamMb  uint64
	TotalDiskMb     uint64
	AvailableDiskMb uint64
}

type memoryStats struct {
	TotalMb     uint64
	AvailableMb uint64
}

type diskStats struct {
	TotalMb     uint64
	AvailableMb uint64
}

func Read() (Snapshot, error) {
	bootID, err := readBootID()
	if err != nil {
		return Snapshot{}, err
	}

	memory, err := readMemoryStats()
	if err != nil {
		return Snapshot{}, err
	}

	disk, err := readDiskStats(rootMountDir)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		BootID:          bootID,
		TotalCores:      uint32(runtime.NumCPU()),
		TotalRamMb:      memory.TotalMb,
		AvailableRamMb:  memory.AvailableMb,
		TotalDiskMb:     disk.TotalMb,
		AvailableDiskMb: disk.AvailableMb,
	}, nil
}

func readBootID() (string, error) {
	raw, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", fmt.Errorf("read boot id: %w", err)
	}

	bootID := strings.TrimSpace(string(raw))
	if bootID == "" {
		return "", fmt.Errorf("read boot id: empty value")
	}

	return bootID, nil
}

func readMemoryStats() (memoryStats, error) {
	file, err := os.Open(memInfoPath)
	if err != nil {
		return memoryStats{}, fmt.Errorf("open %s: %w", memInfoPath, err)
	}
	defer file.Close()

	var stats memoryStats
	var foundTotal bool
	var foundAvailable bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			value, err := parseMemInfoKBLine(line, "MemTotal:")
			if err != nil {
				return memoryStats{}, err
			}
			stats.TotalMb = value
			foundTotal = true

		case strings.HasPrefix(line, "MemAvailable:"):
			value, err := parseMemInfoKBLine(line, "MemAvailable:")
			if err != nil {
				return memoryStats{}, err
			}
			stats.AvailableMb = value
			foundAvailable = true
		}

		if foundTotal && foundAvailable {
			return stats, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return memoryStats{}, fmt.Errorf("scan %s: %w", memInfoPath, err)
	}
	if !foundTotal {
		return memoryStats{}, fmt.Errorf("scan %s: MemTotal not found", memInfoPath)
	}
	if !foundAvailable {
		return memoryStats{}, fmt.Errorf("scan %s: MemAvailable not found", memInfoPath)
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
