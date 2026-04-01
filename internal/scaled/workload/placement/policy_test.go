package placement

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSellableRAMMB(t *testing.T) {
	tests := []struct {
		total uint64
		want  uint64
	}{
		{32768, 29082},
		{65536, 60212},
		{131072, 122471},
		{262144, 253543},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, sellableRAMMB(tc.total))
	}
}

func TestSellableThreads(t *testing.T) {
	tests := []struct {
		total uint32
		want  uint32
	}{
		{32, 30},
		{48, 46},
		{2, 0},
		{1, 0},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, sellableThreads(tc.total))
	}
}
