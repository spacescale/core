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

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	scaledruntime "github.com/spacescale/core/internal/scaled/runtime"
)

// scoutdKernelArgs is the fixed first-boot command line for the minimal scoutd
// guest. It keeps the Firecracker device model small, mounts the root disk, and
// gives scoutd enough bootstrap network metadata to finish initialization.
const scoutdKernelArgs = "console=ttyS0 reboot=k panic=-1 pci=off nomodules rw root=/dev/vda scoutd.ipv4=172.16.0.2/30 scoutd.gateway=172.16.0.1 scoutd.mmds=169.254.169.254"

var (
	errInvalidLauncherConfig = errors.New("invalid microvm launcher config")
	errInvalidLaunchRequest  = errors.New("invalid microvm launch request")
	errMicroVMAlreadyActive  = errors.New("microvm already active")
)

// LauncherConfig is the host-local configuration needed to start jailed
// Firecracker processes.
//
// JailerUID and JailerGID must identify a dedicated non-root host account for
// Firecracker. The jailer starts with enough privilege to build the jail, then
// drops the VMM process to this uid/gid.
type LauncherConfig struct {
	RuntimePaths scaledruntime.Paths
	JailerUID    int
	JailerGID    int
}

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
	logger *slog.Logger
	cfg    LauncherConfig
	cids   *cidAllocator

	mu        sync.Mutex
	active    map[string]*ActiveVM
	launching map[string]struct{}
}

// NewLauncher validates static host configuration and creates the in-memory
// lifecycle owner for local Firecracker VMs.
func NewLauncher(logger *slog.Logger, cfg LauncherConfig) (*Launcher, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.JailerUID <= 0 {
		return nil, fmt.Errorf("%w: dedicated non-root jailer uid is required", errInvalidLauncherConfig)
	}
	if cfg.JailerGID <= 0 {
		return nil, fmt.Errorf("%w: dedicated non-root jailer gid is required", errInvalidLauncherConfig)
	}
	if cfg.RuntimePaths.FirecrackerPath == "" {
		return nil, fmt.Errorf("%w: firecracker path is required", errInvalidLauncherConfig)
	}
	if cfg.RuntimePaths.JailerPath == "" {
		return nil, fmt.Errorf("%w: jailer path is required", errInvalidLauncherConfig)
	}
	if cfg.RuntimePaths.KernelPath == "" {
		return nil, fmt.Errorf("%w: kernel path is required", errInvalidLauncherConfig)
	}
	if cfg.RuntimePaths.RootFSPath == "" {
		return nil, fmt.Errorf("%w: rootfs path is required", errInvalidLauncherConfig)
	}

	return &Launcher{
		logger:    logger.With("subsystem", "microvm"),
		cfg:       cfg,
		cids:      newCIDAllocator(),
		active:    make(map[string]*ActiveVM),
		launching: make(map[string]struct{}),
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
	if err := validateRuntimeAssets(l.cfg.RuntimePaths); err != nil {
		return nil, err
	}
	if err := l.reserveLaunch(req.MicroVMID); err != nil {
		return nil, err
	}

	// reserveLaunch blocks duplicate launches while this goroutine is still
	// preparing files and waiting for scoutd. Once activate succeeds, ownership
	// moves from the launching map to the active map.
	launchCommitted := false
	defer func() {
		if !launchCommitted {
			l.releaseLaunch(req.MicroVMID)
		}
	}()

	// vm becomes non-nil as soon as cleanup has something useful to own. The named
	// return err lets this defer append cleanup failures to the original launch
	// error instead of hiding either one.
	var vm *ActiveVM
	defer func() {
		if err != nil && vm != nil {
			err = errors.Join(err, l.cleanupActive(vm, true))
		}
	}()

	workspace, err := newWorkspace(
		microVMStateDir,
		microVMJailerStateDir,
		req.MicroVMID,
		l.cfg.RuntimePaths.FirecrackerPath,
	)
	if err != nil {
		return nil, err
	}

	// Use a VM lifecycle context that is independent of the request context.
	// Before hello, any launch error triggers cleanup and cancels this context.
	// After hello, Stop or process exit owns teardown.
	vmCtx, cancel := context.WithCancel(context.Background())
	vm = &ActiveVM{
		MicroVMID: req.MicroVMID,
		Workspace: workspace,
		cancel:    cancel,
	}

	if err := workspace.Prepare(); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}

	if err := prepareRootFS(l.cfg.RuntimePaths.RootFSPath, workspace.RootFSPath); err != nil {
		return nil, err
	}

	// The SDK links the copied rootfs into the jail. Because Firecracker drops to
	// the dedicated uid/gid, that account must be able to open the writable drive.
	if err := os.Chown(workspace.RootFSPath, l.cfg.JailerUID, l.cfg.JailerGID); err != nil {
		return nil, fmt.Errorf("chown rootfs for jailer user: %w", err)
	}
	if err := os.Chmod(workspace.RootFSPath, 0o640); err != nil {
		return nil, fmt.Errorf("chmod rootfs for jailer user: %w", err)
	}

	cid, err := l.cids.Acquire()
	if err != nil {
		return nil, err
	}
	vm.GuestCID = cid

	// scoutd connects very early during guest boot, so the host-side Unix sockets
	// must exist before Firecracker starts the VM.
	listeners, err := openVSockListeners(workspace)
	if err != nil {
		return nil, err
	}
	vm.listeners = listeners

	jailerLog, err := openJailerLog(workspace.RootDir)
	if err != nil {
		return nil, err
	}
	defer jailerLog.Close()

	fcCfg := l.buildFirecrackerConfig(req, workspace, cid, jailerLog)
	machine, err := firecracker.NewMachine(vmCtx, fcCfg)
	if err != nil {
		return nil, fmt.Errorf("create firecracker machine: %w", err)
	}
	vm.machine = machine

	if err := machine.Start(vmCtx); err != nil {
		return nil, fmt.Errorf("start firecracker machine: %w", err)
	}

	if err := listeners.WaitForHello(ctx); err != nil {
		return nil, fmt.Errorf("wait for scoutd hello: %w", err)
	}

	l.activate(vm)
	launchCommitted = true

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
	uid := l.cfg.JailerUID
	gid := l.cfg.JailerGID

	return firecracker.Config{
		SocketPath:      workspace.FirecrackerSocketPathInJail(),
		KernelImagePath: l.cfg.RuntimePaths.KernelPath,
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
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(l.cfg.RuntimePaths.KernelPath),
			ExecFile:       l.cfg.RuntimePaths.FirecrackerPath,
			JailerBinary:   l.cfg.RuntimePaths.JailerPath,
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
