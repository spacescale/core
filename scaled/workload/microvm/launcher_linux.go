//go:build linux

package microvm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/spacescale/core/scaled/node"
)

// guestdKernelArgs is the fixed first-boot command line for the minimal guestd
// guest. It keeps the Firecracker device model small, leaves panic/error output
// on the serial console, mounts the root disk, and gives guestd enough bootstrap
// network metadata to finish initialization.
const guestdKernelArgs = "console=ttyS0 quiet loglevel=3 reboot=k panic=-1 pci=off acpi=off nomodule ro root=/dev/vda guestd.ipv4=172.16.0.2/30 guestd.gateway=172.16.0.1 guestd.mmds=169.254.169.254"

// LaunchRequest is the local, transport-free guestd boot request.
//
// Root disk size is intentionally absent. The guestd rootfs is a
// platform-managed boot image copied as-is; later OCI workload scratch space or
// durable data should be modeled separately from this boot request.
type LaunchRequest struct {
	MicroVMID string
	VCPU      uint32
	RAMMB     uint64

	ImageRef          string
	ImageDigest       string
	WorkloadImagePath string
	Command           []string
	WorkingDir        string
	User              string
	Env               map[string]string
	RuntimePort       uint32
}

// ActiveVM is the in-memory record for a VM launched by this scaled process.
type ActiveVM struct {
	MicroVMID string
	GuestCID  uint32
	Workspace Workspace
	Network   *Network

	machine     *firecracker.Machine
	listeners   *VSockListeners
	cancel      context.CancelFunc
	cleanupOnce sync.Once
}

// Launcher owns local Firecracker process lifecycle for this scaled process.
// Placement, duplicate launch prevention, and request validation are handled by
// the workload launch handler before it calls Launch.
type Launcher struct {
	logger       *slog.Logger
	runtimePaths node.RuntimePaths
	jailerUID    int
	jailerGID    int
	cids         *cidAllocator

	mu     sync.Mutex
	active map[string]*ActiveVM
}

// NewLauncher creates the in-memory lifecycle owner for local Firecracker VMs.
// Startup preflight owns creating and resolving the fixed jailer account, so
// runtimePaths come from the golden image and the jailer identity is already
// resolved when the launcher is built.
func NewLauncher(logger *slog.Logger, runtimePaths node.RuntimePaths, jailerIdentity node.FirecrackerJailerIdentity) *Launcher {
	return &Launcher{
		logger:       logger.With("component", "microvm"),
		runtimePaths: runtimePaths,
		jailerUID:    jailerIdentity.UID,
		jailerGID:    jailerIdentity.GID,
		cids:         newCIDAllocator(),
		active:       make(map[string]*ActiveVM),
	}
}

// Launch boots one jailed Firecracker VM and returns only after guestd sends its
// hello frame over the control vsock channel.
//
// The input context bounds the boot attempt and hello wait. After hello is
// received, the VM gets its own background lifecycle context so the caller's
// request timeout does not kill an already accepted VM.
//
// Launch assumes the caller has already committed placement capacity and passed
// validated runtime paths and shape values.
func (l *Launcher) Launch(ctx context.Context, req LaunchRequest) (active *ActiveVM, err error) {
	startedAt := time.Now()
	vm, vmCtx := l.newActiveVM(ctx, req)
	defer func() {
		if err != nil && vm != nil {
			err = errors.Join(err, l.cleanupActive(vm, true, false))
		}
	}()

	if err := l.prepareVM(vm); err != nil {
		return nil, err
	}
	go l.drainGuestdLog(vmCtx, vm)

	jailerLog, err := openJailerLog(vm.Workspace.RootDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = jailerLog.Close() }()

	if err := l.startFirecracker(vmCtx, req, vm, jailerLog); err != nil {
		return nil, err
	}

	// Assign a core scheduling cookie to the jailed Firecracker VMM process.
	// The kernel will only allow sibling SMT threads to execute tasks sharing
	// the same cookie simultaneously, preventing cross-tenant side channels.
	pid, err := vm.machine.PID()
	if err != nil {
		return nil, fmt.Errorf("get firecracker pid for core scheduling: %w", err)
	}
	if err := assignCoreSchedCookie(pid); err != nil {
		return nil, fmt.Errorf("assign core scheduling cookie: %w", err)
	}

	vmmExit := waitForMachine(vmCtx, vm.machine)
	if err := l.waitForHelloOrExit(ctx, vm, vmmExit); err != nil {
		return nil, err
	}

	l.activate(vm)

	go l.watchVM(vm, vmmExit)

	l.logger.Info("microvm booted guestd",
		"microvm_id", req.MicroVMID,
		"guest_cid", vm.GuestCID,
		"boot_ms", time.Since(startedAt).Milliseconds(),
		"workspace", vm.Workspace.RootDir,
		"tap", vm.Network.TapName,
		"guest_mac", vm.Network.GuestMAC,
		"mmds", vm.Network.MMDSIP.String(),
	)

	return vm, nil
}

