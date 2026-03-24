package sysinfo

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const memInfoPath = "/proc/meminfo"

var (
	ErrMemTotalNotFound     = errors.New("MemTotal not found")
	ErrMemAvailableNotFound = errors.New("MemAvailable not found")
)

type MemoryStats struct {
	TotalMB     uint64
	AvailableMB uint64
}

// ReadMemoryStats parses the kernel's memory allocation ledger to determine
// the true physical RAM capacity of the bare-metal node.
//
// Hardware Mechanics (The Linux Cache Paradox):
// Linux aggressively uses empty RAM to cache disk operations. Therefore,
// "MemFree" represents strictly unused memory, which is a deceptively low number.
// "MemAvailable" represents the absolute capacity we can safely allocate to
// Firecracker microVMs before triggering the OOM (Out-Of-Memory) killer.
// The kernel will instantly drop its disk caches to satisfy "MemAvailable" demands.
//
// Performance Optimization:
// This function uses a buffered I/O scanner to read the virtual /proc/meminfo
// file line-by-line. Because MemTotal and MemAvailable are typically located
// in the first three lines, the scanner halts and returns immediately upon
// finding both, ensuring this sensor consumes virtually zero CPU overhead.
func ReadMemoryStats() (MemoryStats, error) {
	file, err := os.Open(memInfoPath)
	if err != nil {
		return MemoryStats{}, fmt.Errorf("open %s: %w", memInfoPath, err)
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	var stats MemoryStats
	var foundTotal bool
	var foundAvailable bool

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {

		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			value, err := parseMemInfoKBLine(line, "MemTotal:")
			if err != nil {
				return MemoryStats{}, err
			}
			stats.TotalMB = value
			foundTotal = true

		case strings.HasPrefix(line, "MemAvailable:"):
			value, err := parseMemInfoKBLine(line, "MemAvailable:")
			if err != nil {
				return MemoryStats{}, err
			}
			stats.AvailableMB = value
			foundAvailable = true
		}

		if foundTotal && foundAvailable {
			return stats, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return MemoryStats{}, fmt.Errorf("scan %s: %w", memInfoPath, err)
	}

	if !foundTotal {
		return MemoryStats{}, ErrMemTotalNotFound
	}

	if !foundAvailable {
		return MemoryStats{}, ErrMemAvailableNotFound
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
