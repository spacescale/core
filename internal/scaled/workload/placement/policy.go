// Package placement validates microVM shape and capacity for one edge node.
//
// Placement owns shape-to-hardware-spec policy, local capacity accounting, and
// auction bidding. Executor owns targeted launch command handling, and microvm
// owns local Firecracker execution, so placement stays limited to shape and
// capacity decisions.
package placement

import (
	"errors"

	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

// ErrInvalidMicroVMShape is returned when the control plane sends a shape the
// edge cannot execute.
var ErrInvalidMicroVMShape = errors.New("invalid microvm shape")

// HardwareSpec is the capacity-relevant slice of one resolved microVM shape.
type HardwareSpec struct {
	VCPU uint32
	RAM  uint64

	IsPinned bool
}

const (
	// sharedVCPUOvercommitRatio defines how many shared guest vCPUs may be sold
	// per remaining physical host core. This ratio applies only to the shared
	// pool after host reserved cores and dedicated pinned workloads have already
	// been removed from the node capacity.
	//
	// Since SpaceScale operates on a Physical Core Truth model with SMT disabled,
	// a ratio of 4 ensures that even under maximum contention, each virtual CPU
	// is guaranteed at least 25% of a physical core's execution time. This balances
	// high-density multi-tenancy with deterministic, "snappy" performance.
	sharedVCPUOvercommitRatio uint32 = 4
	// Nodes up to and including this size reserve one host core.
	singleHostReserveCoreCeiling uint32 = 64
	// Smaller nodes keep one core for the host. Very large nodes keep two.
	baseHostReservedCores  uint32 = 1
	largeHostReservedCores uint32 = 2
)

const (
	// Node RAM thresholds in Megabytes. These determine the progressive host
	// memory tax applied before any customer workload capacity is exposed.
	node32GBMB uint64 = 32768
	node64GBMB uint64 = 65536
	// hostTax constants define the exact amount of RAM in Megabytes that is
	// withheld from the customer workload pool. This pays for the Linux kernel,
	// system services, the scaled daemon, network buffers, and microVM overhead.
	hostTax32GBMB     uint64 = 3686
	hostTax64GBMB     uint64 = 5324
	hostTaxOver64GBMB uint64 = 8601
)

func SpecFromShape(shape *pb.MicroVMShape) (HardwareSpec, error) {
	if shape == nil || shape.Vcpu == 0 || shape.RamMb == 0 {
		return HardwareSpec{}, ErrInvalidMicroVMShape
	}

	spec := HardwareSpec{VCPU: shape.Vcpu, RAM: shape.RamMb}
	switch shape.CpuMode {
	case pb.CpuMode_CPU_MODE_SHARED:
		return spec, nil
	case pb.CpuMode_CPU_MODE_PINNED:
		spec.IsPinned = true
		return spec, nil
	default:
		return HardwareSpec{}, ErrInvalidMicroVMShape
	}
}

func CpuModeLogValue(shape *pb.MicroVMShape) string {
	if shape == nil {
		return "unspecified"
	}
	if shape.CpuMode == pb.CpuMode_CPU_MODE_PINNED {
		return "pinned"
	}
	if shape.CpuMode == pb.CpuMode_CPU_MODE_SHARED {
		return "shared"
	}
	return "unspecified"
}

// hostReservedCores returns how many physical cores must always remain with the
// host operating system and control processes.
//
// The current policy is:
//
// 1. Nodes with up to 64 physical cores reserve 1 host core.
// 2. Nodes above 64 physical cores reserve 2 host cores.
//
// This policy assumes routing and other heavier data plane work is offloaded
// elsewhere, so the node host only needs a small protected CPU slice for
// scaled, systemd, tap and bridge work, and basic operating system activity.
func hostReservedCores(total uint32) uint32 {
	switch {
	case total == 0:
		return 0
	case total > singleHostReserveCoreCeiling:
		return largeHostReservedCores
	default:
		return baseHostReservedCores
	}
}

// sellableCores calculates the final pool of physical host cores available for
// customer workloads after the host reserve is removed.
//
// Callers pass the true physical core count discovered by host preflight, not
// logical thread counts.
func sellableCores(total uint32) uint32 {
	reserved := hostReservedCores(total)
	if total <= reserved {
		return 0
	}
	return total - reserved
}

// hostTaxRAMMB determines the exact amount of RAM in Megabytes that must be
// withheld from the workload pool based on the physical size of the server.
// Larger servers run more microVMs and require a larger networking and
// file descriptor buffer in the kernel.
func hostTaxRAMMB(total uint64) uint64 {
	switch {
	case total <= node32GBMB:
		return hostTax32GBMB
	case total <= node64GBMB:
		return hostTax64GBMB
	default:
		return hostTaxOver64GBMB
	}
}

// sellableRAMMB calculates the final pool of RAM available for customer workloads
// by subtracting the host tax from the total physical RAM.
func sellableRAMMB(total uint64) uint64 {
	tax := hostTaxRAMMB(total)
	if tax >= total {
		return 0
	}
	return total - tax
}
