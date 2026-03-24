package sysinfo

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const bootIDPath = "/proc/sys/kernel/random/boot_id"

var ErrInvalidBootID = errors.New("invalid boot id")

// ReadBootID reads the Linux kernel's dynamically generated boot identifier.
//
// Hardware Mechanics:
// Upon every system boot, the Linux kernel's random number generator creates
// a unique UUID and exposes it via /proc/sys/kernel/random/boot_id. This is a
// virtual file that resides entirely in RAM, meaning reading it costs zero disk I/O.
//
// Architectural Purpose:
// In a distributed bare-metal platform, static identifiers (like MAC addresses
// or hostnames) are dangerous because they cannot detect a silent hard-reboot.
// By embedding this boot_id in every NATS heartbeat, the Control Plane can
// instantly detect if a physical node crashed and restarted (the ID will change).
// This allows the orchestrator to definitively mark the old state and any
// previously running microVMs as dead.
//
// Safety:
// The returned string is strictly trimmed of whitespace and hidden newlines (\n)
// to prevent silent message corruption during NATS routing or Protobuf serialization.
func ReadBootID() (string, error) {
	raw, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", fmt.Errorf("read boot id: %w", err)
	}
	bootID := strings.TrimSpace(string(raw))
	if bootID == "" {
		return "", ErrInvalidBootID
	}
	return bootID, nil
}
