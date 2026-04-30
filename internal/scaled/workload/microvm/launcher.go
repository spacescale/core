// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

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

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	scaledruntime "github.com/spacescale/core/internal/scaled/runtime"
	"github.com/spacescale/core/internal/scaled/system"
)

// scoutdKernelArgs is the fixed first-boot command line for the minimal scoutd
// guest. It keeps the Firecracker device model small, mounts the root disk, and
// gives scoutd enough bootstrap network metadata to finish initialization.
const scoutdKernelArgs = "console=ttyS0 reboot=k panic=-1 pci=off nomodules rw root=/dev/vda scoutd.ipv4=172.16.0.2/30 scoutd.gateway=172.16.0.1 scoutd.mmds=169.254.169.254"

// LaunchRequest is the local, transport-free scoutd boot request.
//
// Root disk size is intentionally absent. The scoutd rootfs is a
// platform-managed boot image copied as-is; later OCI workload scratch space or
// durable data should be modeled separately from this boot request.
type LaunchRequest struct {
	MicroVMID string
	VCPU      uint32
	RAMMB     uint64
}

// ActiveVM is the in-memory record for a VM launched by this scaled process.
type ActiveVM struct {
	MicroVMID string
	GuestCID  uint32
	Workspace Workspace

	machine     *firecracker.Machine
	listeners   *VSockListeners
	cancel      context.CancelFunc
	cleanupOnce sync.Once
}

// Launcher owns local Firecracker process lifecycle for this scaled process.
// Placement, duplicate launch prevention, and request validation are handled by
// the executor before it calls Launch.
type Launcher struct {
	logger       *slog.Logger
	runtimePaths scaledruntime.Paths
	jailerUID    int
	jailerGID    int
	cids         *cidAllocator

	mu     sync.Mutex
	active map[string]*ActiveVM
}

// NewLauncher creates the in-memory lifecycle owner for local Firecracker VMs.
// Startup preflight owns creating and resolving the fixed jailer account.
func NewLauncher(logger *slog.Logger, runtimePaths scaledruntime.Paths, jailerIdentity system.FirecrackerJailerIdentity) *Launcher {
	return &Launcher{
		logger:       logger.With("subsystem", "microvm"),
		runtimePaths: runtimePaths,
		jailerUID:    jailerIdentity.UID,
		jailerGID:    jailerIdentity.GID,
		cids:         newCIDAllocator(),
		active:       make(map[string]*ActiveVM),
	}
}

// Launch boots one jailed Firecracker VM and returns only after scoutd sends its
// hello frame over the control vsock channel.
//
// The input context bounds the boot attempt and hello wait. After hello is
// received, the VM gets its own background lifecycle context so the caller's
// request timeout does not kill an already accepted VM.
//
// Launch assumes the caller has already committed placement capacity and passed
// validated runtime paths and shape values.
func (l *Launcher) Launch(ctx context.Context, req LaunchRequest) (active *ActiveVM, err error) {
	vm, vmCtx := l.newActiveVM(req)
	defer func() {
		if err != nil && vm != nil {
			err = errors.Join(err, l.cleanupActive(vm, true))
		}
	}()

	if err := l.prepareVM(vm); err != nil {
		return nil, err
	}
	go l.drainScoutdLog(vmCtx, vm)

	jailerLog, err := openJailerLog(vm.Workspace.RootDir)
	if err != nil {
		return nil, err
	}
	defer jailerLog.Close()

	if err := l.startFirecracker(vmCtx, req, vm, jailerLog); err != nil {
		return nil, err
	}

	vmmExit := waitForMachine(vm.machine)
	if err := l.waitForHelloOrExit(ctx, vm, vmmExit); err != nil {
		return nil, err
	}

	l.activate(vm)

	go l.watchVM(vm, vmmExit)

	l.logger.Info("microvm booted scoutd",
		"microvm_id", req.MicroVMID,
		"guest_cid", vm.GuestCID,
		"workspace", vm.Workspace.RootDir,
	)

	return vm, nil
}

// Stop tears down a VM that was launched by this scaled process.
func (l *Launcher) Stop(_ context.Context, microvmID string) error {
	vm := l.removeActive(microvmID)
	if vm == nil {
		return nil
	}
	return l.cleanupActive(vm, true)
}

// newActiveVM creates the local lifecycle record before any host resources exist.
// The returned context belongs to the VM lifecycle; the caller's ctx only gates
// the boot attempt and scoutd hello wait.
func (l *Launcher) newActiveVM(req LaunchRequest) (*ActiveVM, context.Context) {
	workspace := newWorkspace(
		microVMStateDir,
		microVMJailerStateDir,
		req.MicroVMID,
		l.runtimePaths.FirecrackerPath,
	)

	vmCtx, cancel := context.WithCancel(context.Background())
	vm := &ActiveVM{
		MicroVMID: req.MicroVMID,
		Workspace: workspace,
		cancel:    cancel,
	}

	return vm, vmCtx
}

// prepareVM creates every host-side resource that must exist before Firecracker
// starts: workspace directories, rootfs copy, jailer ownership, CID, and vsock
// listeners.
func (l *Launcher) prepareVM(vm *ActiveVM) error {
	if err := vm.Workspace.Prepare(); err != nil {
		return fmt.Errorf("prepare workspace: %w", err)
	}

	if err := prepareRootFS(l.runtimePaths.RootFSPath, vm.Workspace.RootFSPath); err != nil {
		return err
	}

	// The copied rootfs is opened by Firecracker after the jailer drops privileges.
	// Give the fixed jailer account ownership while keeping the image private.
	if err := os.Chown(vm.Workspace.RootFSPath, l.jailerUID, l.jailerGID); err != nil {
		return fmt.Errorf("chown rootfs for jailer user: %w", err)
	}
	if err := os.Chmod(vm.Workspace.RootFSPath, 0o640); err != nil {
		return fmt.Errorf("chmod rootfs for jailer user: %w", err)
	}

	cid, err := l.cids.Acquire()
	if err != nil {
		return err
	}
	vm.GuestCID = cid

	listeners, err := openVSockListeners(vm.Workspace)
	if err != nil {
		return err
	}
	vm.listeners = listeners

	return nil
}

