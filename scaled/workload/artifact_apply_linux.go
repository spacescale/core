//go:build linux

// Package workload prepares host-side workload artifacts for Firecracker guests.
// This file holds the host-side mechanics for turning cached OCI layers into a
// final read-only EROFS workload image.
//
// The orchestration file, artifact_linux.go, decides what work is needed for an
// image digest and workspace scope. This file performs the lower-level work for
// that plan:
//
//  1. download any missing compressed layer blobs into the local cache
//  2. reopen cached blobs as tar streams with the right decompressor
//  3. replay each layer into a merged rootfs tree using OCI whiteout semantics
//  4. build the final EROFS image from the merged tree with mkfs.erofs
//  5. atomically publish cached blobs and the final image so partial work never
//     looks complete to later launches
//
// The file is intentionally separate from the orchestration code so the layer
// replay and image-building details stay isolated from cache planning and path
// layout code.
package workload

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

// cacheMissingLayers ensures every ordered layer blob is present in the chosen
// raw layer cache before merge/apply begins.
func cacheMissingLayers(ctx context.Context, layout artifactCacheLayout, layers []resolvedLayer) error {
	for _, layer := range layers {
		layerPath, err := layout.layerBlobPath(layer.Digest)
		if err != nil {
			return err
		}

		// Skip any layer whose compressed blob is already cached locally. This is the
		// cross-image reuse point that saves us from re-downloading common base layers.
		exists, err := fileExists(layerPath)
		if err != nil {
			return fmt.Errorf("stat layer cache for %s: %w", layer.Digest, err)
		}
		if exists {
			continue
		}

		if err := cacheOneLayer(ctx, layer, layerPath); err != nil {
			return err
		}
	}

	return nil
}

// cacheOneLayer downloads one compressed layer blob into the selected raw layer cache.
//
// This write uses a temp file plus atomic publish so interrupted downloads do not
// leave behind a partially valid cache entry.
func cacheOneLayer(_ context.Context, layer resolvedLayer, finalPath string) error {
	if layer.source == nil {
		return fmt.Errorf("layer %s has no source handle", layer.Digest)
	}

	// We cache the compressed registry blob as-is. Later steps reopen and
	// decompress it only when they need to replay the tar stream into a rootfs.
	rc, err := layer.source.Compressed()
	if err != nil {
		return fmt.Errorf("open compressed layer %s: %w", layer.Digest, err)
	}
	defer func() { _ = rc.Close() }()

	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("create layer cache dir for %s: %w", layer.Digest, err)
	}

	// Write through a temp file first so a killed process or failed copy never
	// leaves behind a path that looks like a complete cached layer.
	tmpFile, err := os.CreateTemp(filepath.Dir(finalPath), "blob-*")
	if err != nil {
		return fmt.Errorf("create temp layer file for %s: %w", layer.Digest, err)
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, rc); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cache compressed layer %s: %w", layer.Digest, err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp layer file for %s: %w", layer.Digest, err)
	}

	if err := publishImmutableFile(tmpPath, finalPath); err != nil {
		return fmt.Errorf("publish cached layer %s: %w", layer.Digest, err)
	}

	return nil
}

// applyLayersFromCache replays the ordered cached layer blobs into one merged
// root filesystem tree.
func applyLayersFromCache(layout artifactCacheLayout, layers []resolvedLayer, rootfsDir string) error {
	for _, layer := range layers {
		layerPath, err := layout.layerBlobPath(layer.Digest)
		if err != nil {
			return err
		}

		// Reopen the cached blob and decode it back into a tar stream so the layer
		// can be replayed onto the merged rootfs.
		rc, err := openCachedLayer(layerPath, layer.MediaType)
		if err != nil {
			return fmt.Errorf("open cached layer %s: %w", layer.Digest, err)
		}

		if err := applyLayerTar(rootfsDir, rc); err != nil {
			_ = rc.Close()
			return fmt.Errorf("apply layer %s: %w", layer.Digest, err)
		}

		if err := rc.Close(); err != nil {
			return fmt.Errorf("close cached layer %s: %w", layer.Digest, err)
		}
	}

	return nil
}

