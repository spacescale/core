// Package system provides the lifecycle and identity management for the edge daemon.
//
// This file implements the Linux-specific pre-flight checks required to ensure
// the hardware and kernel are correctly configured for secure, high-density
// MicroVM orchestration.
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

var (
	preflightEnsureKVM               = ensureKVM
	preflightDisableSwap             = disableSwap
	preflightDisableKSM              = disableKSM
	preflightDisableSMT              = disableSMT
	preflightEnsureFirecrackerJailer = EnsureFirecrackerJailerAccount
)

const (
	// kvmDevicePath is the primary interface for the Kernel-based Virtual Machine.
	//
	// This character device provides access to the hardware-assisted virtualization
	// features of the CPU (Intel VMX or AMD SVM). It is the foundational substrate
	// for the Firecracker VMM. If this device is missing, inaccessible, or if
	// nested virtualization is disabled in the BIOS, the edge node will be unable
	// to spawn hardened MicroVM workloads and will fail its pre-flight check.
	kvmDevicePath = "/dev/kvm"

	// ksmRunPath provides control over Kernel Samepage Merging (KSM).
	//
	// KSM is a memory-saving feature that allows the Linux kernel to identify
	// and merge identical memory pages across different MicroVM processes. In a
	// high-density environment where many guests might be running the same
	// kernel or base OS image, KSM acts as a significant "Density Multiplier."
	// It allows the platform to overcommit physical RAM by reclaiming redundant
	// pages, which is essential for hyperscale efficiency.
	ksmRunPath = "/sys/kernel/mm/ksm/run"

	// smtControlPath manages Simultaneous Multithreading (SMT) or Hyperthreading.
	//
	// In a high-security, multi-tenant cloud environment, SMT is often considered
	// a security liability due to its vulnerability to speculative execution
	// side-channel attacks. SpaceScale operates on a Physical Core Truth model
	// where we prioritize security and deterministic performance over raw thread
	// density. We use this path to verify that SMT is disabled at the kernel
	// level, ensuring that each customer workload is isolated to its own
	// physical silicon core without sharing execution resources with another guest.
	smtControlPath = "/sys/devices/system/cpu/smt/control"

	// procSwapsPath is the kernel's live source of truth for the active swap devices.
	//
	// This virtual file in the procfs provides a real-time list of all swap partitions
	// or files that the Linux kernel is currently using as "Overflow RAM." We use
	// this path during our pre-flight check to double-check the success of the
	// "swapoff -a" command. If this file is anything other than a header-only
	// empty list, the pre-flight check will fail to prevent the node from
	// running in a low-performance "swapping" state.
	procSwapsPath = "/proc/swaps"

	// FirecrackerJailerAccountName is the dedicated Linux account used for
	// jailed Firecracker VMM processes. scaled prepares the jail as root, then
	// the jailer drops Firecracker into this non-root account.
	FirecrackerJailerAccountName = "spacescale-firecracker"

	// nologinShellPath prevents the Firecracker jailer account from being used
	// for interactive shell login. The account exists only as a process identity
	// for jailed VMM execution.
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

// Preflight performs a rigorous audit of the host operating system to verify
// that all hardware virtualization and security primitives are operational.
//
// This function acts as a "Gatekeeper" during the daemon's boot sequence. It
// ensures that we do not join the regional auction fabric unless the node is
// in a hardened, performant state.
func Preflight(logger *slog.Logger) error {
	logger.Info("running system preflight")
	if err := preflightEnsureKVM(); err != nil {
		return err
	}
	logger.Info("system preflight verified kvm")

	jailerIdentity, err := preflightEnsureFirecrackerJailer()
	if err != nil {
		return err
	}
	logger.Info("system preflight ensured firecracker jailer user",
		"user", FirecrackerJailerAccountName,
		"uid", jailerIdentity.UID,
		"gid", jailerIdentity.GID,
	)

	if err := preflightDisableSwap(); err != nil {
		return err
	}
	logger.Info("system preflight disabled swap")

	if err := preflightDisableKSM(); err != nil {
		return err
	}
	logger.Info("system preflight disabled ksm")

	if err := preflightDisableSMT(); err != nil {
		return err
	}
	logger.Info("system preflight disabled smt")

	logger.Info("system preflight complete")
	return nil
}

// ensureKVM performs a "Probing Attack" on the local hardware to verify that
// the Linux kernel is ready to host MicroVMs.
func ensureKVM() error {
	// We open the KVM device with O_RDWR (Read and Write) permissions.
	// This is not because we want to read or write data to a file, but because
	// we are asking the kernel to verify that our process has the necessary
	// capabilities to manage the CPU's hardware virtualization features.
	//
	// If this open succeeds, it means:
	// 1. The KVM kernel module is loaded.
	// 2. Hardware virtualization (VT-x or AMD-V) is enabled in the BIOS.
	// 3. The scaled daemon has the correct group permissions (usually 'kvm').
	file, err := os.OpenFile(kvmDevicePath, os.O_RDWR, 0)
	if err != nil {
		// If the device node is missing entirely, we are likely running on a
		// VPS that does not support nested virtualization.
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: %s not found (virtualization disabled in bios or vps)", kvmDevicePath)
		}
		// If we get "Permission Denied", the hardware is ready but our
		// security configuration is wrong.
		return fmt.Errorf("system preflight: open %s: %w (check kvm group permissions)", kvmDevicePath, err)
	}

	// We immediately close the file descriptor. We don't need to keep it open;
	// the fact that we COULD open it is the only "Truth" we need for pre-flight.
	if err := file.Close(); err != nil {
		return fmt.Errorf("system preflight: close %s: %w", kvmDevicePath, err)
	}

	return nil
}

