// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package system

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

const (
	kvmDevicePath  = "/dev/kvm"
	ksmRunPath     = "/sys/kernel/mm/ksm/run"
	smtControlPath = "/sys/devices/system/cpu/smt/control"
	procSwapsPath  = "/proc/swaps"

	// FirecrackerJailerAccountName is the dedicated Linux account used for
	// jailed Firecracker VMM processes.
	FirecrackerJailerAccountName = "spacescale-firecracker"

	nologinShellPath = "/usr/sbin/nologin"
)

// FirecrackerJailerIdentity is the numeric Linux identity for the dedicated
// non-root account that jailed Firecracker processes run as.
type FirecrackerJailerIdentity struct {
	// UID is the Linux user ID passed into the Firecracker jailer config.
	UID int

	// GID is the Linux group ID passed into the Firecracker jailer config.
	GID int
}

// Preflight prepares the host before scaled joins the workload fabric. It returns
// the jailer identity that later Firecracker launches should use.
func Preflight(logger *slog.Logger) (FirecrackerJailerIdentity, error) {
	logger = logger.With("component", "system")

	if err := ensureKVM(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	jailerIdentity, err := EnsureFirecrackerJailerAccount()
	if err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if err := disableSwap(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if err := disableKSM(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if err := disableSMT(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	logger.Info("system preflight ready",
		"kvm", kvmDevicePath,
		"jailer_user", FirecrackerJailerAccountName,
		"jailer_uid", jailerIdentity.UID,
		"jailer_gid", jailerIdentity.GID,
		"swap", "off",
		"ksm", "off",
		"smt", "off",
	)
	return jailerIdentity, nil
}

// ensureKVM verifies that scaled can open the KVM device Firecracker needs.
func ensureKVM() error {
	file, err := os.OpenFile(kvmDevicePath, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: %s not found (virtualization disabled in bios or vps)", kvmDevicePath)
		}
		return fmt.Errorf("system preflight: open %s: %w (check kvm group permissions)", kvmDevicePath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("system preflight: close %s: %w", kvmDevicePath, err)
	}

	return nil
}

// disableSwap keeps guest memory off host swap.
func disableSwap() error {
	if err := exec.Command("swapoff", "-a").Run(); err != nil {
		return fmt.Errorf("system preflight: disable swap: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
	}

	if err := ensureSwapDisabled(); err != nil {
		return err
	}

	return nil
}

// ensureSwapDisabled verifies that /proc/swaps has no active devices.
func ensureSwapDisabled() error {
	file, err := os.Open(procSwapsPath)
	if err != nil {
		return fmt.Errorf("system preflight: open %s: %w", procSwapsPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNumber++

		if lineNumber == 1 {
			continue
		}

		return fmt.Errorf("system preflight: swap still enabled (detected active device in /proc/swaps)")
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("system preflight: scan %s: %w", procSwapsPath, err)
	}

	return nil
}

// disableKSM disables page merging between tenants.
func disableKSM() error {
	_, err := os.Stat(ksmRunPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("system preflight: stat %s: %w", ksmRunPath, err)
	}

	if err := writeSysfsValue(ksmRunPath, "0"); err != nil {
		return fmt.Errorf("system preflight: disable ksm: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
	}

	value, err := readSysfsValue(ksmRunPath)
	if err != nil {
		return fmt.Errorf("system preflight: read ksm state: %w", err)
	}

	if value != "0" {
		return fmt.Errorf("system preflight: ksm still enabled with value %q (expected 0)", value)
	}

	return nil
}

// disableSMT keeps capacity based on physical cores only.
func disableSMT() error {
	current, err := readSysfsValue(smtControlPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: %s not found (ensure kernel 4.19+ and SMT support)", smtControlPath)
		}
		return fmt.Errorf("system preflight: read smt state: %w", err)
	}

	switch current {
	case "off", "forceoff", "notsupported":
		return nil
	}

	if err := writeSysfsValue(smtControlPath, "off"); err != nil {
		return fmt.Errorf("system preflight: disable smt: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
	}

	current, err = readSysfsValue(smtControlPath)
	if err != nil {
		return fmt.Errorf("system preflight: read smt state: %w", err)
	}

	switch current {
	case "off", "forceoff", "notsupported":
		return nil
	default:
		return fmt.Errorf("system preflight: smt still enabled with value %q (expected off/forceoff)", current)
	}
}

// EnsureFirecrackerJailerAccount makes sure the dedicated Firecracker jailer
// Linux account exists and returns its UID/GID.
func EnsureFirecrackerJailerAccount() (FirecrackerJailerIdentity, error) {
	kvmGID, err := kvmDeviceGID()
	if err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if kvmGID <= 0 {
		return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: %s group must be non-root", kvmDevicePath)
	}

	if _, err := user.LookupGroupId(strconv.Itoa(kvmGID)); err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: lookup group for %s gid %d: %w", kvmDevicePath, kvmGID, err)
	}

	if _, err := user.Lookup(FirecrackerJailerAccountName); err != nil {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: lookup firecracker jailer user: %w", err)
		}
		if err := createFirecrackerJailerUser(kvmGID); err != nil {
			return FirecrackerJailerIdentity{}, err
		}
	}

	return firecrackerJailerIdentity(kvmGID)
}

func firecrackerJailerIdentity(kvmGID int) (FirecrackerJailerIdentity, error) {
	u, err := user.Lookup(FirecrackerJailerAccountName)
	if err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("lookup firecracker jailer user %q: %w", FirecrackerJailerAccountName, err)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("parse firecracker jailer uid %q: %w", u.Uid, err)
	}

	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("parse firecracker jailer gid %q: %w", u.Gid, err)
	}

	if uid <= 0 {
		return FirecrackerJailerIdentity{}, fmt.Errorf("firecracker jailer user %q must be non-root", FirecrackerJailerAccountName)
	}

	if gid != kvmGID {
		return FirecrackerJailerIdentity{}, fmt.Errorf("firecracker jailer user %q gid %d does not match %s gid %d", FirecrackerJailerAccountName, gid, kvmDevicePath, kvmGID)
	}

	return FirecrackerJailerIdentity{UID: uid, GID: gid}, nil
}

func createFirecrackerJailerUser(kvmGID int) error {
	shell := nologinShellPath

	if _, err := os.Stat(shell); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: stat %s: %w", shell, err)
		}
		shell = "/bin/false"
	}

	args := []string{
		"--system",
		"--no-create-home",
		"--gid", strconv.Itoa(kvmGID),
		"--shell", shell,
		FirecrackerJailerAccountName,
	}

	output, err := exec.Command("useradd", args...).CombinedOutput()
	if err != nil {
		if _, lookupErr := user.Lookup(FirecrackerJailerAccountName); lookupErr == nil {
			return nil
		}
		if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
			return fmt.Errorf("system preflight: create firecracker jailer user: %w: %s", err, trimmed)
		}
		return fmt.Errorf("system preflight: create firecracker jailer user: %w", err)
	}
	return nil
}

// kvmDeviceGID returns the Linux group ID that owns /dev/kvm.
func kvmDeviceGID() (int, error) {
	info, err := os.Stat(kvmDevicePath)
	if err != nil {
		return 0, fmt.Errorf("system preflight: stat %s: %w", kvmDevicePath, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("system preflight: stat %s: missing linux stat metadata", kvmDevicePath)
	}

	return int(stat.Gid), nil
}

func readSysfsValue(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}
func writeSysfsValue(path string, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}