// applyLayerTar unpacks one layer tar stream into the merged rootfs while
// preserving OCI replacement and whiteout semantics.
func applyLayerTar(rootfsDir string, reader io.Reader) error {
	tr := tar.NewReader(reader)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		rel, err := normalizeTarPath(hdr.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}

		// Whiteouts are layer-local delete markers. They do not materialize into the
		// final filesystem as files; instead they remove or hide lower-layer content.
		handled, err := applyWhiteout(rootfsDir, rel)
		if err != nil {
			return err
		}
		if handled {
			continue
		}

		// Every non-whiteout tar entry becomes one path inside the merged rootfs.
		// The Typeflag decides whether that path is a directory, regular file,
		// symlink, hardlink, or metadata-only header.
		targetPath := filepath.Join(rootfsDir, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureDirAt(targetPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("create dir %q: %w", rel, err)
			}
			applyOwnership(targetPath, hdr)
		case tar.TypeReg, typeRegLegacy:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir for %q: %w", rel, err)
			}
			if err := removePathForReplace(targetPath); err != nil {
				return fmt.Errorf("remove existing path for %q: %w", rel, err)
			}
			if err := writeRegularFile(targetPath, tr, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("write file %q: %w", rel, err)
			}
			applyOwnership(targetPath, hdr)
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir for symlink %q: %w", rel, err)
			}
			if err := removePathForReplace(targetPath); err != nil {
				return fmt.Errorf("remove existing path for symlink %q: %w", rel, err)
			}
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
				return fmt.Errorf("create symlink %q -> %q: %w", rel, hdr.Linkname, err)
			}
			applyOwnership(targetPath, hdr)
		case tar.TypeLink:
			linkRel, err := normalizeTarPath(hdr.Linkname)
			if err != nil {
				return err
			}
			linkPath := filepath.Join(rootfsDir, filepath.FromSlash(linkRel))
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir for hardlink %q: %w", rel, err)
			}
			if err := removePathForReplace(targetPath); err != nil {
				return fmt.Errorf("remove existing path for hardlink %q: %w", rel, err)
			}
			if err := os.Link(linkPath, targetPath); err != nil {
				return fmt.Errorf("create hardlink %q -> %q: %w", rel, hdr.Linkname, err)
			}
			applyOwnership(targetPath, hdr)
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		default:
			return fmt.Errorf("unsupported tar entry type %d for %q", hdr.Typeflag, rel)
		}
	}
}

// applyWhiteout handles the OCI delete markers that upper layers use to hide or
// remove filesystem content from lower layers.
func applyWhiteout(rootfsDir, rel string) (bool, error) {
	dirRel, base := path.Split(rel)
	if base == ".wh..wh..opq" {
		dirPath := filepath.Join(rootfsDir, filepath.FromSlash(strings.TrimSuffix(dirRel, "/")))
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return false, fmt.Errorf("prepare opaque whiteout dir %q: %w", dirRel, err)
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return false, fmt.Errorf("read opaque whiteout dir %q: %w", dirRel, err)
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(dirPath, entry.Name())); err != nil {
				return false, fmt.Errorf("remove opaque whiteout entry %q: %w", entry.Name(), err)
			}
		}

		return true, nil
	}

	if !strings.HasPrefix(base, ".wh.") {
		return false, nil
	}

	targetName := strings.TrimPrefix(base, ".wh.")
	targetPath := filepath.Join(rootfsDir, filepath.FromSlash(strings.TrimSuffix(dirRel, "/")), targetName)
	if err := os.RemoveAll(targetPath); err != nil {
		return false, fmt.Errorf("apply whiteout for %q: %w", rel, err)
	}

	return true, nil
}

