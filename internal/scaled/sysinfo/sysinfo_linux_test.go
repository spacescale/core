package sysinfo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead(t *testing.T) {
	snapshot, err := Read()
	require.NoError(t, err)

	assert.NotEmpty(t, snapshot.BootID)
	assert.Equal(t, strings.TrimSpace(snapshot.BootID), snapshot.BootID)
	assert.Len(t, snapshot.BootID, 36)
	assert.Contains(t, snapshot.BootID, "-")
	assert.Greater(t, snapshot.TotalThreads, uint32(0))
	assert.Greater(t, snapshot.TotalRamMb, uint64(0))
	assert.Greater(t, snapshot.AvailableRamMb, uint64(0))
	assert.GreaterOrEqual(t, snapshot.TotalRamMb, snapshot.AvailableRamMb)
	assert.Greater(t, snapshot.TotalDiskMb, uint64(0))
	assert.GreaterOrEqual(t, snapshot.TotalDiskMb, snapshot.AvailableDiskMb)
}

func TestParseMemInfoKBLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		label       string
		wantValue   uint64
		wantErrText string
	}{
		{
			name:      "parses megabytes",
			line:      "MemTotal:       2097152 kB",
			label:     "MemTotal:",
			wantValue: 2048,
		},
		{
			name:        "rejects unexpected unit",
			line:        "MemAvailable:       1024 MB",
			label:       "MemAvailable:",
			wantErrText: "unexpected MemAvailable unit",
		},
		{
			name:        "rejects wrong label",
			line:        "MemFree:       1024 kB",
			label:       "MemAvailable:",
			wantErrText: "unexpected meminfo label",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			value, err := parseMemInfoKBLine(tc.line, tc.label)

			if tc.wantErrText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrText)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, value)
		})
	}
}

func TestReadDiskStatsRejectsMissingPath(t *testing.T) {
	_, err := readDiskStats("/path/that/should/not/exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "statfs")
}
