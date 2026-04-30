// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

//go:build linux

package microvm

import (
	"testing"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/spacescale/core/internal/scaled/runtime"
	"github.com/stretchr/testify/require"
)

func TestBuildFirecrackerConfigUsesJailVisiblePaths(t *testing.T) {
	paths := runtime.Paths{
		FirecrackerPath: "/var/lib/spacescale/runtime/host/firecracker-v1.15.1-x86_64",
		JailerPath:      "/var/lib/spacescale/runtime/host/jailer-v1.15.1-x86_64",
		KernelPath:      "/var/lib/spacescale/runtime/guest/vmlinux-v6.1.80-x86_64",
		RootFSPath:      "/var/lib/spacescale/runtime/guest/scoutd-rootfs-v0.1.3-x86_64-ext4",
	}

	launcher := &Launcher{
		runtimePaths: paths,
		jailerUID:    123,
		jailerGID:    456,
	}

	workspace := newWorkspace(
		"/var/lib/spacescale/microvms",
		"/var/lib/spacescale/j",
		"vm-123",
		paths.FirecrackerPath,
	)

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
