//go:build linux

package microvm

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// firecrackerPlan is the host-side Firecracker boot plan for one VM.
//
// The plan captures every host and jail-visible value in one struct so the
// SDK config builder stays a pure translation with no hidden state. Build
// the plan with (*Launcher).buildFirecrackerPlan, then translate it to the
// SDK Config with firecrackerConfigFromPlan.
type firecrackerPlan struct {
	SocketPath      string
	KernelImagePath string
	KernelArgs      string
	RootFSPath      string
	HostDevName     string
	MacAddress      string
	AllowMMDS       bool
	LogPath         string
	VCPUCount       int64
	MemSizeMib      int64
	Smt             bool
	VSockID         string
	VSockPath       string
	VSockCID        uint32
	MMDSAddress     net.IP

	Jailer firecrackerPlanJailer
}

type firecrackerPlanJailer struct {
	UID           int
	GID           int
	ID            string
	NumaNode      int
	ChrootBaseDir string
	KernelPath    string
	ExecFile      string
	JailerBinary  string
	CgroupVersion string
}

// buildFirecrackerPlan collects every host and jail-visible value for one VM
// boot attempt. Keeping the values in a dedicated struct makes the SDK config
// translation a pure function and gives callers a place to assert on the
// exact configuration that was sent to Firecracker.
func (l *Launcher) buildFirecrackerPlan(req LaunchRequest, workspace Workspace, cid uint32, network *Network) firecrackerPlan {
	return firecrackerPlan{
		SocketPath:      workspace.FirecrackerSocketPathInJail(),
		KernelImagePath: l.runtimePaths.KernelPath,
		KernelArgs:      guestdKernelArgs,
		RootFSPath:      workspace.RootFSPath,
		HostDevName:     network.TapName,
		MacAddress:      network.GuestMAC,
		AllowMMDS:       true,
		LogPath:         workspace.FirecrackerLogPathInJail(),
		VCPUCount:       int64(req.VCPU),
		MemSizeMib:      int64(req.RAMMB),
		Smt:             false,
		VSockID:         "guestd",
		VSockPath:       workspace.VSockPathInJail(),
		VSockCID:        cid,
		MMDSAddress:     network.MMDSIP,
		Jailer: firecrackerPlanJailer{
			UID:           l.jailerUID,
			GID:           l.jailerGID,
			ID:            req.MicroVMID,
			NumaNode:      0,
			ChrootBaseDir: workspace.JailerBaseDir,
			KernelPath:    l.runtimePaths.KernelPath,
			ExecFile:      l.runtimePaths.FirecrackerPath,
			JailerBinary:  l.runtimePaths.JailerPath,
			CgroupVersion: "2",
		},
	}
}

// startFirecracker builds the jailer-aware SDK config from a plan and starts
// the jailed VMM.
func (l *Launcher) startFirecracker(ctx context.Context, req LaunchRequest, vm *ActiveVM, jailerOutput io.Writer) error {
	plan := l.buildFirecrackerPlan(req, vm.Workspace, vm.GuestCID, vm.Network)
	machine, err := firecracker.NewMachine(ctx, firecrackerConfigFromPlan(plan, jailerOutput))
	if err != nil {
		return fmt.Errorf("create firecracker machine: %w", err)
	}

	// Keep normal SDK request/response chatter out of scaled stdout. Firecracker,
	// jailer, and guestd still write their diagnostic files in the VM workspace.
	machine.Logger().Logger.SetOutput(io.Discard)

	// Firecracker's FcInit handler chain configures the VM before InstanceStart.
	// Append metadata after the SDK has configured MMDS so guestd can read stable
	// boot metadata from inside the guest.
	machine.Handlers.FcInit = machine.Handlers.FcInit.AppendAfter(
		firecracker.ConfigMmdsHandlerName,
		firecracker.NewSetMetadataHandler(runtimeMetadataDocument(req)),
	)

	// Store the Machine before Start so cleanup can stop the VMM if startup
	// fails after the process is created.
	vm.machine = machine

	if err := machine.Start(ctx); err != nil {
		return fmt.Errorf("start firecracker machine: %w", err)
	}
	return nil
}

func firecrackerConfigFromPlan(plan firecrackerPlan, jailerOutput io.Writer) firecracker.Config {
	return firecracker.Config{
		SocketPath:      plan.SocketPath,
		KernelImagePath: plan.KernelImagePath,
		KernelArgs:      plan.KernelArgs,
		Drives:          firecracker.NewDrivesBuilder(plan.RootFSPath).Build(),
		NetworkInterfaces: firecracker.NetworkInterfaces{{
			StaticConfiguration: &firecracker.StaticNetworkConfiguration{
				HostDevName: plan.HostDevName,
				MacAddress:  plan.MacAddress,
			},
			AllowMMDS: plan.AllowMMDS,
		}},
		LogPath:  plan.LogPath,
		LogLevel: "Info",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  new(plan.VCPUCount),
			MemSizeMib: new(plan.MemSizeMib),
			Smt:        new(plan.Smt),
		},
		VsockDevices: []firecracker.VsockDevice{{
			ID:   plan.VSockID,
			Path: plan.VSockPath,
			CID:  plan.VSockCID,
		}},
		MmdsAddress: plan.MMDSAddress,
		MmdsVersion: firecracker.MMDSv2,
		JailerCfg: &firecracker.JailerConfig{
			UID:            new(plan.Jailer.UID),
			GID:            new(plan.Jailer.GID),
			ID:             plan.Jailer.ID,
			NumaNode:       new(plan.Jailer.NumaNode),
			ChrootBaseDir:  plan.Jailer.ChrootBaseDir,
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(plan.Jailer.KernelPath),
			ExecFile:       plan.Jailer.ExecFile,
			JailerBinary:   plan.Jailer.JailerBinary,
			Stdout:         jailerOutput,
			Stderr:         jailerOutput,
			CgroupVersion:  plan.Jailer.CgroupVersion,
		},
	}
}
