package sysinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadDiskStats(t *testing.T) {
	stats, err := ReadDiskStats("")
	require.NoError(t, err)

	assert.Greater(t, stats.TotalMB, uint64(0))
	assert.GreaterOrEqual(t, stats.TotalMB, stats.AvailableMB)
}

func TestReadDiskStatsRejectsMissingPath(t *testing.T) {
	_, err := ReadDiskStats("/path/that/should/not/exist")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "statfs")
}
