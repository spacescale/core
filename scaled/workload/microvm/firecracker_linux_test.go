//go:build linux

package microvm

import (
	"net"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/spacescale/core/scaled/node"
	"github.com/stretchr/testify/require"
)

// firecrackerPlanInputsFixture returns one coherent launcher/VM setup for plan tests.
func firecrackerPlanInputsFixture() (*Launcher, Workspace, LaunchRequest, *Network, node.RuntimePaths) {
	paths := node.RuntimePaths{
		FirecrackerPath: "/usr/bin/firecracker",
		JailerPath:      "/usr/bin/jailer",
		KernelPath:      "/var/lib/spacescale/golden/vmlinux",
		RootFSPath:      "/var/lib/spacescale/golden/rootfs.ext4",
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

	network := &Network{
		TapName:   tapNameForMicroVM(req.MicroVMID),
		GuestMAC:  guestMACForMicroVM(req.MicroVMID),
		HostCIDR:  hostIPv4CIDR,
		GuestCIDR: guestIPv4CIDR,
		MMDSIP:    net.ParseIP(mmdsIPv4).To4(),
	}

	return launcher, workspace, req, network, paths
}

// firecrackerPlanFixture returns a literal Firecracker plan for config translation tests.
func firecrackerPlanFixture() firecrackerPlan {
	return firecrackerPlan{
		SocketPath:      "api.sock",
		KernelImagePath: "/var/lib/spacescale/golden/vmlinux",
		KernelArgs:      guestdKernelArgs,
		RootFSPath:      "/var/lib/spacescale/microvms/vm-123/rootfs.ext4",
		HostDevName:     "tapvm123",
		MacAddress:      "02:53:aa:bb:cc:dd",
		AllowMMDS:       true,
		LogPath:         "fc.log",
		VCPUCount:       2,
		MemSizeMib:      512,
		Smt:             false,
		VSockID:         "guestd",
		VSockPath:       "v.sock",
		VSockCID:        3,
		MMDSAddress:     net.ParseIP(mmdsIPv4).To4(),
		Jailer: firecrackerPlanJailer{
			UID:           123,
			GID:           456,
			ID:            "vm-123",
			NumaNode:      0,
			ChrootBaseDir: "/var/lib/spacescale/j",
			KernelPath:    "/var/lib/spacescale/golden/vmlinux",
			ExecFile:      "/usr/bin/firecracker",
			JailerBinary:  "/usr/bin/jailer",
			CgroupVersion: "2",
		},
	}
}

func TestBuildFirecrackerPlanBuildsExpectedPlan(t *testing.T) {
	launcher, workspace, req, network, paths := firecrackerPlanInputsFixture()

	plan := launcher.buildFirecrackerPlan(req, workspace, 3, network)

	require.Equal(t, workspace.FirecrackerSocketPathInJail(), plan.SocketPath)
	require.Equal(t, paths.KernelPath, plan.KernelImagePath)
	require.Equal(t, guestdKernelArgs, plan.KernelArgs)
	require.Equal(t, workspace.RootFSPath, plan.RootFSPath)
	require.Equal(t, network.TapName, plan.HostDevName)
	require.Equal(t, network.GuestMAC, plan.MacAddress)
	require.True(t, plan.AllowMMDS)
	require.Equal(t, workspace.FirecrackerLogPathInJail(), plan.LogPath)
	require.Equal(t, int64(req.VCPU), plan.VCPUCount)
	require.Equal(t, int64(req.RAMMB), plan.MemSizeMib)
	require.False(t, plan.Smt)
	require.Equal(t, "guestd", plan.VSockID)
	require.Equal(t, workspace.VSockPathInJail(), plan.VSockPath)
	require.Equal(t, uint32(3), plan.VSockCID)
	require.Equal(t, network.MMDSIP, plan.MMDSAddress)

	require.Equal(t, 123, plan.Jailer.UID)
	require.Equal(t, 456, plan.Jailer.GID)
	require.Equal(t, req.MicroVMID, plan.Jailer.ID)
	require.Zero(t, plan.Jailer.NumaNode)
	require.Equal(t, workspace.JailerBaseDir, plan.Jailer.ChrootBaseDir)
	require.Equal(t, paths.KernelPath, plan.Jailer.KernelPath)
	require.Equal(t, paths.FirecrackerPath, plan.Jailer.ExecFile)
	require.Equal(t, paths.JailerPath, plan.Jailer.JailerBinary)
	require.Equal(t, "2", plan.Jailer.CgroupVersion)
}

func TestFirecrackerConfigFromPlanBuildsSDKConfig(t *testing.T) {
	plan := firecrackerPlanFixture()
	cfg := firecrackerConfigFromPlan(plan, nil)

	require.Equal(t, plan.SocketPath, cfg.SocketPath)
	require.Equal(t, plan.LogPath, cfg.LogPath)
	require.Equal(t, guestdKernelArgs, cfg.KernelArgs)
	require.Equal(t, plan.KernelImagePath, cfg.KernelImagePath)

	require.Len(t, cfg.VsockDevices, 1)
	require.Equal(t, plan.VSockID, cfg.VsockDevices[0].ID)
	require.Equal(t, plan.VSockPath, cfg.VsockDevices[0].Path)
	require.Equal(t, plan.VSockCID, cfg.VsockDevices[0].CID)

	require.Len(t, cfg.NetworkInterfaces, 1)
	require.True(t, cfg.NetworkInterfaces[0].AllowMMDS)
	require.NotNil(t, cfg.NetworkInterfaces[0].StaticConfiguration)
	require.Equal(t, plan.HostDevName, cfg.NetworkInterfaces[0].StaticConfiguration.HostDevName)
	require.Equal(t, plan.MacAddress, cfg.NetworkInterfaces[0].StaticConfiguration.MacAddress)
	require.Nil(t, cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration)
	require.Equal(t, plan.MMDSAddress.String(), cfg.MmdsAddress)
	require.Equal(t, firecracker.MMDSv2, cfg.MmdsVersion)

	require.Len(t, cfg.Drives, 1)
	require.Equal(t, plan.RootFSPath, firecracker.StringValue(cfg.Drives[0].PathOnHost))

	require.Equal(t, plan.VCPUCount, firecracker.Int64Value(cfg.MachineCfg.VcpuCount))
	require.Equal(t, plan.MemSizeMib, firecracker.Int64Value(cfg.MachineCfg.MemSizeMib))
	require.False(t, firecracker.BoolValue(cfg.MachineCfg.Smt))

	require.NotNil(t, cfg.JailerCfg)
	require.Equal(t, plan.Jailer.UID, *cfg.JailerCfg.UID)
	require.Equal(t, plan.Jailer.GID, *cfg.JailerCfg.GID)
	require.Equal(t, plan.Jailer.ID, cfg.JailerCfg.ID)
	require.Equal(t, plan.Jailer.ChrootBaseDir, cfg.JailerCfg.ChrootBaseDir)
	require.Equal(t, plan.Jailer.ExecFile, cfg.JailerCfg.ExecFile)
	require.Equal(t, plan.Jailer.JailerBinary, cfg.JailerCfg.JailerBinary)
	require.Equal(t, plan.Jailer.CgroupVersion, cfg.JailerCfg.CgroupVersion)
}
