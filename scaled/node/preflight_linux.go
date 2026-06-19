package node

import (
	"bufio"
	"context"
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
	machineIDPath  = "/etc/machine-id"

	// FirecrackerJailerAccountName is the dedicated Linux account used for
	// jailed Firecracker VMM processes.
	FirecrackerJailerAccountName = "spacescale-firecracker"

	nologinShellPath = "/usr/sbin/nologin"

	// Golden image paths — the node image is responsible for placing the
	// correct binaries and guest files at these fixed locations.
	defaultFirecrackerPath = "/usr/bin/firecracker"
	defaultJailerPath      = "/usr/bin/jailer"
	defaultKernelPath      = "/var/lib/spacescale/golden/vmlinux"
	defaultRootFSPath = "/var/lib/spacescale/golden/rootfs.ext4"
)

const (
	// PRSchedCore is the prctl operation number for Linux core scheduling (PR_SCHED_CORE).
	PRSchedCore = 62

	// PRSchedCoreGet queries the core scheduling cookie for a given process.
	PRSchedCoreGet = 0

	// PRSchedCoreCreate creates a new unique core scheduling cookie and assigns it to a process.
	PRSchedCoreCreate = 1

	// PRSchedCoreShareTo copies the caller's core scheduling cookie to another process.
	PRSchedCoreShareTo = 2

	// PidTypeTGID targets the thread group (all threads in a process) for prctl core scheduling ops.
	PidTypeTGID = 0

	// pidTypePGID targets a process group. Used internally by the preflight probe.
	pidTypePGID = 1
)

var (
	errMachineIDNotFound = errors.New("machine id not found")
	errInvalidIdentity   = errors.New("invalid node identity")
)

// Identity is the persistent node identity provisioned during initial boot.
type Identity struct {
	NodeID string
	Region string
}

// RuntimePaths are the host-local binaries and guest images baked into the node image.
type RuntimePaths struct {
	FirecrackerPath string
	JailerPath      string
	KernelPath      string
	RootFSPath      string
}

// Info bundles everything the workload subsystem and downstream components need
// about the host. Collected once at startup by Collect.
type Info struct {
	RuntimePaths   RuntimePaths
	JailerIdentity FirecrackerJailerIdentity
	Snapshot       Snapshot
	Identity       Identity
}

// Collect gathers host facts, validates runtime paths, loads identity, and runs
// node preflight. Call once at startup and pass the result to workload.Start.
func Collect(ctx context.Context, logger *slog.Logger) (Info, error) {
	runtimePaths, err := validateRuntimePaths()
	if err != nil {
		return Info{}, err
	}
	jailerIdentity, err := preflight(ctx, logger)
	if err != nil {
		return Info{}, err
	}
	snapshot, err := readSnapshot(bootIDPath, memInfoPath, cpuTopologyCoreIDGlob, rootMountDir)
	if err != nil {
		return Info{}, err
	}
	identity, err := loadIdentity(machineIDPath, os.Getenv("SPACESCALE_REGION"))
	if err != nil {
		return Info{}, err
	}

	return Info{
		RuntimePaths:   runtimePaths,
		JailerIdentity: jailerIdentity,
		Snapshot:       snapshot,
		Identity:       identity,
	}, nil
}

func loadIdentity(machineIDPath, region string) (Identity, error) {
	raw, err := os.ReadFile(machineIDPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Identity{}, errMachineIDNotFound
		}

		return Identity{}, fmt.Errorf("read machine id: %w", err)
	}
	identity := Identity{
		NodeID: strings.TrimSpace(string(raw)),
		Region: strings.TrimSpace(region),
	}
	if identity.NodeID == "" || identity.Region == "" {
		return Identity{}, errInvalidIdentity
	}

	return identity, nil
}

// FirecrackerJailerIdentity is the numeric Linux identity for the dedicated
// non-root account that jailed Firecracker processes run as.
type FirecrackerJailerIdentity struct {
	UID int
	GID int
}

