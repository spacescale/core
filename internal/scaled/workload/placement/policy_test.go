package placement

import (
	"testing"

	pb "github.com/spacescale/core/internal/shared/pb/v1"
	"github.com/stretchr/testify/assert"
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

func TestSellableRAMMB(t *testing.T) {
	tests := []struct {
		name  string
		total uint64
		want  uint64
	}{
		{name: "thirty two gb node", total: 32768, want: 29082},
		{name: "sixty four gb node", total: 65536, want: 60212},
		{name: "one hundred twenty eight gb node", total: 131072, want: 122471},
		{name: "two hundred fifty six gb node", total: 262144, want: 253543},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, sellableRAMMB(tc.total), tc.name)
	}
}

func TestSellableCores(t *testing.T) {
	tests := []struct {
		name  string
		total uint32
		want  uint32
	}{
		{name: "zero", total: 0, want: 0},
		{name: "one core node", total: 1, want: 0},
		{name: "two core node", total: 2, want: 1},
		{name: "thirty two core node", total: 32, want: 31},
		{name: "forty eight core node", total: 48, want: 47},
		{name: "sixty four core node", total: 64, want: 63},
		{name: "sixty five core node", total: 65, want: 63},
		{name: "one hundred twenty eight core node", total: 128, want: 126},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, sellableCores(tc.total), tc.name)
	}
}

func TestHostReservedCores(t *testing.T) {
	tests := []struct {
		name  string
		total uint32
		want  uint32
	}{
		{name: "zero", total: 0, want: 0},
		{name: "small node", total: 1, want: 1},
		{name: "medium node", total: 32, want: 1},
		{name: "sixty four core node", total: 64, want: 1},
		{name: "large node", total: 65, want: 2},
		{name: "very large node", total: 128, want: 2},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, hostReservedCores(tc.total), tc.name)
	}
}