// disableSwap ensures the host operating system is operating in a "RAM-Only"
// high-performance mode by disabling the Linux swap subsystem.
//
// In Linux, "Swap" is a mechanism that uses the physical disk (SSD or HDD) as
// a virtual overflow area for RAM when physical memory is exhausted. While
// useful for desktop systems, swap is a reliability antipattern for
// high-density microVM orchestration for three architectural reasons:
//
// 1. Performance Determinism: Disk access is several orders of magnitude
// slower than RAM. If a customer workload is "swapped out" to disk, its
// performance will drop unpredictably, leading to "Noisy Neighbor" issues.
//
// 2. I/O Starvation: Excessive swapping can saturate the host's disk I/O
// bandwidth, negatively impacting the rootfs and volume performance of
// every other guest on the node.
//
// 3. Data Integrity: Memory pages written to swap files on physical disks
// can leave long-lived artifacts of customer data even after a VM is deleted.
func disableSwap() error {
	// We use the standard Linux "swapoff -a" command to immediately stop the
	// kernel from using any disk-backed memory. This is critical for performance
	// because swapping memory to disk can slow down a MicroVM by several
	// orders of magnitude.
	//
	// NOTE: This command only affects the current boot session. If the node
	// reboots, the OS might try to re-enable swap. We call this every time
	// our daemon starts to ensure the node is in a "RAM-Only" state before
	// we accept any customer workloads.
	if err := exec.Command("swapoff", "-a").Run(); err != nil {
		// If this fails, it usually means the daemon isn't running as root.
		return fmt.Errorf("system preflight: disable swap: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
	}

	// We double-check our work by parsing /proc/swaps to verify that no
	// active swap devices remain.
	if err := ensureSwapDisabled(); err != nil {
		return err
	}

	return nil
}

// ensureSwapDisabled performs a direct audit of the kernel's active swap list
// to verify that the host is operating in a RAM-only mode.
//
// This function parses the /proc/swaps virtual file, which is the kernel's
// definitive source of truth for all active swap partitions or files. We
// skip the header line (Filename Type Size...) and fail if any subsequent
// lines are found. This ensures that the "swapoff -a" command was successful
// and that no "zombie" swap devices remain active.
//
// By performing this audit at the kernel level, we ensure that the node
// remains in a high-performance, deterministic state before it begins
// accepting customer MicroVM workloads.
func ensureSwapDisabled() error {
	// We open /proc/swaps which is the kernel's virtual file for tracking
	// all currently active swap devices and files.
	file, err := os.Open(procSwapsPath)
	if err != nil {
		return fmt.Errorf("system preflight: open %s: %w", procSwapsPath, err)
	}
	defer file.Close()

	// We use a scanner to perform a line-by-line audit of the file.
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// We skip any empty lines to prevent false positives.
		if line == "" {
			continue
		}
		lineNumber++

		// The first line of /proc/swaps is always a header (Filename Type Size...).
		// We skip the header and only look for actual swap device entries
		// on subsequent lines.
		if lineNumber == 1 {
			continue
		}

		// If we find any line beyond the header, it means the kernel still has
		// an active swap device. This means our "swapoff -a" command failed
		// or was incomplete.
		return fmt.Errorf("system preflight: swap still enabled (detected active device in /proc/swaps)")
	}

	// Ensure we didn't encounter any I/O errors while reading the kernel file.
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("system preflight: scan %s: %w", procSwapsPath, err)
	}

	// If we only saw the header (or an empty file), the audit is successful.
	return nil
}

