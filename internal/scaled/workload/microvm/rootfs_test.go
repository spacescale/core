package microvm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareRootFSRejectsInvalidInput(t *testing.T) {
	tmp := t.TempDir()
	templatePath := filepath.Join(tmp, "template.ext4")
	targetPath := filepath.Join(tmp, "rootfs.ext4")
	require.NoError(t, os.WriteFile(templatePath, []byte("template"), 0o644))

	tests := []struct {
		name         string
		templatePath string
		targetPath   string
		wantErr      string
	}{
		{
			name:         "missing template path",
			templatePath: "",
			targetPath:   targetPath,
			wantErr:      "rootfs template path is required",
		},
		{
			name:         "missing target path",
			templatePath: templatePath,
			targetPath:   "",
			wantErr:      "rootfs target path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := prepareRootFS(tt.templatePath, tt.targetPath)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
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
