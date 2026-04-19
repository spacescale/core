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

type Resolver struct {
	logger  *slog.Logger
	client  *http.Client
	baseURL string
	rootDir string
}

// NewResolver creates the startup helper that makes sure runtime assets exist
// on disk before scaled moves forward into bootstrap readiness and workload
// handling
//
// # The resolver is intentionally simple
//
// It knows one bucket base URL
// It knows one local runtime directory
// It knows the fixed set of assets the daemon expects right now
func NewResolver(logger *slog.Logger) *Resolver {
	if logger == nil {
		logger = slog.Default()
	}

	return &Resolver{
		logger:  logger.With("subsystem", "runtime"),
		client:  &http.Client{Timeout: 2 * time.Minute},
		baseURL: runtimeBucketBaseURL,
		rootDir: runtimeStateDir,
	}
}

// Reconcile makes sure every required runtime file is present locally before
// scaled is allowed to continue startup
//
// # The flow stays intentionally mechanical
//
// Build the fixed local paths
// Ensure the runtime root exists
// Reuse or download firecracker
// Reuse or download jailer
// Reuse or download the kernel
// Reuse or download scoutd
// Return the final local paths
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
	if err := r.ensureAsset(ctx, "scoutd", scoutdObjectKey, paths.ScoutdPath, true); err != nil {
		return Paths{}, err
	}

	return paths, nil
}

// ensureAsset is the cache boundary for one runtime file
//
// If the local file already exists and passes the current local checks we reuse
// it
// Otherwise we download it and validate the local result one more time before
// returning success
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

// downloadAsset fetches one immutable runtime file and stages it atomically in
// the local cache
//
// We always write into a temporary file first and only rename into place after
// the transfer finishes cleanly
//
// That keeps partial downloads from being mistaken for good cached assets on a
// later daemon boot
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

// validateLocalAsset answers whether a cached file is ready for reuse
//
// # The checks stay intentionally small in this issue
//
// The file must exist
// The path must be a file not a directory
// The file must not be empty
// Executable assets must have executable bits locally
//
// We are not doing deep checksum or metadata verification in this slice
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