// disableKSM ensures that Kernel Samepage Merging (KSM) is disabled to provide
// absolute physical memory isolation between guest MicroVMs.
//
// KSM is a Linux kernel feature that allows the OS to "deduplicate" identical
// memory pages across different processes. While this increases RAM density,
// it is a security liability in a multi-tenant cloud environment because it
// enables side-channel "Memory Timing Attacks." By measuring how long it takes
// to write to a page, a malicious tenant can "guess" if that page is already
// being shared by another tenant's VM.
//
// SpaceScale rejects this density multiplier in favor of absolute physical
// integrity. We force KSM to 0 (disabled) to ensure that every 4KB page of
// customer RAM is unique and unshared on the physical silicon.
func disableKSM() error {
	// We first check if the kernel even supports KSM by probing for its
	// sysfs control path. If the path is missing, the kernel was likely
	// compiled without KSM support, which is an implicitly "Safe" state.
	_, err := os.Stat(ksmRunPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("system preflight: stat %s: %w", ksmRunPath, err)
	}

	// We explicitly write "0" to the KSM run switch. This tells the kernel
	// to immediately stop any background memory scanning or page merging.
	if err := writeSysfsValue(ksmRunPath, "0"); err != nil {
		return fmt.Errorf("system preflight: disable ksm: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
	}

	// We perform a "Truth Verification" by reading the value back from the
	// kernel. This ensures the write operation was successful and that the
	// node is now in a "Safe" memory-isolated state.
	value, err := readSysfsValue(ksmRunPath)
	if err != nil {
		return fmt.Errorf("system preflight: read ksm state: %w", err)
	}

	// If the value is anything other than "0", the node has failed its
	// security audit and cannot be trusted to host customer workloads.
	if value != "0" {
		return fmt.Errorf("system preflight: ksm still enabled with value %q (expected 0)", value)
	}

	return nil
}

// disableSMT ensures that Simultaneous Multithreading (SMT) or Hyperthreading
// is disabled at the kernel level to provide absolute physical execution
// isolation between guest MicroVMs.
//
// SMT is a security liability in multi-tenant environments because it allows
// two different virtual machines to share the same physical execution units
// (L1 cache, branch predictors, etc.) at the exact same nanosecond. This
// opens the door for side-channel attacks (Spectre, L1TF) where one tenant
// can "spy" on another tenant's compute state.
//
// SpaceScale operates on a "Physical Core Truth" model. We sacrifice the
// thread density of Hyperthreading to ensure that every customer workload
// has the exclusive, deterministic use of a dedicated physical silicon core.
func disableSMT() error {
	// We first read the current state of the kernel's SMT control knob.
	current, err := readSysfsValue(smtControlPath)
	if err != nil {
		// If the path is missing entirely, the kernel or hardware is either
		// extremely old or specialized, which is a pre-flight failure state.
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: %s not found (ensure kernel 4.19+ and SMT support)", smtControlPath)
		}
		return fmt.Errorf("system preflight: read smt state: %w", err)
	}

	// We check if the node is already in a secure state. "forceoff" means the
	// user disabled SMT in the BIOS, and "notsupported" means the CPU
	// doesn't have the feature at all—both are perfect for our security model.
	switch current {
	case "off", "forceoff", "notsupported":
		return nil
	}

	// If the node is currently in a vulnerable "on" state, we write "off" to
	// the magic sysfs control file. This instructs the kernel to instantly
	// "unplug" all logical sibling threads from the active CPU pool.
	if err := writeSysfsValue(smtControlPath, "off"); err != nil {
		return fmt.Errorf("system preflight: disable smt: %w (ensure daemon has root or CAP_SYS_ADMIN)", err)
	}

	// We perform a "Truth Verification" by reading the value back from the
	// hardware. This ensures the node has successfully transitioned to a
	// physically isolated execution state.
	current, err = readSysfsValue(smtControlPath)
	if err != nil {
		return fmt.Errorf("system preflight: read smt state: %w", err)
	}

	// If the value did not transition to a safe state, we fail the pre-flight
	// check to prevent this node from hosting sensitive workloads.
	switch current {
	case "off", "forceoff", "notsupported":
		return nil
	default:
		return fmt.Errorf("system preflight: smt still enabled with value %q (expected off/forceoff)", current)
	}
}

