//go:build linux

// launcher_linux_test covers the concrete launcher helpers that are worth unit
// testing without introducing fake-oriented interfaces into the Firecracker
// boot path.
//
// Functions tested here:
//   - newActiveVM
//   - activate
//   - removeActive
//   - removeActiveIfSame
//   - Stop for unknown microVM IDs
//   - jailerLogHostPath
//   - guestdLogHostPath
//
// Functions intentionally skipped here:
//   - Launch
//   - prepareVM
//   - startFirecracker
//   - cleanupActive error injection paths
//
// Reason: those paths are tightly coupled to real Firecracker processes,
// privileged Linux resources, netlink, Unix vsock listeners, and jailer
// behavior. Forcing unit-test seams there would distort the most delicate
// package. They are better covered by concrete helper tests here and by future
// Linux integration tests if needed.
package microvm

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spacescale/core/scaled/node"
	"github.com/stretchr/testify/require"
)

// newTestLauncher is a tiny test fixture for the concrete launcher helper tests
// below. It only wires the minimum state those tests need: a Firecracker binary
// name for workspace path construction, a CID allocator, and the active VM map.
func newTestLauncher() *Launcher {
	return &Launcher{
		runtimePaths: node.RuntimePaths{
			FirecrackerPath: "/usr/bin/firecracker",
		},
		cids:    newCIDAllocator(),
		subnets: newSubnetAllocator(),
		active:  make(map[string]*ActiveVM),
	}
}

func TestLauncherNewActiveVMCreatesScopedContext(t *testing.T) {
	launcher := newTestLauncher()
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	req := LaunchRequest{
		MicroVMID: "vm-123",
		VCPU:      2,
		RAMMB:     512,
	}

	vm, vmCtx := launcher.newActiveVM(parentCtx, req)

	require.Equal(t, req.MicroVMID, vm.MicroVMID)
	require.Equal(t, filepath.Join(microVMStateDir, req.MicroVMID), vm.Workspace.RootDir)
	require.Equal(t, filepath.Join(microVMJailerStateDir, "firecracker", req.MicroVMID), vm.Workspace.JailerDir)
	require.NotNil(t, vm.cancel)

	cancelParent()

	select {
	case <-vmCtx.Done():
		t.Fatal("vm context should outlive the parent boot context")
	default:
	}

	vm.cancel()

	select {
	case <-vmCtx.Done():
	default:
		t.Fatal("vm cancel should stop the vm lifecycle context")
	}
}

func TestLauncherActivateAndRemoveActive(t *testing.T) {
	launcher := newTestLauncher()
	vm := &ActiveVM{MicroVMID: "vm-123"}

	launcher.activate(vm)
	require.Same(t, vm, launcher.active[vm.MicroVMID])

	removed := launcher.removeActive(vm.MicroVMID)
	require.Same(t, vm, removed)
	require.Empty(t, launcher.active)
}

func TestLauncherRemoveActiveIfSameRemovesMatchingVMOnly(t *testing.T) {
	launcher := newTestLauncher()
	original := &ActiveVM{MicroVMID: "vm-123"}
	other := &ActiveVM{MicroVMID: "vm-123"}

	launcher.activate(original)
	launcher.removeActiveIfSame(other)
	require.Same(t, original, launcher.active[original.MicroVMID])

	launcher.removeActiveIfSame(original)
	require.Empty(t, launcher.active)
}

func TestLauncherStopReturnsNilForUnknownVM(t *testing.T) {
	launcher := newTestLauncher()

	err := launcher.Stop(context.Background(), "missing-vm")
	require.NoError(t, err)
}

func TestLauncherLogPaths(t *testing.T) {
	rootDir := "/var/lib/spacescale/microvms/vm-123"

	require.Equal(t, filepath.Join(rootDir, "jailer.log"), jailerLogHostPath(rootDir))
	require.Equal(t, filepath.Join(rootDir, "guestd.log"), guestdLogHostPath(rootDir))
}
