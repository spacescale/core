//go:build linux

package microvm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/spacescale/core/internal/scaled/runtime"
	"github.com/spacescale/core/internal/scaled/system"
	"github.com/stretchr/testify/require"
)

func stubJailerIdentity(t *testing.T, uid, gid int) {
	t.Helper()

	original := lookupJailerIdentity
	lookupJailerIdentity = func() (system.FirecrackerJailerIdentity, error) {
		return system.FirecrackerJailerIdentity{UID: uid, GID: gid}, nil
	}
	t.Cleanup(func() {
		lookupJailerIdentity = original
	})
}

func TestNewLauncherRequiresDedicatedNonRootJailerUser(t *testing.T) {
	paths := runtime.Paths{
		FirecrackerPath: "/runtime/firecracker",
		JailerPath:      "/runtime/jailer",
		KernelPath:      "/runtime/vmlinux",
		RootFSPath:      "/runtime/rootfs.ext4",
	}

	original := lookupJailerIdentity
	t.Cleanup(func() {
		lookupJailerIdentity = original
	})

	lookupJailerIdentity = func() (system.FirecrackerJailerIdentity, error) {
		return system.FirecrackerJailerIdentity{}, errors.New("missing jailer user")
	}
	_, err := NewLauncher(nil, paths)
	require.Error(t, err)

	lookupJailerIdentity = func() (system.FirecrackerJailerIdentity, error) {
		return system.FirecrackerJailerIdentity{UID: 0, GID: 100}, nil
	}
	_, err = NewLauncher(nil, paths)
	require.Error(t, err)

	lookupJailerIdentity = func() (system.FirecrackerJailerIdentity, error) {
		return system.FirecrackerJailerIdentity{UID: 100, GID: 0}, nil
	}
	_, err = NewLauncher(nil, paths)
	require.Error(t, err)
}

func TestLaunchRequestValidation(t *testing.T) {
	tests := []struct {
		name string
		req  LaunchRequest
	}{
		{
			name: "missing id",
			req: LaunchRequest{
				VCPU:  1,
				RAMMB: 128,
			},
		},
		{
			name: "missing vcpu",
			req: LaunchRequest{
				MicroVMID: "vm-123",
				RAMMB:     128,
			},
		},
		{
			name: "missing ram",
			req: LaunchRequest{
				MicroVMID: "vm-123",
				VCPU:      1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.req.validate())
		})
	}

	require.NoError(t, LaunchRequest{
		MicroVMID: "vm-123",
		VCPU:      1,
		RAMMB:     128,
	}.validate())
}

func TestBuildFirecrackerConfigUsesJailVisiblePaths(t *testing.T) {
	paths := runtime.Paths{
		FirecrackerPath: "/var/lib/spacescale/runtime/host/firecracker-v1.15.1-x86_64",
		JailerPath:      "/var/lib/spacescale/runtime/host/jailer-v1.15.1-x86_64",
		KernelPath:      "/var/lib/spacescale/runtime/guest/vmlinux-v6.1.80-x86_64",
		RootFSPath:      "/var/lib/spacescale/runtime/guest/scoutd-rootfs-v0.1.3-x86_64-ext4",
	}

	stubJailerIdentity(t, 123, 456)

	launcher, err := NewLauncher(nil, paths)
	require.NoError(t, err)

	workspace, err := newWorkspace(
		"/var/lib/spacescale/microvms",
		"/var/lib/spacescale/j",
		"vm-123",
		paths.FirecrackerPath,
	)
	require.NoError(t, err)

	req := LaunchRequest{
		MicroVMID: "vm-123",
		VCPU:      2,
		RAMMB:     512,
	}

	cfg := launcher.buildFirecrackerConfig(req, workspace, 3, nil)

	require.Equal(t, workspace.FirecrackerSocketPathInJail(), cfg.SocketPath)
	require.Equal(t, workspace.FirecrackerLogPathInJail(), cfg.LogPath)
	require.Equal(t, scoutdKernelArgs, cfg.KernelArgs)
	require.Equal(t, paths.KernelPath, cfg.KernelImagePath)

	require.Len(t, cfg.VsockDevices, 1)
	require.Equal(t, "scoutd", cfg.VsockDevices[0].ID)
	require.Equal(t, workspace.VSockPathInJail(), cfg.VsockDevices[0].Path)
	require.Equal(t, uint32(3), cfg.VsockDevices[0].CID)

	require.Len(t, cfg.Drives, 1)
	require.Equal(t, workspace.RootFSPath, firecracker.StringValue(cfg.Drives[0].PathOnHost))

	require.Equal(t, int64(2), firecracker.Int64Value(cfg.MachineCfg.VcpuCount))
	require.Equal(t, int64(512), firecracker.Int64Value(cfg.MachineCfg.MemSizeMib))
	require.False(t, firecracker.BoolValue(cfg.MachineCfg.Smt))

	require.NotNil(t, cfg.JailerCfg)
	require.Equal(t, 123, *cfg.JailerCfg.UID)
	require.Equal(t, 456, *cfg.JailerCfg.GID)
	require.Equal(t, "vm-123", cfg.JailerCfg.ID)
	require.Equal(t, workspace.JailerBaseDir, cfg.JailerCfg.ChrootBaseDir)
	require.Equal(t, paths.FirecrackerPath, cfg.JailerCfg.ExecFile)
	require.Equal(t, paths.JailerPath, cfg.JailerCfg.JailerBinary)
	require.Equal(t, "2", cfg.JailerCfg.CgroupVersion)
}

func TestValidateRuntimeAssets(t *testing.T) {
	root := t.TempDir()

	firecrackerPath := filepath.Join(root, "firecracker")
	jailerPath := filepath.Join(root, "jailer")
	kernelPath := filepath.Join(root, "vmlinux")
	rootfsPath := filepath.Join(root, "rootfs.ext4")

	require.NoError(t, os.WriteFile(firecrackerPath, []byte("firecracker"), 0o755))
	require.NoError(t, os.WriteFile(jailerPath, []byte("jailer"), 0o755))
	require.NoError(t, os.WriteFile(kernelPath, []byte("kernel"), 0o644))
	require.NoError(t, os.WriteFile(rootfsPath, []byte("rootfs"), 0o644))

	require.NoError(t, validateRuntimeAssets(runtime.Paths{
		FirecrackerPath: firecrackerPath,
		JailerPath:      jailerPath,
		KernelPath:      kernelPath,
		RootFSPath:      rootfsPath,
	}))
}

func TestValidateRuntimeAssetsRejectsNonExecutableHostBinary(t *testing.T) {
	root := t.TempDir()

	firecrackerPath := filepath.Join(root, "firecracker")
	jailerPath := filepath.Join(root, "jailer")
	kernelPath := filepath.Join(root, "vmlinux")
	rootfsPath := filepath.Join(root, "rootfs.ext4")

	require.NoError(t, os.WriteFile(firecrackerPath, []byte("firecracker"), 0o644))
	require.NoError(t, os.WriteFile(jailerPath, []byte("jailer"), 0o755))
	require.NoError(t, os.WriteFile(kernelPath, []byte("kernel"), 0o644))
	require.NoError(t, os.WriteFile(rootfsPath, []byte("rootfs"), 0o644))

	err := validateRuntimeAssets(runtime.Paths{
		FirecrackerPath: firecrackerPath,
		JailerPath:      jailerPath,
		KernelPath:      kernelPath,
		RootFSPath:      rootfsPath,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "firecracker is not executable")
}
