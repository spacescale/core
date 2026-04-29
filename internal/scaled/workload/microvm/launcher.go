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

var (
	errInvalidLauncherConfig = errors.New("invalid microvm launcher config")
	errInvalidLaunchRequest  = errors.New("invalid microvm launch request")
	errMicroVMAlreadyActive  = errors.New("microvm already active")
	lookupJailerIdentity     = system.LookupFirecrackerJailerIdentity
)

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
type Launcher struct {
	logger       *slog.Logger
	runtimePaths scaledruntime.Paths
	jailerUID    int
	jailerGID    int
	cids         *cidAllocator

	mu        sync.Mutex
	active    map[string]*ActiveVM
	launching map[string]struct{}
}

// NewLauncher validates host-local runtime asset paths, resolves the dedicated
// Firecracker jailer identity, and creates the in-memory lifecycle owner for
// local Firecracker VMs.
func NewLauncher(logger *slog.Logger, runtimePaths scaledruntime.Paths) (*Launcher, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if runtimePaths.FirecrackerPath == "" {
		return nil, fmt.Errorf("%w: firecracker path is required", errInvalidLauncherConfig)
	}
	if runtimePaths.JailerPath == "" {
		return nil, fmt.Errorf("%w: jailer path is required", errInvalidLauncherConfig)
	}
	if runtimePaths.KernelPath == "" {
		return nil, fmt.Errorf("%w: kernel path is required", errInvalidLauncherConfig)
	}
	if runtimePaths.RootFSPath == "" {
		return nil, fmt.Errorf("%w: rootfs path is required", errInvalidLauncherConfig)
	}
	jailerIdentity, err := lookupJailerIdentity()
	if err != nil {
		return nil, fmt.Errorf("%w: resolve firecracker jailer identity: %w", errInvalidLauncherConfig, err)
	}
	if jailerIdentity.UID <= 0 {
		return nil, fmt.Errorf("%w: firecracker jailer uid must be non-root", errInvalidLauncherConfig)
	}
	if jailerIdentity.GID <= 0 {
		return nil, fmt.Errorf("%w: firecracker jailer gid must be non-root", errInvalidLauncherConfig)
	}

	return &Launcher{
		logger:       logger.With("subsystem", "microvm"),
		runtimePaths: runtimePaths,
		jailerUID:    jailerIdentity.UID,
		jailerGID:    jailerIdentity.GID,
		cids:         newCIDAllocator(),
		active:       make(map[string]*ActiveVM),
		launching:    make(map[string]struct{}),
	}, nil
}

func (r LaunchRequest) validate() error {
	if r.MicroVMID == "" {
		return fmt.Errorf("%w: microvm id is required", errInvalidLaunchRequest)
	}
	if r.VCPU == 0 {
		return fmt.Errorf("%w: vcpu must be greater than zero", errInvalidLaunchRequest)
	}
	if r.RAMMB == 0 {
		return fmt.Errorf("%w: ram mb must be greater than zero", errInvalidLaunchRequest)
	}
	return nil
}