// openCachedLayer opens one cached layer blob and wraps it with the appropriate
// decompressor so later code can treat all layers as tar streams.
func openCachedLayer(path string, mediaType types.MediaType) (_ io.ReadCloser, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
		}
	}()

	//exhaustive:ignore Non-layer media types are rejected below.
	switch mediaType {
	case types.OCILayer, types.DockerLayer:
		// Both OCILayer (application/vnd.oci.image.layer.v1.tar+gzip) and
		// DockerLayer (application/vnd.docker.image.rootfs.diff.tar.gzip) are
		// gzip-compressed tars. Decompress before handing to the tar reader.
		return openGzipLayer(file)
	case types.OCILayerZStd:
		return openZstdLayer(file)
	default:
		if strings.Contains(string(mediaType), "gzip") {
			return openGzipLayer(file)
		}
		if strings.Contains(string(mediaType), "zstd") {
			return openZstdLayer(file)
		}

		return nil, fmt.Errorf("unsupported layer media type %q", mediaType)
	}
}

// openGzipLayer wraps one cached gzip-compressed layer blob in a reader that
// closes both the decompressor and the underlying file together.
func openGzipLayer(file *os.File) (io.ReadCloser, error) {
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}

	return multiCloser{
		Reader:  gzr,
		closers: []io.Closer{gzr, file},
	}, nil
}

// openZstdLayer wraps one cached zstd-compressed layer blob in a reader that
// closes both the decompressor and the underlying file together.
func openZstdLayer(file *os.File) (io.ReadCloser, error) {
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return nil, err
	}

	return multiCloser{
		Reader:  decoder,
		closers: []io.Closer{decoder.IOReadCloser(), file},
	}, nil
}

// buildEROFS turns the merged rootfs directory tree into the final immutable
// workload image.
func buildEROFS(ctx context.Context, rootfsDir, outputPath string) error {
	if _, err := exec.LookPath("mkfs.erofs"); err != nil {
		return fmt.Errorf("mkfs.erofs not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, "mkfs.erofs", "--all-root", "-T", "0", "-z", "lz4", outputPath, rootfsDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("build erofs image: %w: %s", err, msg)
		}
		return fmt.Errorf("build erofs image: %w", err)
	}

	return nil
}

// publishImmutableFile atomically moves a freshly built file into its final
// cache location and treats pre-existing final files as cache hits.
func publishImmutableFile(tmpPath, finalPath string) error {
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("create final parent dir: %w", err)
	}

	exists, err := fileExists(finalPath)
	if err != nil {
		return fmt.Errorf("stat final file: %w", err)
	}
	if exists {
		if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove temp file after cache hit: %w", err)
		}
		return nil
	}

	if err := os.Chmod(tmpPath, 0o444); err != nil {
		return fmt.Errorf("chmod immutable file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("publish immutable file: %w", err)
	}

	return nil
}

// ensureDirAt guarantees the target path ends up as a directory with the
// requested mode, replacing non-directory entries if necessary.
func ensureDirAt(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.IsDir():
		return os.Chmod(path, mode)
	case err == nil:
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return err
	}

	return os.MkdirAll(path, mode)
}

// removePathForReplace removes any existing filesystem entry at the target path
// before a later layer replaces it with new content.
func removePathForReplace(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	return os.RemoveAll(path)
}

// writeRegularFile streams one tar file entry into the merged rootfs.
func writeRegularFile(path string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, reader); err != nil {
		return err
	}

	return file.Close()
}

// applyOwnership best-effort applies tar header ownership information.
func applyOwnership(path string, hdr *tar.Header) {
	if hdr == nil {
		return
	}

	if err := os.Lchown(path, hdr.Uid, hdr.Gid); err != nil {
		return
	}
}

// normalizeTarPath turns one tar entry path into a safe rootfs-relative path
// and rejects traversal outside the merged rootfs.
func normalizeTarPath(name string) (string, error) {
	clean := path.Clean("/" + name)
	if clean == "/" {
		return "", nil
	}

	rel := strings.TrimPrefix(clean, "/")
	if rel == "" || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", fmt.Errorf("invalid tar path %q", name)
	}

	return rel, nil
}

// multiCloser lets one decompressed layer stream close both the decoder and the
// underlying file with one Close call.
type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m multiCloser) Close() error {
	var errs []error
	for _, closer := range m.closers {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
