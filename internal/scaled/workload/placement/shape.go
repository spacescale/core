package placement

import (
	"errors"

	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

// ErrInvalidMicroVMShape is returned when the control plane sends a shape the
// edge cannot execute.
var ErrInvalidMicroVMShape = errors.New("invalid microvm shape")

// HardwareSpec is the capacity relevant slice of one resolved microvm shape.
//
// The edge capacity ledger only needs CPU mode, vcpu count, and RAM to answer
// placement questions. Disk fields stay on the transport for later runtime work.
type HardwareSpec struct {
	VCPU uint32
	RAM  uint64

	IsPinned bool
}

func specFromShape(shape *pb.MicroVMShape) (HardwareSpec, error) {
	if shape == nil || shape.Vcpu == 0 || shape.RamMb == 0 || shape.RootDiskMb == 0 {
		return HardwareSpec{}, ErrInvalidMicroVMShape
	}

	switch shape.CpuMode {
	case pb.CpuMode_CPU_MODE_SHARED:
		return HardwareSpec{VCPU: shape.Vcpu, RAM: shape.RamMb, IsPinned: false}, nil
	case pb.CpuMode_CPU_MODE_PINNED:
		return HardwareSpec{VCPU: shape.Vcpu, RAM: shape.RamMb, IsPinned: true}, nil
	default:
		return HardwareSpec{}, ErrInvalidMicroVMShape
	}
}

func cpuModeLogValue(shape *pb.MicroVMShape) string {
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