// Launch boots one jailed Firecracker VM and returns only after scoutd sends its
// hello frame over the control vsock channel.
//
// The input context bounds the boot attempt and hello wait. After hello is
// received, the VM gets its own background lifecycle context so the caller's
// request timeout does not kill an already accepted VM.
func (l *Launcher) Launch(ctx context.Context, req LaunchRequest) (active *ActiveVM, err error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if err := validateRuntimeAssets(l.runtimePaths); err != nil {
		return nil, err
	}
	if err := l.reserveLaunch(req.MicroVMID); err != nil {
		return nil, err
	}

	// reserveLaunch marks this microVM ID as in-flight so another goroutine cannot
	// launch the same VM while this one is preparing files and waiting for scoutd.
	// If launch fails, the reservation is released. If launch succeeds, activate
	// moves the VM from launching to active.
	launchCommitted := false
	defer func() {
		if !launchCommitted {
			l.releaseLaunch(req.MicroVMID)
		}
	}()

	// After vm is assigned, later launch steps may create resources that need
	// teardown: workspace files, vsock listeners, a CID, or a Firecracker process.
	// If Launch returns an error, clean up that partial VM. Launch uses a named
	// return err so cleanup failures can be joined with the original launch error
	// instead of replacing it.
	var vm *ActiveVM
	defer func() {
		if err != nil && vm != nil {
			// Preserve the original launch failure while reporting any cleanup failure too.
			err = errors.Join(err, l.cleanupActive(vm, true))
		}
	}()

	workspace, err := newWorkspace(
		microVMStateDir,
		microVMJailerStateDir,
		req.MicroVMID,
		l.runtimePaths.FirecrackerPath,
	)
	if err != nil {
		return nil, err
	}

	// The request ctx only bounds boot and the scoutd hello wait. Firecracker gets
	// its own lifecycle ctx so a successfully accepted VM is not killed when the
	// request ctx is canceled after Launch returns. If launch fails before hello,
	// cleanup cancels this ctx; after hello, Stop or watchVM owns teardown.
	vmCtx, cancel := context.WithCancel(context.Background())
	vm = &ActiveVM{
		MicroVMID: req.MicroVMID,
		Workspace: workspace,
		cancel:    cancel,
	}

	if err := workspace.Prepare(); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}

	if err := prepareRootFS(l.runtimePaths.RootFSPath, workspace.RootFSPath); err != nil {
		return nil, err
	}

	// The copied rootfs is opened by the Firecracker process after the jailer drops
	// privileges to the dedicated uid/gid. Give that account ownership and writable
	// access while keeping the disk image private from other host users.
	if err := os.Chown(workspace.RootFSPath, l.jailerUID, l.jailerGID); err != nil {
		return nil, fmt.Errorf("chown rootfs for jailer user: %w", err)
	}
	if err := os.Chmod(workspace.RootFSPath, 0o640); err != nil {
		return nil, fmt.Errorf("chmod rootfs for jailer user: %w", err)
	}

	// Allocate the guest vsock CID before building the Firecracker config. CID 2 is
	// the host; every active guest needs its own CID so scoutd traffic is routed to
	// the correct VM.
	cid, err := l.cids.Acquire()
	if err != nil {
		return nil, err
	}
	vm.GuestCID = cid

	// Firecracker exposes guest-initiated vsock connections as host Unix sockets
	// named from this workspace path. scoutd connects very early during guest boot,
	// so the host listeners must exist before machine.Start.
	listeners, err := openVSockListeners(workspace)
	if err != nil {
		return nil, err
	}
	vm.listeners = listeners

	// Keep jailer stdout/stderr in the VM workspace. The file stays open while the
	// SDK starts the jailer so startup failures leave useful host-side logs behind.
	jailerLog, err := openJailerLog(workspace.RootDir)
	if err != nil {
		return nil, err
	}
	defer jailerLog.Close()

	// Build the SDK config after all referenced host resources exist. The config
	// intentionally mixes host paths for files the SDK links into the jail and
	// jail-visible paths for files Firecracker opens after chroot.
	fcCfg := l.buildFirecrackerConfig(req, workspace, cid, jailerLog)

	// NewMachine wires SDK handlers and jailer metadata. It does not start the
	// jailer or Firecracker process yet.
	machine, err := firecracker.NewMachine(vmCtx, fcCfg)
	if err != nil {
		return nil, fmt.Errorf("create firecracker machine: %w", err)
	}
	vm.machine = machine

	// Start launches the jailer, builds the chroot, drops to the dedicated uid/gid,
	// and execs Firecracker. Any failure after this point must stop a partial VMM.
	if err := machine.Start(vmCtx); err != nil {
		return nil, fmt.Errorf("start firecracker machine: %w", err)
	}

	// Accept the launch only after guest userspace proves it booted by sending the
	// scoutd hello frame on the control vsock socket. The request ctx controls how
	// long we wait for that proof.
	if err := listeners.WaitForHello(ctx); err != nil {
		return nil, fmt.Errorf("wait for scoutd hello: %w", err)
	}

	// Hello succeeded: move the VM from launching to active ownership before
	// returning it to the caller.
	l.activate(vm)
	launchCommitted = true

	// From here the VM lifecycle is asynchronous. Stop handles explicit teardown;
	// watchVM handles Firecracker exiting on its own.
	go l.watchVM(vm)

	l.logger.Info("microvm booted scoutd",
		"microvm_id", req.MicroVMID,
		"guest_cid", cid,
		"workspace", workspace.RootDir,
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

// buildFirecrackerConfig assembles the SDK config. Host paths are used for
// files the SDK/jailer must link into the jail; jail-visible paths are used for
// paths Firecracker opens after chroot.
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

// validateRuntimeAssets performs a last local sanity check before launch. The
// startup resolver should already have reconciled these files, but launcher keeps
// this boundary defensive because bad paths here would fail deep inside the SDK.
func validateRuntimeAssets(paths scaledruntime.Paths) error {
	checks := []struct {
		name       string
		path       string
		executable bool
	}{
		{name: "firecracker", path: paths.FirecrackerPath, executable: true},
		{name: "jailer", path: paths.JailerPath, executable: true},
		{name: "kernel", path: paths.KernelPath},
		{name: "rootfs", path: paths.RootFSPath},
	}

	for _, check := range checks {
		if check.path == "" {
			return fmt.Errorf("runtime asset %s path is required", check.name)
		}
		info, err := os.Stat(check.path)
		if err != nil {
			return fmt.Errorf("stat runtime asset %s: %w", check.name, err)
		}
		if info.IsDir() {
			return fmt.Errorf("runtime asset %s is a directory: %s", check.name, check.path)
		}
		if info.Size() == 0 {
			return fmt.Errorf("runtime asset %s is empty: %s", check.name, check.path)
		}
		if check.executable && info.Mode()&0o111 == 0 {
			return fmt.Errorf("runtime asset %s is not executable: %s", check.name, check.path)
		}
	}

	return nil
}

func openJailerLog(rootDir string) (*os.File, error) {
	path := filepath.Join(rootDir, "jailer.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open jailer log: %w", err)
	}
	return file, nil
}

