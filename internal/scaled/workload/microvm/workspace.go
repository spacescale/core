// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// Package microvm owns the host-side Firecracker boot path for SpaceScale
// workloads.
//
// The package prepares local host state, starts one jailed Firecracker VM, and
// accepts launch only after scoutd proves guest userspace is alive with a hello
// frame over virtio-vsock. Startup, placement, and executor code validate
// runtime assets, shape values, reservations, and duplicate launches before this
// package is called. microvm intentionally avoids duplicating those checks so the
// local lifecycle path stays simple.
//
// File boundaries stay concrete: launcher.go owns Firecracker lifecycle,
// workspace.go owns host paths and rootfs files, network files own TAP/MMDS host
// network state, and vsock.go owns guest CIDs, host-side vsock listeners, and
// scoutd hello parsing.
package microvm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// microVMStateDir stores full-identity VM workspaces keyed by the
	// control-plane microVM UUID.
	microVMStateDir = "/var/lib/spacescale/microvms"

	// microVMJailerStateDir stores jailer paths under a short root. The full
	// microVM UUID is still used in the jailer path, but the short root plus short
	// socket filenames keep Unix socket paths within Linux's small limit.
	microVMJailerStateDir = "/var/lib/spacescale/j"
)

const (
	firecrackerSocketName = "api.sock"
	vsockSocketName       = "v.sock"
	firecrackerLogName    = "fc.log"
)

// Workspace describes the per-microVM host directory layout.
//
// RootDir is the outer workspace that SpaceScale manages directly. JailerBaseDir
// is the input directory handed to the jailer. JailerRootDir is the final chroot
// directory that becomes Firecracker's root filesystem view.
type Workspace struct {
	// MicroVMID is the full control-plane identity for this VM.
	MicroVMID string

	// RootDir is the outer per-VM workspace owned by SpaceScale.
	RootDir string

	// JailerBaseDir is the chroot base we hand to the SDK jailer config as
	// ChrootBaseDir. It is not itself the directory Firecracker sees as /.
	JailerBaseDir string

	// JailerDir is the per-VM jailer directory under JailerBaseDir.
	JailerDir string

	// JailerRootDir is the final host path that becomes Firecracker's / after the
	// jailer performs chroot.
	JailerRootDir string

	// RootFSPath is the copied per-VM root disk that we prepare before launch.
	// It lives in the outer workspace. Launcher code passes this host path to the
	// SDK, and the naive chroot strategy makes it visible inside the jail.
	RootFSPath string
}

// newWorkspace calculates paths for one microVM without creating them.
//
// The jailer path shape is fixed by the SDK:
//
//	<jailerStateDir>/<firecracker-binary-name>/<microvm-id>/root
func newWorkspace(rootDir, jailerStateDir, microvmID, firecrackerBinPath string) Workspace {
	vmRoot := filepath.Join(rootDir, microvmID)
	firecrackerBinName := filepath.Base(firecrackerBinPath)

	jailerBaseDir := jailerStateDir
	jailerDir := filepath.Join(jailerBaseDir, firecrackerBinName, microvmID)
	jailerRootDir := filepath.Join(jailerDir, "root")

	return Workspace{
		MicroVMID:     microvmID,
		RootDir:       vmRoot,
		JailerBaseDir: jailerBaseDir,
		JailerDir:     jailerDir,
		JailerRootDir: jailerRootDir,
		RootFSPath:    filepath.Join(vmRoot, "rootfs.ext4"),
	}
}

// FirecrackerSocketHostPath is the host-visible API socket path inside the jail
// root. scaled uses this path from outside the jail.
func (w Workspace) FirecrackerSocketHostPath() string {
	return filepath.Join(w.JailerRootDir, firecrackerSocketName)
}

// FirecrackerSocketPathInJail is the API socket path Firecracker sees after
// chroot. This is the value launcher code should pass to the SDK config.
func (w Workspace) FirecrackerSocketPathInJail() string {
	return firecrackerSocketName
}

// VSockHostPath is the host-visible vsock base path used for Firecracker's
// guest-initiated host listeners.
func (w Workspace) VSockHostPath() string {
	return filepath.Join(w.JailerRootDir, vsockSocketName)
}

// VSockPathInJail is the vsock base path Firecracker sees after chroot. This is
// the value launcher code should pass to Firecracker's vsock device config.
func (w Workspace) VSockPathInJail() string {
	return vsockSocketName
}

// FirecrackerLogHostPath is the host-visible Firecracker log path inside the
// jail root.
func (w Workspace) FirecrackerLogHostPath() string {
	return filepath.Join(w.JailerRootDir, firecrackerLogName)
}

// FirecrackerLogPathInJail is the log path Firecracker sees after chroot.
func (w Workspace) FirecrackerLogPathInJail() string {
	return firecrackerLogName
}

// Prepare creates the directories that must exist before launch.
//
// The outer workspace holds the copied rootfs and host-managed files. The jail
// root holds Firecracker-visible runtime paths after chroot.
func (w Workspace) Prepare() error {
	if err := os.MkdirAll(w.RootDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(w.JailerRootDir, 0o755)
}

// prepareRootFS copies the platform-managed scoutd rootfs into the workspace.
func prepareRootFS(templatePath, targetPath string) error {
	if err := copyFile(templatePath, targetPath); err != nil {
		return fmt.Errorf("copy rootfs template: %w", err)
	}
	return nil
}

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

// Cleanup removes the full UUID workspace and this VM's jailer directory.
func (w Workspace) Cleanup() error {
	var errs []error

	if w.RootDir != "" {
		errs = append(errs, os.RemoveAll(w.RootDir))
	}
	if w.JailerDir != "" {
		errs = append(errs, os.RemoveAll(w.JailerDir))
	}

	return errors.Join(errs...)
}

// CleanupStaleState removes VM state left behind by a previous scaled process.
//
// scaled does not yet reattach to Firecracker processes after restart, so local
// microVM state from an old process is invalid. This runs before launch
// subscriptions are registered, keeping stale workspaces and jailer directories
// from blocking the next launch attempt.
func CleanupStaleState() error {
	return cleanupStaleState(microVMStateDir, microVMJailerStateDir)
}

func cleanupStaleState(rootDir, jailerStateDir string) error {
	var errs []error
	errs = append(errs, os.RemoveAll(rootDir))
	errs = append(errs, os.RemoveAll(jailerStateDir))
	return errors.Join(errs...)
}