// preflight prepares the host before scaled joins the workload fabric. It returns
// the jailer identity that later Firecracker launches should use.
func preflight(ctx context.Context, logger *slog.Logger) (FirecrackerJailerIdentity, error) {
	logger = logger.With("component", "preflight")

	if err := ensureKVM(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	jailerIdentity, err := ensureFirecrackerJailerAccount(ctx)
	if err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if err := disableSwap(ctx); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if err := disableKSM(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	if err := ensureCoreSchedulingSupport(); err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	logger.Info("node preflight ready",
		"kvm", kvmDevicePath,
		"jailer_user", FirecrackerJailerAccountName,
		"jailer_uid", jailerIdentity.UID,
		"jailer_gid", jailerIdentity.GID,
		"swap", "off",
		"ksm", "off",
		"smt", "on (core-scheduled)",
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
func disableSwap(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "swapoff", "-a").Run(); err != nil {
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
	defer func() { _ = file.Close() }()

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

		device := strings.Fields(line)[0]
		return fmt.Errorf("system preflight: swap still enabled on %q (run swapoff %s)", device, device)
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

// ensureCoreSchedulingSupport verifies the kernel supports PR_SCHED_CORE.
// With core scheduling enabled, SMT remains on but the kernel guarantees
// sibling threads only run tasks from the same trust group simultaneously.
func ensureCoreSchedulingSupport() error {
	current, err := readSysfsValue(smtControlPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: %s not found (ensure kernel 4.19+ and SMT support)", smtControlPath)
		}

		return fmt.Errorf("system preflight: read smt state: %w", err)
	}

	// SMT must be "on" for core scheduling to be useful.
	switch current {
	case "on":
		// Already enabled, nothing to do.
	case "off", "forceoff":
		if err := writeSysfsValue(smtControlPath, "on"); err != nil {
			return fmt.Errorf("system preflight: enable smt: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
		}
	case "notsupported":
		// Single-threaded CPU. Core scheduling is a no-op but harmless.
		return nil
	}

	// Probe whether the kernel supports PR_SCHED_CORE by attempting a no-op
	// query on the current process.
	_, _, errno := syscall.RawSyscall6(
		syscall.SYS_PRCTL,
		PRSchedCore,
		PRSchedCoreGet,
		0, // pid 0 = self
		pidTypePGID,
		0,
		0,
	)
	if errors.Is(errno, syscall.EINVAL) {
		return errors.New("system preflight: kernel does not support PR_SCHED_CORE (require Linux 5.14+ with CONFIG_SCHED_CORE=y)")
	}

	return nil
}

// ensureFirecrackerJailerAccount makes sure the dedicated Firecracker jailer
// Linux account exists and returns its UID/GID.
func ensureFirecrackerJailerAccount(ctx context.Context) (FirecrackerJailerIdentity, error) {
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
		if _, ok := errors.AsType[user.UnknownUserError](err); !ok {
			return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: lookup firecracker jailer user: %w", err)
		}
		if err := createFirecrackerJailerUser(ctx, kvmGID); err != nil {
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

func createFirecrackerJailerUser(ctx context.Context, kvmGID int) error {
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
	output, err := exec.CommandContext(ctx, "useradd", args...).CombinedOutput()
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

// validateRuntimePaths verifies the golden-image binaries and guest files exist
// at their fixed host locations.
func validateRuntimePaths() (RuntimePaths, error) {
	paths := RuntimePaths{
		FirecrackerPath: defaultFirecrackerPath,
		JailerPath:      defaultJailerPath,
		KernelPath:      defaultKernelPath,
		RootFSPath:      defaultRootFSPath,
	}

	if err := validateRuntimePath("firecracker", paths.FirecrackerPath, true); err != nil {
		return RuntimePaths{}, err
	}
	if err := validateRuntimePath("jailer", paths.JailerPath, true); err != nil {
		return RuntimePaths{}, err
	}
	if err := validateRuntimePath("kernel", paths.KernelPath, false); err != nil {
		return RuntimePaths{}, err
	}
	if err := validateRuntimePath("rootfs", paths.RootFSPath, false); err != nil {
		return RuntimePaths{}, err
	}

	return paths, nil
}

func validateRuntimePath(name, path string, executable bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("runtime %s path %q: %w", name, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("runtime %s path %q is a directory", name, path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("runtime %s path %q is empty", name, path)
	}
	if executable && info.Mode()&0o111 == 0 {
		return fmt.Errorf("runtime %s path %q is not executable", name, path)
	}

	return nil
}