// EnsureFirecrackerJailerAccount makes sure the dedicated Firecracker jailer
// Linux account exists and returns its UID/GID.
//
// The account is intentionally tied to the group owner of /dev/kvm so jailed
// Firecracker processes can open KVM without running as root. If the account is
// missing, preflight creates it with a nologin shell and then looks it up again
// so launcher code can pass the numeric UID/GID into the jailer config.
func EnsureFirecrackerJailerAccount() (FirecrackerJailerIdentity, error) {
	// /dev/kvm's group is the host's source of truth for who may access hardware
	// virtualization. The jailer account must belong to this group.
	kvmGID, err := kvmDeviceGID()
	if err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	// Refuse root as the KVM group. Firecracker should drop into a dedicated
	// non-root identity, not continue as root after the jail is created.
	if kvmGID <= 0 {
		return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: %s group must be non-root", kvmDevicePath)
	}

	// Verify that the numeric KVM group actually resolves on this host before we
	// attach the jailer user to it.
	if _, err := user.LookupGroupId(strconv.Itoa(kvmGID)); err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: lookup group for %s gid %d: %w", kvmDevicePath, kvmGID, err)
	}

	// If the dedicated account is missing, create it. Any lookup error other than
	// "unknown user" is treated as a real host identity failure.
	if _, err := user.Lookup(FirecrackerJailerAccountName); err != nil {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return FirecrackerJailerIdentity{}, fmt.Errorf("system preflight: lookup firecracker jailer user: %w", err)
		}
		if err := createFirecrackerJailerUser(kvmGID); err != nil {
			return FirecrackerJailerIdentity{}, err
		}
	}

	// Return numeric IDs because the Firecracker SDK jailer config expects UID
	// and GID values, not Linux account names.
	return LookupFirecrackerJailerIdentity()
}

