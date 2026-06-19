package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSnapshot(t *testing.T) {
	t.Run("reads snapshot", func(t *testing.T) {
		root := t.TempDir()
		bootIDPath := filepath.Join(root, "boot_id")
		memInfoPath := filepath.Join(root, "meminfo")
		cpuRoot := filepath.Join(root, "sys", "devices", "system", "cpu")

		require.NoError(t, os.WriteFile(bootIDPath, []byte("11111111-2222-3333-4444-555555555555\n"), 0o644))
		require.NoError(t, os.WriteFile(memInfoPath, []byte("MemTotal:       2097152 kB\n"), 0o644))
		writeCPUFixture(t, cpuRoot, "cpu0", "0", "0")
		writeCPUFixture(t, cpuRoot, "cpu1", "1", "0")

		snapshot, err := readSnapshot(
			bootIDPath,
			memInfoPath,
			filepath.Join(cpuRoot, "cpu[0-9]*", "topology", "core_id"),
			root,
		)
		require.NoError(t, err)

		assert.Equal(t, "11111111-2222-3333-4444-555555555555", snapshot.BootID)
		assert.Equal(t, uint32(2), snapshot.TotalCores)
		assert.Positive(t, snapshot.TotalThreads)
		assert.Equal(t, uint64(2048), snapshot.TotalRAMMb)
		assert.Positive(t, snapshot.TotalDiskMb)
		assert.GreaterOrEqual(t, snapshot.TotalDiskMb, snapshot.AvailableDiskMb)
	})

	t.Run("propagates errors", func(t *testing.T) {
		_, err := readSnapshot(
			"/no/such/boot_id",
			"/no/such/meminfo",
			"/no/such/cpu/*/topology/core_id",
			"/no/such/root",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read boot id")
	})
}

func TestReadBootID(t *testing.T) {
	t.Run("reads valid boot id", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "boot_id")
		require.NoError(t, os.WriteFile(path, []byte("11111111-2222-3333-4444-555555555555\n"), 0o644))

		id, err := readBootID(path)
		require.NoError(t, err)
		assert.Equal(t, "11111111-2222-3333-4444-555555555555", id)
	})

	t.Run("rejects empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "boot_id")
		require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

		_, err := readBootID(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty value")
	})

	t.Run("rejects missing file", func(t *testing.T) {
		_, err := readBootID("/no/such/boot_id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read boot id")
	})
}

func TestReadMemoryStats(t *testing.T) {
	t.Run("reads MemTotal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "meminfo")
		require.NoError(t, os.WriteFile(path, []byte("MemTotal:       2097152 kB\n"), 0o644))

		stats, err := readMemoryStats(path)
		require.NoError(t, err)
		assert.Equal(t, uint64(2048), stats.TotalMb)
	})

	t.Run("rejects missing MemTotal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "meminfo")
		require.NoError(t, os.WriteFile(path, []byte("MemFree:       1024 kB\n"), 0o644))

		_, err := readMemoryStats(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MemTotal not found")
	})

	t.Run("rejects missing file", func(t *testing.T) {
		_, err := readMemoryStats("/no/such/meminfo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "open")
	})
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

func TestReadPhysicalCoreCount(t *testing.T) {
	t.Run("counts distinct package and core pairs", func(t *testing.T) {
		root := t.TempDir()
		writeCPUFixture(t, root, "cpu0", "0", "0")
		writeCPUFixture(t, root, "cpu1", "1", "0")
		writeCPUFixture(t, root, "cpu2", "0", "1")

		count, err := readPhysicalCoreCount(filepath.Join(root, "cpu[0-9]*", "topology", "core_id"))
		require.NoError(t, err)
		assert.Equal(t, uint32(3), count)
	})

	t.Run("deduplicates sibling threads on one physical core", func(t *testing.T) {
		root := t.TempDir()
		writeCPUFixture(t, root, "cpu0", "0", "0")
		writeCPUFixture(t, root, "cpu1", "0", "0")

		count, err := readPhysicalCoreCount(filepath.Join(root, "cpu[0-9]*", "topology", "core_id"))
		require.NoError(t, err)
		assert.Equal(t, uint32(1), count)
	})
}

func TestReadTopologyValueRejectsMissingPath(t *testing.T) {
	_, err := readTopologyValue("/path/that/should/not/exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read topology")
}

func TestReadTopologyValue(t *testing.T) {
	t.Run("reads value", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "topology_value")
		require.NoError(t, os.WriteFile(path, []byte("42\n"), 0o644))

		value, err := readTopologyValue(path)
		require.NoError(t, err)
		assert.Equal(t, "42", value)
	})

	t.Run("rejects missing path", func(t *testing.T) {
		_, err := readTopologyValue("/no/such/topology")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read topology")
	})

	t.Run("rejects empty value", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "topology_value")
		require.NoError(t, os.WriteFile(path, []byte(" \n"), 0o644))

		_, err := readTopologyValue(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty value")
	})
}

func writeCPUFixture(t *testing.T, root, cpu, coreID, packageID string) {
	t.Helper()
	topology := filepath.Join(root, cpu, "topology")
	require.NoError(t, os.MkdirAll(topology, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(topology, "core_id"), []byte(coreID+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(topology, "physical_package_id"), []byte(packageID+"\n"), 0o644))
}