// reserveLaunch prevents concurrent duplicate launches for the same microVM ID.
// The reservation is local to this scaled process and is released when launch
// fails or promoted to active after scoutd hello succeeds.
func (l *Launcher) reserveLaunch(microvmID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.active[microvmID]; ok {
		return errMicroVMAlreadyActive
	}

	if _, ok := l.launching[microvmID]; ok {
		return errMicroVMAlreadyActive
	}

	l.launching[microvmID] = struct{}{}
	return nil
}

func (l *Launcher) releaseLaunch(microvmID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.launching, microvmID)
}

func (l *Launcher) activate(vm *ActiveVM) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.launching, vm.MicroVMID)
	l.active[vm.MicroVMID] = vm
}

func (l *Launcher) removeActive(microvmID string) *ActiveVM {
	l.mu.Lock()
	defer l.mu.Unlock()

	vm := l.active[microvmID]
	delete(l.active, microvmID)
	return vm
}

func (l *Launcher) removeActiveIfSame(vm *ActiveVM) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active[vm.MicroVMID] == vm {
		delete(l.active, vm.MicroVMID)
	}
}

// watchVM waits for Firecracker process exit after a successful launch and then
// performs local cleanup. It is separate from Launch so Launch can return once
// scoutd hello proves the VM booted.
func (l *Launcher) watchVM(vm *ActiveVM) {
	err := vm.machine.Wait(context.Background())
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

// cleanupActive unwinds every host-side resource owned by an ActiveVM.
//
// It is safe to call from multiple paths: launch failure, explicit Stop, and the
// process watcher. cleanupOnce makes those paths converge on one teardown.
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
