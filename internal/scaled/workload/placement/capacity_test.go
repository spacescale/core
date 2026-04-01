package placement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapacityReserveSharedPoolLimit(t *testing.T) {
	capacity := NewCapacity(131072, 8)
	spec := HardwareSpec{VCPU: 4, RAM: 8192, IsPinned: false}

	for i := 0; i < 4; i++ {
		machineID := "growth-" + string(rune('a'+i))
		_, ok := capacity.Reserve(machineID, spec, time.Second)
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

	_, ok := capacity.Reserve("machine-1", spec, time.Second)
	require.True(t, ok)

	committed, ok := capacity.Commit("machine-1")
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

	_, ok := capacity.Reserve("machine-1", spec, 10*time.Millisecond)
	require.True(t, ok)

	capacity.ReleaseExpired(time.Now().Add(time.Second))

	_, exists := capacity.reservations["machine-1"]
	assert.False(t, exists)
	assert.Zero(t, capacity.reservedRAMMB)
	assert.Zero(t, capacity.reservedSharedVCPU)
}

func TestCapacityFreeMathClampsAtZero(t *testing.T) {
	capacity := NewCapacity(1024, 4)
	capacity.usedRAMMB = 900
	capacity.reservedRAMMB = 200
	capacity.usedPinnedThreads = 3
	capacity.reservedPinnedThreads = 2
	capacity.usedSharedVCPU = 10
	capacity.reservedSharedVCPU = 10

	assert.Zero(t, capacity.freeRAMMBLocked())
	assert.Zero(t, capacity.freePinnedThreadsLocked())
	assert.Zero(t, capacity.freeSharedVCPULocked())
}
