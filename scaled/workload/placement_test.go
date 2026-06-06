package workload

import (
	"testing"
	"time"

	pb "github.com/spacescale/core/shared/pb/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapacityReserveSharedPoolLimit(t *testing.T) {
	capacity := NewCapacity(131072, 8)
	spec := HardwareSpec{VCPU: 4, RAM: 8192, IsPinned: false}

	for i := 0; i < 7; i++ {
		microvmID := "growth-" + string(rune('a'+i))
		_, ok := capacity.Reserve(microvmID, spec, time.Second)
		require.True(t, ok)
	}

	_, ok := capacity.Reserve("growth-overflow", spec, time.Second)
	assert.False(t, ok)
}

func TestCapacityReservePinnedLimit(t *testing.T) {
	capacity := NewCapacity(131072, 10)
	spec := HardwareSpec{VCPU: 8, RAM: 16384, IsPinned: true}

	_, ok := capacity.Reserve("scale-1", spec, time.Second)
	require.True(t, ok)
	_, ok = capacity.Reserve("scale-2", spec, time.Second)
	assert.False(t, ok)
}

func TestCapacityCommitAndRevert(t *testing.T) {
	capacity := NewCapacity(65536, 8)
	spec := HardwareSpec{VCPU: 4, RAM: 8192, IsPinned: false}

	_, ok := capacity.Reserve("microvm-1", spec, time.Second)
	require.True(t, ok)

	committed, ok := capacity.Commit("microvm-1")
	require.True(t, ok)
	assert.Equal(t, spec, committed)
	assert.Equal(t, spec.RAM, capacity.usedRAMMB)
	assert.Equal(t, spec.VCPU, capacity.usedSharedVCPU)

	capacity.Revert(committed)
	assert.Zero(t, capacity.usedRAMMB)
	assert.Zero(t, capacity.usedSharedVCPU)
}

func TestCapacityReleaseExpired(t *testing.T) {
	capacity := NewCapacity(65536, 8)
	spec := HardwareSpec{VCPU: 2, RAM: 4096, IsPinned: false}

	_, ok := capacity.Reserve("microvm-1", spec, 10*time.Millisecond)
	require.True(t, ok)

	capacity.ReleaseExpired(time.Now().Add(time.Second))

	_, exists := capacity.reservations["microvm-1"]
	assert.False(t, exists)
	assert.Zero(t, capacity.reservedRAMMB)
	assert.Zero(t, capacity.reservedSharedVCPU)
}

func TestCapacityFreeMathClampsAtZero(t *testing.T) {
	capacity := NewCapacity(1024, 4)
	capacity.usedRAMMB = 900
	capacity.reservedRAMMB = 200
	capacity.usedPinnedCores = 3
	capacity.reservedPinnedCores = 2
	capacity.usedSharedVCPU = 10
	capacity.reservedSharedVCPU = 10

	assert.Zero(t, capacity.freeRAMMBLocked())
	assert.Zero(t, capacity.freePinnedCoresLocked())
	assert.Zero(t, capacity.freeSharedVCPULocked())
}

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
