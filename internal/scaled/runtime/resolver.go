// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// Package runtime reconciles fixed host and guest files before scaled accepts
// work.
//
// The package owns the runtime asset cache for Firecracker, jailer, guest kernel,
// and scoutd rootfs. Startup calls it once, receives concrete paths, and passes
// those paths downstream instead of letting launch code fetch or validate assets.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	runtimeBucketBaseURL = "https://spacescale-runtime-assets.s3.eu-west-par.io.cloud.ovh.net"
	runtimeStateDir      = "/var/lib/spacescale/runtime"

	firecrackerObjectKey = "firecracker-v1.15.1-x86_64"
	jailerObjectKey      = "jailer-v1.15.1-x86_64"
	kernelObjectKey      = "vmlinux-v6.1.80-x86_64"
	rootfsObjectKey      = "scoutd-rootfs-v0.1.3-x86_64-ext4"
)

// Paths holds the concrete local runtime files prepared during startup.
type Paths struct {
	RootDir         string
	FirecrackerPath string
	JailerPath      string
	KernelPath      string
	RootFSPath      string
}

type Resolver struct {
	logger  *slog.Logger
	client  *http.Client
	baseURL string
	rootDir string
}

// NewResolver creates the startup helper that prepares runtime assets before
// scaled joins bootstrap readiness or workload handling.
func NewResolver(logger *slog.Logger) *Resolver {
	return &Resolver{
		logger:  logger.With("subsystem", "runtime"),
		client:  &http.Client{Timeout: 2 * time.Minute},
		baseURL: runtimeBucketBaseURL,
		rootDir: runtimeStateDir,
	}
}

// Reconcile makes sure every required runtime file is present locally.
func (r *Resolver) Reconcile(ctx context.Context) (Paths, error) {
	paths := currentPaths(r.rootDir)

	if err := os.MkdirAll(r.rootDir, 0o755); err != nil {
		return Paths{}, fmt.Errorf("create runtime root dir: %w", err)
	}

	if err := r.ensureAsset(ctx, "firecracker", firecrackerObjectKey, paths.FirecrackerPath, true); err != nil {
		return Paths{}, err
	}
	if err := r.ensureAsset(ctx, "jailer", jailerObjectKey, paths.JailerPath, true); err != nil {
		return Paths{}, err
	}
	if err := r.ensureAsset(ctx, "kernel", kernelObjectKey, paths.KernelPath, false); err != nil {
		return Paths{}, err
	}
	if err := r.ensureAsset(ctx, "rootfs", rootfsObjectKey, paths.RootFSPath, false); err != nil {
		return Paths{}, err
	}

	return paths, nil
}

func currentPaths(root string) Paths {
	return Paths{
		RootDir:         root,
		FirecrackerPath: filepath.Join(root, "host", firecrackerObjectKey),
		JailerPath:      filepath.Join(root, "host", jailerObjectKey),
		KernelPath:      filepath.Join(root, "guest", kernelObjectKey),
		RootFSPath:      filepath.Join(root, "guest", rootfsObjectKey),
	}
}

// ensureAsset reuses a valid cached file or downloads a fresh copy.
func (r *Resolver) ensureAsset(ctx context.Context, name, objectKey, localPath string, executable bool) error {
	ready, err := validateLocalAsset(localPath, executable)
	switch {
	case err != nil:
		return fmt.Errorf("runtime asset %s cached state invalid: %w", name, err)
	case ready:
		r.logger.Info("runtime asset ready",
			"asset", name,
			"path", localPath,
			"source", "cache",
		)
		return nil
	}

	if err := r.downloadAsset(ctx, objectKey, localPath, executable); err != nil {
		return fmt.Errorf("download runtime asset %s: %w", name, err)
	}

	ready, err = validateLocalAsset(localPath, executable)
	if err != nil {
		return fmt.Errorf("runtime asset %s download validation failed: %w", name, err)
	}
	if !ready {
		return fmt.Errorf("runtime asset %s missing after download", name)
	}

	r.logger.Info("runtime asset ready",
		"asset", name,
		"path", localPath,
		"source", "download",
	)

	return nil
}

// downloadAsset stages one runtime file atomically in the local cache.
func (r *Resolver) downloadAsset(ctx context.Context, objectKey, localPath string, executable bool) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("create runtime asset dir: %w", err)
	}

	tmpPath := localPath + ".tmp"
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	url := r.assetURL(objectKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: unexpected status %s", url, resp.Status)
	}

	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open temp file: %w", err)
	}

	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if executable {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return fmt.Errorf("mark executable: %w", err)
		}
	}

	if err := os.Rename(tmpPath, localPath); err != nil {
		return fmt.Errorf("replace runtime asset: %w", err)
	}

	cleanupTmp = false
	return nil
}

func (r *Resolver) assetURL(objectKey string) string {
	return r.baseURL + "/" + objectKey
}

// validateLocalAsset answers whether a cached runtime file is ready for reuse.
func validateLocalAsset(path string, executable bool) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat asset: %w", err)
	}

	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory", path)
	}

	if info.Size() == 0 {
		return false, fmt.Errorf("%s is empty", path)
	}

	if executable && info.Mode()&0o111 == 0 {
		if err := os.Chmod(path, 0o755); err != nil {
			return false, fmt.Errorf("chmod asset: %w", err)
		}
	}

	return true, nil
}