// Stop tears down a VM that was launched by this scaled process.
func (l *Launcher) Stop(_ context.Context, microvmID string) error {
	vm := l.removeActive(microvmID)
	if vm == nil {
		return nil
	}
	return l.cleanupActive(vm, true, true)
}

// newActiveVM creates the local lifecycle record before any host resources exist.
// The returned context belongs to the VM lifecycle; the caller's ctx only gates
// the boot attempt and guestd hello wait.
func (l *Launcher) newActiveVM(ctx context.Context, req LaunchRequest) (*ActiveVM, context.Context) {
	workspace := newWorkspace(
		microVMStateDir,
		microVMJailerStateDir,
		req.MicroVMID,
		l.runtimePaths.FirecrackerPath,
	)

	vmCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	vm := &ActiveVM{
		MicroVMID: req.MicroVMID,
		Workspace: workspace,
		cancel:    cancel,
	}

	return vm, vmCtx
}

// prepareVM creates every host-side resource that must exist before Firecracker
// starts: workspace directories, CID, vsock listeners, and TAP networking.
func (l *Launcher) prepareVM(vm *ActiveVM) error {
	if err := vm.Workspace.Prepare(); err != nil {
		return fmt.Errorf("prepare workspace: %w", err)
	}

	cid, err := l.cids.Acquire()
	if err != nil {
		return err
	}
	vm.GuestCID = cid

	listeners, err := openVSockListeners(vm.Workspace, l.jailerUID, l.jailerGID)
	if err != nil {
		return err
	}
	vm.listeners = listeners

	network, err := prepareNetwork(vm.MicroVMID, l.jailerUID, l.jailerGID)
	if err != nil {
		return fmt.Errorf("prepare network: %w", err)
	}
	vm.Network = network

	return nil
}

