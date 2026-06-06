package microvm

import (
	"context"
	"fmt"
	"io"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// startFirecracker builds the jailer-aware SDK config and starts the jailed VMM.
func (l *Launcher) startFirecracker(ctx context.Context, req LaunchRequest, vm *ActiveVM, jailerOutput io.Writer) error {
	plan := l.buildFirecrackerPlan(req, vm.Workspace, vm.GuestCID, vm.Network, jailerOutput)
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
		firecracker.NewSetMetadataHandler(map[string]any{
			"version":    1,
			"microvm_id": req.MicroVMID,
		}),
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
