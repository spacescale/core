package microvm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWorkspaceBuildsExpectedPaths(t *testing.T) {
	w := newWorkspace(
		"/var/lib/spacescale/microvms",
		"/var/lib/spacescale/j",
		"vm-123",
		"/var/lib/spacescale/runtime/host/firecracker-v1.15.1-x86_64",
	)

	require.Equal(t, "vm-123", w.MicroVMID)
	require.Equal(t, "/var/lib/spacescale/microvms/vm-123", w.RootDir)
	require.Equal(t, "/var/lib/spacescale/j", w.JailerBaseDir)
	require.Equal(
		t,
		"/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/vm-123",
		w.JailerDir,
	)
	require.Equal(
		t,
		"/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/vm-123/root",
		w.JailerRootDir,
	)
	require.Equal(t, "/var/lib/spacescale/microvms/vm-123/rootfs.ext4", w.RootFSPath)
	require.Equal(
		t,
		"/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/vm-123/root/api.sock",
		w.FirecrackerSocketHostPath(),
	)
	require.Equal(t, "api.sock", w.FirecrackerSocketPathInJail())
	require.Equal(
		t,
		"/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/vm-123/root/v.sock",
		w.VSockHostPath(),
	)
	require.Equal(t, "v.sock", w.VSockPathInJail())
	require.Equal(
		t,
		"/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/vm-123/root/fc.log",
		w.FirecrackerLogHostPath(),
	)
	require.Equal(t, "fc.log", w.FirecrackerLogPathInJail())
}

func TestNewWorkspaceUsesOneMicroVMIdentity(t *testing.T) {
	const microvmID = "9f1d9a3e-8d4a-44fd-80df-2e5c41d82c73"

	w := newWorkspace(
		microVMStateDir,
		microVMJailerStateDir,
		microvmID,
		"/var/lib/spacescale/runtime/host/firecracker-v1.15.1-x86_64",
	)

	require.Equal(t, microvmID, w.MicroVMID)
	require.Equal(t, filepath.Join(microVMStateDir, microvmID), w.RootDir)
	require.Equal(
		t,
		filepath.Join(microVMJailerStateDir, "firecracker-v1.15.1-x86_64", microvmID),
		w.JailerDir,
	)
}

func TestNewWorkspaceKeepsSocketPathsShortForUUID(t *testing.T) {
	const linuxUnixSocketPathMaxLen = 107

	w := newWorkspace(
		microVMStateDir,
		microVMJailerStateDir,
		"9f1d9a3e-8d4a-44fd-80df-2e5c41d82c73",
		"/var/lib/spacescale/runtime/host/firecracker-v1.15.1-x86_64",
	)

	require.LessOrEqual(t, len(w.FirecrackerSocketHostPath()), linuxUnixSocketPathMaxLen)
	require.LessOrEqual(t, len(vsockPortPath(w.VSockHostPath(), controlPort)), linuxUnixSocketPathMaxLen)
	require.LessOrEqual(t, len(vsockPortPath(w.VSockHostPath(), logPort)), linuxUnixSocketPathMaxLen)
}

func TestWorkspacePrepareAndCleanup(t *testing.T) {
	root := t.TempDir()
	w := newWorkspace(
		filepath.Join(root, "microvms"),
		filepath.Join(root, "j"),
		"vm-123",
		"/runtime/firecracker-v1.15.1-x86_64",
	)

	require.NoError(t, w.Prepare())

	rootInfo, err := os.Stat(w.RootDir)
	require.NoError(t, err)
	require.True(t, rootInfo.IsDir())

	jailInfo, err := os.Stat(w.JailerRootDir)
	require.NoError(t, err)
	require.True(t, jailInfo.IsDir())

	// RootFSPath is still a file path to be populated later, not a directory.
	_, err = os.Stat(w.RootFSPath)
	require.True(t, os.IsNotExist(err))

	testFile := filepath.Join(w.RootDir, "marker")
	require.NoError(t, os.WriteFile(testFile, []byte("ok"), 0o644))
	require.NoError(t, w.Cleanup())

	_, err = os.Stat(w.RootDir)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(w.JailerDir)
	require.True(t, os.IsNotExist(err))
}

func TestPrepareRootFSCopiesTemplateAsIs(t *testing.T) {
	tmp := t.TempDir()
	templatePath := filepath.Join(tmp, "template.ext4")
	targetPath := filepath.Join(tmp, "rootfs.ext4")
	templateBytes := []byte("scoutd-rootfs-template")
	require.NoError(t, os.WriteFile(templatePath, templateBytes, 0o644))

	require.NoError(t, prepareRootFS(templatePath, targetPath))

	targetBytes, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, templateBytes, targetBytes)

	targetInfo, err := os.Stat(targetPath)
	require.NoError(t, err)
	require.Equal(t, int64(len(templateBytes)), targetInfo.Size())

	templateInfo, err := os.Stat(templatePath)
	require.NoError(t, err)
	require.Equal(t, int64(len(templateBytes)), templateInfo.Size())
}

func TestPrepareRootFSReturnsCopyErrorWhenTemplateMissing(t *testing.T) {
	tmp := t.TempDir()
	templatePath := filepath.Join(tmp, "missing-template.ext4")
	targetPath := filepath.Join(tmp, "rootfs.ext4")

	err := prepareRootFS(templatePath, targetPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "copy rootfs template")
	require.Contains(t, err.Error(), "open source file")
}

func TestCopyFileOverwritesExistingTarget(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "source")
	targetPath := filepath.Join(tmp, "target")
	require.NoError(t, os.WriteFile(sourcePath, []byte("new-rootfs"), 0o644))
	require.NoError(t, os.WriteFile(targetPath, []byte("old-rootfs-data"), 0o644))

	require.NoError(t, copyFile(sourcePath, targetPath))

	targetBytes, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, []byte("new-rootfs"), targetBytes)
}

func TestCleanupStaleStateRemovesMicroVMAndJailerState(t *testing.T) {
	root := t.TempDir()
	microVMRoot := filepath.Join(root, "microvms")
	jailerRoot := filepath.Join(root, "j")

	currentJailerTree := filepath.Join(jailerRoot, "firecracker-v1.15.1-x86_64", "vm-123")
	otherJailerTree := filepath.Join(jailerRoot, "firecracker-v1.14.0-x86_64", "vm-456")

	require.NoError(t, os.MkdirAll(filepath.Join(microVMRoot, "vm-123"), 0o755))
	require.NoError(t, os.MkdirAll(currentJailerTree, 0o755))
	require.NoError(t, os.MkdirAll(otherJailerTree, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(microVMRoot, "vm-123", "rootfs.ext4"), []byte("rootfs"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(currentJailerTree, "api.sock"), []byte("socket"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(otherJailerTree, "api.sock"), []byte("socket"), 0o644))

	err := cleanupStaleState(microVMRoot, jailerRoot)
	require.NoError(t, err)

	_, err = os.Stat(microVMRoot)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(jailerRoot)
	require.True(t, os.IsNotExist(err))
}
