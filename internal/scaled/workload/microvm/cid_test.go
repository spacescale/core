package microvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCIDAllocatorStartsAtThree(t *testing.T) {
	a := newCIDAllocator()
	cid, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), cid)
}
func TestCIDAllocatorReusesReleasedCID(t *testing.T) {
	a := newCIDAllocator()
	first, err := a.Acquire()
	require.NoError(t, err)
	second, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), first)
	require.Equal(t, uint32(4), second)
	a.Release(first)
	reused, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), reused)
}

func TestCIDAllocatorIgnoresReservedReleaseValues(t *testing.T) {
	a := newCIDAllocator()
	a.Release(0)
	a.Release(1)
	a.Release(2)
	cid, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), cid)
}