// openJailerLog captures jailer stdout/stderr during Firecracker startup.
func openJailerLog(rootDir string) (*os.File, error) {
	file, err := os.OpenFile(jailerLogHostPath(rootDir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open jailer log: %w", err)
	}
	return file, nil
}

// openGuestdLog captures the guest log stream after guestd connects over vsock.
func openGuestdLog(rootDir string) (*os.File, error) {
	file, err := os.OpenFile(guestdLogHostPath(rootDir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open guestd log: %w", err)
	}
	return file, nil
}

func jailerLogHostPath(rootDir string) string {
	return filepath.Join(rootDir, "jailer.log")
}

func guestdLogHostPath(rootDir string) string {
	return filepath.Join(rootDir, "guestd.log")
}

func (l *Launcher) drainGuestdLog(ctx context.Context, vm *ActiveVM) {
	conn, err := vm.listeners.AcceptLog(ctx)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		l.logger.Warn("accept guestd log channel",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
		return
	}
	defer func() { _ = conn.Close() }()

	logFile, err := openGuestdLog(vm.Workspace.RootDir)
	if err != nil {
		l.logger.Warn("open guestd log",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
		return
	}
	defer func() { _ = logFile.Close() }()

	if _, err := io.Copy(logFile, conn); err != nil {
		select {
		case <-ctx.Done():
			return
		default:
			l.logger.Warn("drain guestd log",
				"microvm_id", vm.MicroVMID,
				"error", err,
			)
		}
	}
}

// waitForMachine starts exactly one Firecracker wait goroutine. Launch and
// watchVM share the same result so we do not call Machine.Wait twice.
func waitForMachine(ctx context.Context, machine *firecracker.Machine) <-chan error {
	exited := make(chan error, 1)
	go func(waitCtx context.Context) {
		exited <- machine.Wait(waitCtx)
	}(ctx)
	return exited
}

// waitForHelloOrExit accepts the launch only if guestd sends hello before the
// VMM exits. This avoids waiting for the full hello timeout after an early guest
// panic or clean shutdown.
func (l *Launcher) waitForHelloOrExit(ctx context.Context, vm *ActiveVM, vmmExit <-chan error) error {
	hello := make(chan error, 1)
	go func() {
		hello <- vm.listeners.WaitForHello(ctx)
	}()

	select {
	case err := <-hello:
		if err != nil {
			err = fmt.Errorf("wait for guestd hello: %w", err)
			l.logBootFailure(vm, "guestd hello failed", err)
			return err
		}
		return nil
	case err := <-vmmExit:
		l.logBootFailure(vm, "microvm exited before guestd hello", err)
		if err != nil {
			return fmt.Errorf("microvm exited before guestd hello: %w", err)
		}
		return errors.New("microvm exited before guestd hello")
	}
}

// logBootFailure records paths to diagnostic files without dumping noisy jailer,
// Firecracker, or guest output into scaled logs.
func (l *Launcher) logBootFailure(vm *ActiveVM, reason string, cause error) {
	args := []any{
		"microvm_id", vm.MicroVMID,
		"workspace", vm.Workspace.RootDir,
		"jailer_dir", vm.Workspace.JailerDir,
		"jailer_root", vm.Workspace.JailerRootDir,
		"jailer_log", jailerLogHostPath(vm.Workspace.RootDir),
		"firecracker_log", vm.Workspace.FirecrackerLogHostPath(),
		"guestd_log", guestdLogHostPath(vm.Workspace.RootDir),
	}
	if vm.Network != nil {
		args = append(args,
			"tap", vm.Network.TapName,
			"guest_mac", vm.Network.GuestMAC,
			"mmds", vm.Network.MMDSIP.String(),
		)
	}
	if cause != nil {
		args = append(args, "error", cause)
	}
	l.logger.Warn(reason, args...)
}

// activate records a booted VM for later Stop or Firecracker exit cleanup.
func (l *Launcher) activate(vm *ActiveVM) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.active[vm.MicroVMID] = vm
}

// removeActive transfers active VM ownership to Stop.
func (l *Launcher) removeActive(microvmID string) *ActiveVM {
	l.mu.Lock()
	defer l.mu.Unlock()

	vm := l.active[microvmID]
	delete(l.active, microvmID)
	return vm
}

// removeActiveIfSame lets watchVM unregister the VM only if Stop has not already
// removed it.
func (l *Launcher) removeActiveIfSame(vm *ActiveVM) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active[vm.MicroVMID] == vm {
		delete(l.active, vm.MicroVMID)
	}
}

// watchVM handles Firecracker exiting after a successful hello.
func (l *Launcher) watchVM(vm *ActiveVM, vmmExit <-chan error) {
	err := <-vmmExit
	if err != nil {
		l.logger.Warn("firecracker exited",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
	}

	l.removeActiveIfSame(vm)

	if err := l.cleanupActive(vm, false, true); err != nil {
		l.logger.Warn("cleanup exited microvm",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
	}
}

// cleanupActive unwinds host-side resources owned by a VM. Failed launches keep
// their workspace and jailer tree so the logged diagnostic paths remain useful.
func (l *Launcher) cleanupActive(vm *ActiveVM, stopVMM bool, removeWorkspace bool) (err error) {
	vm.cleanupOnce.Do(func() {
		if stopVMM && vm.machine != nil {
			err = errors.Join(err, vm.machine.StopVMM())
		}
		if vm.cancel != nil {
			vm.cancel()
		}
		if vm.listeners != nil {
			err = errors.Join(err, vm.listeners.Close())
		}
		if vm.GuestCID != 0 {
			l.cids.Release(vm.GuestCID)
		}

		if vm.Network != nil {
			err = errors.Join(err, vm.Network.Cleanup())
		}

		if removeWorkspace {
			err = errors.Join(err, vm.Workspace.Cleanup())
		}
	})

	return err
}

// assignCoreSchedCookie creates a new core scheduling cookie and assigns it to
// the given PID's thread group. The kernel will only allow sibling SMT threads
// to execute tasks that share the same cookie simultaneously.
func assignCoreSchedCookie(pid int) error {
	_, _, errno := syscall.RawSyscall6(
		syscall.SYS_PRCTL,
		node.PRSchedCore,
		node.PRSchedCoreCreate,
		uintptr(pid),
		node.PidTypeTGID,
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("prctl PR_SCHED_CORE create for pid %d: %w", pid, errno)
	}
	return nil
}
