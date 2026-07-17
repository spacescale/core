//go:build linux

package microvm

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

func TestPIDsFromCgroupTree(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vm-a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested", "vm-b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cgroup.procs"), []byte("42\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "vm-a", "cgroup.procs"), []byte("7\n42\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "vm-b", "cgroup.procs"), []byte("99\n"), 0o644))

	pids, err := pidsFromCgroupTree(root)

	require.NoError(t, err)
	require.Equal(t, []int{7, 42, 99}, pids)
}

func TestPIDsFromMissingCgroupTreeIsEmpty(t *testing.T) {
	pids, err := pidsFromCgroupTree(filepath.Join(t.TempDir(), "missing"))

	require.NoError(t, err)
	require.Empty(t, pids)
}

func TestPIDsOwnedByUID(t *testing.T) {
	root := t.TempDir()
	writeProcessStatus(t, root, 12, 1234)
	writeProcessStatus(t, root, 13, 5678)
	require.NoError(t, os.Mkdir(filepath.Join(root, "not-a-pid"), 0o755))

	pids, err := pidsOwnedByUID(root, 1234)

	require.NoError(t, err)
	require.Equal(t, []int{12}, pids)
}

func TestPIDsOwnedByUIDRejectsRoot(t *testing.T) {
	_, err := pidsOwnedByUID(t.TempDir(), 0)

	require.ErrorContains(t, err, "unsafe jailer uid")
}

func TestKillOrphanPIDsTerminatesProcess(t *testing.T) {
	commandCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(commandCtx, "sleep", "30")
	require.NoError(t, command.Start())
	waited := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(waited)
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	killOrphanPIDs(context.Background(), logger, []int{command.Process.Pid})

	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("orphan process was not reaped")
	}
}

func TestIsSpaceScaleTAP(t *testing.T) {
	tests := []struct {
		name string
		link netlink.Link
		want bool
	}{
		{
			name: "generated name",
			link: &netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{Name: "tap0123456789a"}},
			want: true,
		},
		{
			name: "sentinel mac",
			link: &netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{
				Name:         "tap-legacy",
				HardwareAddr: net.HardwareAddr{0x02, 0x53, 0xaa, 0xbb, 0xcc, 0xdd},
			}},
			want: true,
		},
		{
			name: "unrelated tap",
			link: &netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{Name: "tap0"}},
			want: false,
		},
		{
			name: "invalid hex suffix",
			link: &netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{Name: "tap0123456789z"}},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isSpaceScaleTAP(test.link))
		})
	}
}

func TestCleanupStatePathRemovesTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "microvms")
	require.NoError(t, os.MkdirAll(filepath.Join(path, "vm-123"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "vm-123", "api.sock"), []byte("stale"), 0o644))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cleanupStatePath(logger, path)

	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)

	// A second pass is intentionally a no-op.
	cleanupStatePath(logger, path)
}

func writeProcessStatus(t *testing.T, root string, pid, uid int) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	require.NoError(t, os.Mkdir(dir, 0o755))
	status := "Name:\tfirecracker\nUid:\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644))
}
