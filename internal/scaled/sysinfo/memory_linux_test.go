package sysinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMemoryStats(t *testing.T) {
	stats, err := ReadMemoryStats()
	require.NoError(t, err)

	assert.Greater(t, stats.TotalMB, uint64(0))
	assert.Greater(t, stats.AvailableMB, uint64(0))
	assert.GreaterOrEqual(t, stats.TotalMB, stats.AvailableMB)
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