// startFirecracker builds the jailer-aware SDK config and starts the jailed VMM.
func (l *Launcher) startFirecracker(ctx context.Context, req LaunchRequest, vm *ActiveVM, jailerOutput io.Writer) error {
	fcCfg := l.buildFirecrackerConfig(req, vm.Workspace, vm.GuestCID, jailerOutput)
	machine, err := firecracker.NewMachine(ctx, fcCfg)
	if err != nil {
		return fmt.Errorf("create firecracker machine: %w", err)
	}
	vm.machine = machine

	if err := machine.Start(ctx); err != nil {
		return fmt.Errorf("start firecracker machine: %w", err)
	}
	return nil
}

// Firecracker config stays explicit here because this is the boundary where host
// paths become jail-visible paths for the SDK and jailer.
func (l *Launcher) buildFirecrackerConfig(req LaunchRequest, workspace Workspace, cid uint32, jailerOutput io.Writer) firecracker.Config {
	uid := l.jailerUID
	gid := l.jailerGID

	return firecracker.Config{
		SocketPath:      workspace.FirecrackerSocketPathInJail(),
		KernelImagePath: l.runtimePaths.KernelPath,
		KernelArgs:      scoutdKernelArgs,
		Drives:          firecracker.NewDrivesBuilder(workspace.RootFSPath).Build(),
		LogPath:         workspace.FirecrackerLogPathInJail(),
		LogLevel:        "Info",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(req.VCPU)),
			MemSizeMib: firecracker.Int64(int64(req.RAMMB)),
			Smt:        firecracker.Bool(false),
		},
		VsockDevices: []firecracker.VsockDevice{{
			ID:   "scoutd",
			Path: workspace.VSockPathInJail(),
			CID:  cid,
		}},
		JailerCfg: &firecracker.JailerConfig{
			UID:            &uid,
			GID:            &gid,
			ID:             req.MicroVMID,
			NumaNode:       firecracker.Int(0),
			ChrootBaseDir:  workspace.JailerBaseDir,
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(l.runtimePaths.KernelPath),
			ExecFile:       l.runtimePaths.FirecrackerPath,
			JailerBinary:   l.runtimePaths.JailerPath,
			Stdout:         jailerOutput,
			Stderr:         jailerOutput,
			CgroupVersion:  "2",
		},
	}
}

// openJailerLog captures jailer stdout/stderr during Firecracker startup.
func openJailerLog(rootDir string) (*os.File, error) {
	file, err := os.OpenFile(jailerLogHostPath(rootDir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open jailer log: %w", err)
	}
	return file, nil
}

// openScoutdLog captures the guest log stream after scoutd connects over vsock.
func openScoutdLog(rootDir string) (*os.File, error) {
	file, err := os.OpenFile(scoutdLogHostPath(rootDir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open scoutd log: %w", err)
	}
	return file, nil
}

func jailerLogHostPath(rootDir string) string {
	return filepath.Join(rootDir, "jailer.log")
}

func scoutdLogHostPath(rootDir string) string {
	return filepath.Join(rootDir, "scoutd.log")
}

func (l *Launcher) drainScoutdLog(ctx context.Context, vm *ActiveVM) {
	conn, err := vm.listeners.AcceptLog(ctx)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		l.logger.Warn("accept scoutd log channel",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
		return
	}
	defer conn.Close()

	logFile, err := openScoutdLog(vm.Workspace.RootDir)
	if err != nil {
		l.logger.Warn("open scoutd log",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
		return
	}
	defer logFile.Close()

	if _, err := io.Copy(logFile, conn); err != nil {
		select {
		case <-ctx.Done():
			return
		default:
			l.logger.Warn("drain scoutd log",
				"microvm_id", vm.MicroVMID,
				"error", err,
			)
		}
	}
}

// waitForMachine starts exactly one Firecracker wait goroutine. Launch and
// watchVM share the same result so we do not call Machine.Wait twice.
func waitForMachine(machine *firecracker.Machine) <-chan error {
	exited := make(chan error, 1)
	go func() {
		exited <- machine.Wait(context.Background())
	}()
	return exited
}

// waitForHelloOrExit accepts the launch only if scoutd sends hello before the
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
			err = fmt.Errorf("wait for scoutd hello: %w", err)
			l.logBootFailure(vm, "scoutd hello failed", err)
			return err
		}
		return nil
	case err := <-vmmExit:
		l.logBootFailure(vm, "microvm exited before scoutd hello", err)
		if err != nil {
			return fmt.Errorf("microvm exited before scoutd hello: %w", err)
		}
		return errors.New("microvm exited before scoutd hello")
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
		"scoutd_log", scoutdLogHostPath(vm.Workspace.RootDir),
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

	if err := l.cleanupActive(vm, false); err != nil {
		l.logger.Warn("cleanup exited microvm",
			"microvm_id", vm.MicroVMID,
			"error", err,
		)
	}
}

// cleanupActive unwinds all host-side resources owned by a VM. It is safe for
// launch failure, explicit Stop, and watchVM to converge here.
func (l *Launcher) cleanupActive(vm *ActiveVM, stopVMM bool) (err error) {
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
		err = errors.Join(err, vm.Workspace.Cleanup())
	})

	return err
}
