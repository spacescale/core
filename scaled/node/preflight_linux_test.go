// Package node tests cover loadIdentity plus pure file and sysfs helpers.
// The following functions require real Linux host state (KVM, sysfs, /proc) or
// root privileges and are intentionally excluded from normal CI unit tests:
//
//   - ensureKVM                  requires /dev/kvm
//   - disableSwap / ensureSwapDisabled    require root + /proc/swaps
//   - disableKSM / disableSMT    requires sysfs filesystem
//   - ensureFirecrackerJailerAccount / createFirecrackerJailerUser / firecrackerJailerIdentity
//     requires root + useradd
//   - kvmDeviceGID               requires /dev/kvm + syscall.Stat_t
//   - preflight / validateRuntimePaths / Collect compose the above and are
//     better validated in privileged integration environments
package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadIdentityReadsValidIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"node_id":"node-1","region":"eu-central"}`), 0o600))

	identity, err := loadIdentity(path)
	require.NoError(t, err)
	assert.Equal(t, Identity{NodeID: "node-1", Region: "eu-central"}, identity)
}

func TestLoadIdentityMissingFile(t *testing.T) {
	_, err := loadIdentity(filepath.Join(t.TempDir(), "identity.json"))
	require.ErrorIs(t, err, errIdentityNotFound)
}

func TestLoadIdentityRejectsIncompleteIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"node_id":"node-1"}`), 0o600))

	_, err := loadIdentity(path)
	require.ErrorIs(t, err, errInvalidIdentity)
}

func TestValidateRuntimePath(t *testing.T) {
	t.Run("accepts regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

		err := validateRuntimePath("test", path, false)
		require.NoError(t, err)
	})

	t.Run("accepts executable file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bin")
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o755))

		err := validateRuntimePath("test", path, true)
		require.NoError(t, err)
	})

	t.Run("rejects missing file", func(t *testing.T) {
		err := validateRuntimePath("test", "/no/such/file", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such file")
	})

	t.Run("rejects directory", func(t *testing.T) {
		err := validateRuntimePath("test", t.TempDir(), false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is a directory")
	})

	t.Run("rejects empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty")
		require.NoError(t, os.WriteFile(path, nil, 0o644))

		err := validateRuntimePath("test", path, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is empty")
	})

	t.Run("rejects non-executable when executable required", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "script")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

		err := validateRuntimePath("test", path, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not executable")
	})

	t.Run("includes component name in error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")
		err := validateRuntimePath("firecracker", path, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "firecracker")
	})
}

func TestReadSysfsValue(t *testing.T) {
	t.Run("reads file content", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "value")
		require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0o644))

		val, err := readSysfsValue(path)
		require.NoError(t, err)
		assert.Equal(t, "hello", val)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "value")
		require.NoError(t, os.WriteFile(path, []byte("  42  \n"), 0o644))

		val, err := readSysfsValue(path)
		require.NoError(t, err)
		assert.Equal(t, "42", val)
	})

	t.Run("rejects missing path", func(t *testing.T) {
		_, err := readSysfsValue("/no/such/sysfs")
		require.Error(t, err)
	})
}

func TestWriteSysfsValue(t *testing.T) {
	t.Run("writes and reads back", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "value")
		require.NoError(t, writeSysfsValue(path, "1"))

		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "1", string(raw))
	})
}
