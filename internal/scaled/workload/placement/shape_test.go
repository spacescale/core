package placement

import (
	"testing"

	pb "github.com/spacescale/core/internal/shared/pb/v1"
	"github.com/stretchr/testify/require"
)

func TestSpecFromShapeDoesNotRequireRootDisk(t *testing.T) {
	spec, err := SpecFromShape(&pb.MicroVMShape{
		Vcpu:    2,
		RamMb:   512,
		CpuMode: pb.CpuMode_CPU_MODE_SHARED,
	})

	require.NoError(t, err)
	require.Equal(t, HardwareSpec{VCPU: 2, RAM: 512, IsPinned: false}, spec)
}

func TestSpecFromShapeValidation(t *testing.T) {
	tests := []struct {
		name  string
		shape *pb.MicroVMShape
	}{
		{name: "nil shape"},
		{
			name: "missing vcpu",
			shape: &pb.MicroVMShape{
				RamMb:   512,
				CpuMode: pb.CpuMode_CPU_MODE_SHARED,
			},
		},
		{
			name: "missing ram",
			shape: &pb.MicroVMShape{
				Vcpu:    2,
				CpuMode: pb.CpuMode_CPU_MODE_SHARED,
			},
		},
		{
			name: "unspecified cpu mode",
			shape: &pb.MicroVMShape{
				Vcpu:    2,
				RamMb:   512,
				CpuMode: pb.CpuMode_CPU_MODE_UNSPECIFIED,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SpecFromShape(tt.shape)
			require.ErrorIs(t, err, ErrInvalidMicroVMShape)
		})
	}
}

func TestSpecFromShapePinnedMode(t *testing.T) {
	spec, err := SpecFromShape(&pb.MicroVMShape{
		Vcpu:    4,
		RamMb:   1024,
		CpuMode: pb.CpuMode_CPU_MODE_PINNED,
	})

	require.NoError(t, err)
	require.Equal(t, HardwareSpec{VCPU: 4, RAM: 1024, IsPinned: true}, spec)
}
