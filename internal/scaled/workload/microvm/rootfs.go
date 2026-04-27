package microvm

import (
	"fmt"
	"io"
	"os"
)

// prepareRootFS copies the shared scoutd rootfs template into the VM workspace.
//
// The scoutd boot image is platform-managed. The launcher does not accept a
// user-facing root disk size and does not grow the template; later OCI workload
// storage should be modeled as platform-managed ephemeral space or volumes.
func prepareRootFS(templatePath, targetPath string) error {
	if templatePath == "" {
		return fmt.Errorf("rootfs template path is required")
	}
	if targetPath == "" {
		return fmt.Errorf("rootfs target path is required")
	}

	if err := copyFile(templatePath, targetPath); err != nil {
		return fmt.Errorf("copy rootfs template: %w", err)
	}
	return nil
}

// copyFile performs a straightforward full-file copy from src to dst.
//
// We keep this helper local and explicit rather than pulling in a generic file
// abstraction because the first Firecracker milestone only needs one concrete
// file copy path for the rootfs template.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer source.Close()
	dest, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open destination file: %w", err)
	}
	defer dest.Close()
	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("copy bytes: %w", err)
	}
	if err := dest.Sync(); err != nil {
		return fmt.Errorf("sync destination file: %w", err)
	}
	return nil
}