// LookupFirecrackerJailerIdentity resolves the dedicated Firecracker jailer
// Linux account into the numeric UID/GID that Firecracker's jailer needs.
//
// This function does not create the account. It only verifies the host already
// has a valid jailer user, that the user is non-root, and that its primary group
// matches the group owner of /dev/kvm so the jailed VMM can access KVM.
func LookupFirecrackerJailerIdentity() (FirecrackerJailerIdentity, error) {
	// /dev/kvm tells us which Linux group is allowed to use hardware
	// virtualization on this host.
	kvmGID, err := kvmDeviceGID()
	if err != nil {
		return FirecrackerJailerIdentity{}, err
	}

	// Look up the dedicated jailer account by name. This account is the identity
	// the Firecracker process drops into after the jailer has prepared the jail.
	u, err := user.Lookup(FirecrackerJailerAccountName)
	if err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("lookup firecracker jailer user %q: %w", FirecrackerJailerAccountName, err)
	}

	// The os/user package returns UID/GID as strings because system account
	// databases are text based. Convert them to ints for the Firecracker config.
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("parse firecracker jailer uid %q: %w", u.Uid, err)
	}

	// Parse the account's primary group ID. We expect this to be the /dev/kvm
	// group so the jailed VMM can open KVM without running as root.
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return FirecrackerJailerIdentity{}, fmt.Errorf("parse firecracker jailer gid %q: %w", u.Gid, err)
	}

	// UID 0 is root. A jailer identity must be non-root or the chroot boundary is
	// much less useful as a security boundary.
	if uid <= 0 {
		return FirecrackerJailerIdentity{}, fmt.Errorf("firecracker jailer user %q must be non-root", FirecrackerJailerAccountName)
	}

	// The jailer user must be in the same primary group as /dev/kvm. Otherwise
	// Firecracker may start as a restricted user but fail when it tries to create
	// the VM through KVM.
	if gid != kvmGID {
		return FirecrackerJailerIdentity{}, fmt.Errorf("firecracker jailer user %q gid %d does not match %s gid %d", FirecrackerJailerAccountName, gid, kvmDevicePath, kvmGID)
	}

	// Return the numeric identity because the Firecracker jailer config takes
	// UID/GID numbers, not Linux account names.
	return FirecrackerJailerIdentity{UID: uid, GID: gid}, nil
}

func createFirecrackerJailerUser(kvmGID int) error {
	shell := nologinShellPath

	if _, err := os.Stat(shell); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("system preflight: stat %s: %w", shell, err)
		}
		// Some minimal Linux images do not ship /usr/sbin/nologin. /bin/false is
		// a safe fallback for a service account because it exits immediately and
		// still prevents interactive shell login.
		shell = "/bin/false"
	}

	// Ask Linux to create a locked-down service account for Firecracker.
	// We use useradd instead of editing /etc/passwd and /etc/group ourselves
	// because useradd handles distro account rules, file locking, and shadow
	// database updates safely.
	args := []string{
		"--system",                    // Create a system/service account, not a normal human user.
		"--no-create-home",            // Do not create /home/spacescale-firecracker.
		"--gid", strconv.Itoa(kvmGID), // Make /dev/kvm's group the primary group.
		"--shell", shell, // Block interactive login with nologin or /bin/false.
		FirecrackerJailerAccountName, // The Linux username to create.
	}

	// Go can look up users through os/user, but the standard library does not
	// provide a native API for creating Linux users. Account creation is host
	// administration work, so shelling out to useradd is the safer boundary.
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
//
// Firecracker needs access to /dev/kvm to create hardware-accelerated
// microVMs. By reading the device file's group owner, scaled can create or
// validate a jailer account that belongs to the same group instead of running
// Firecracker as root.
func kvmDeviceGID() (int, error) {
	// Stat the actual KVM device file. The generic os.FileInfo tells us the path
	// exists, and its Sys field carries Linux-specific ownership metadata.
	info, err := os.Stat(kvmDevicePath)
	if err != nil {
		return 0, fmt.Errorf("system preflight: stat %s: %w", kvmDevicePath, err)
	}

	// On Linux, os.Stat stores the raw stat(2) result as *syscall.Stat_t. That is
	// where the numeric UID/GID live. If this cast fails, we cannot safely know
	// which group should own the Firecracker jailer account.
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("system preflight: stat %s: missing linux stat metadata", kvmDevicePath)
	}

	// stat.Gid is the group owner of /dev/kvm, usually the kvm group. The jailer
	// user must use this GID so jailed Firecracker can open /dev/kvm.
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
