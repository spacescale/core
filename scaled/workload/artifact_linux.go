//go:build linux

// Package workload owns the host-side preparation work that happens before a
// Firecracker guest boots a customer workload.
//
// For issue #38, that means turning an OCI image reference into a reusable,
// immutable workload artifact on the node itself. The important concepts in this
// file are:
//
//  1. Image digest: the exact identity of the full OCI image we resolved.
//     This is the cache key for a final immutable artifact because the fully
//     merged filesystem should be identical for the same image digest.
//  2. Layer digest: the identity of one compressed OCI layer blob. Different
//     images can share lower layers, so these are cached independently to avoid
//     repeated downloads of common public bases such as Ubuntu.
//  3. Materialization: the process of taking the ordered OCI layers, applying
//     them with whiteout semantics, and producing one immutable artifact file.
//  4. Cache scope: layer reuse and final artifact reuse are not identical. This
//     file keeps public OCI layers in one node-global cache, but keeps final
//     immutable artifacts scoped to one workspace so a customer mistake such as
//     baking secrets into an image does not become cross-workspace reuse by
//     default.
//
// This file intentionally stops before guest attachment. It prepares and caches
// the workload payload on the host; issue #36 will attach that payload to a
// Firecracker guest.
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

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

const (
	workloadCacheRootDir      = "/var/lib/spacescale/workloads"
	sharedPublicLayersDirName = "sharedOCIPublicLayers"
	workspaceCacheDirName     = "workspaces"
	privateLayersDirName      = "privateOCILayers"
	artifactCacheDirName      = "artifacts"
	buildScratchDirName       = "tmp"
	artifactFileName          = "artifact.squashfs"
	typeRegLegacy             = byte(0)
)

type imageSourceVisibility string

const (
	imageSourcePublic  imageSourceVisibility = "public"
	imageSourcePrivate imageSourceVisibility = "private"
)

// artifactCacheScope describes how one workload image should be isolated on the
// current node.
//
// WorkspaceID is always required because final immutable artifacts are scoped to
// the workspace that asked for them. SourceVisibility controls only the raw OCI
// layer cache behavior:
//  1. public images may reuse a shared node-global layer cache
//  2. private images keep layer blobs under the requesting workspace scope
type artifactCacheScope struct {
	WorkspaceID      string
	SourceVisibility imageSourceVisibility
}

// artifactCacheLayout collects the stable node-local directories used while
// materializing and caching workload artifacts for one workspace/image scope.
//
// The split is deliberate:
//  1. public OCI layers live in one shared node-global cache
//  2. private OCI layers live in a workspace-scoped cache
//  3. final immutable artifacts are always workspace-scoped
//  4. scratch space stays temporary so failed or interrupted builds do not pollute
//     the persistent cache roots
type artifactCacheLayout struct {
	RootDir               string
	WorkspaceID           string
	SourceVisibility      imageSourceVisibility
	SharedPublicLayersDir string
	PrivateLayersDir      string
	ArtifactsDir          string
	TmpDir                string
}

// resolvedImageLayers represents the ordered layer information for one selected
// OCI image after platform resolution has already happened.
//
// This shape exists so later cache planning and artifact materialization can use
// one deterministic view of the image without repeating registry lookups.
type resolvedImageLayers struct {
	ImageRef    string
	ImageDigest string
	Layers      []resolvedLayer
}

// resolvedLayer describes one OCI layer in the exact order it should be applied
// to construct the merged root filesystem.
//
// Digest identifies the compressed blob used for raw cache reuse. DiffID is the
// uncompressed identity and is useful for debugging and future validation work.
// The source handle is kept privately so this file can cache the layer blob
// without exposing registry types to the rest of the package.
type resolvedLayer struct {
	Digest    string
	DiffID    string
	Size      int64
	MediaType types.MediaType

	source gcrv1.Layer
}

// materializationPlan is the host-side answer to "what work is still left to do
// for this image digest?"
//
// It separates two reuse levels:
//  1. whether the final immutable artifact already exists for this workspace
//  2. which raw layers still need to be downloaded into the chosen layer cache
type materializationPlan struct {
	ImageDigest         string
	ArtifactPath        string
	ArtifactExists      bool
	Layers              []resolvedLayer
	MissingLayerDigests []string
}

// materializedArtifact is a handle describing the on-disk SquashFS artifact
// produced by this file. The artifact is a digest-addressed, workspace-scoped,
// immutable rootfs built from the resolved OCI layers and ready for later
// Firecracker attachment; this struct carries only its path, source image
// digest, and ordered layer digests — not the file's bytes.
type materializedArtifact struct {
	ImageDigest  string
	ArtifactPath string
	LayerDigests []string
}

// artifactCacheLayout collects the stable node-local directories used while
// materializing and caching workload artifacts for one workspace/image scope.
//
// On-disk layout (under RootDir):
//
//	/var/lib/spacescale/workloads/                  <- RootDir
//	├── sharedOCIPublicLayers/                      <- SharedPublicLayersDir (node-global, public layers)
//	└── workspaces/
//	    └── <WorkspaceID>/                          <- workspaceRoot
//	        ├── privateOCILayers/                   <- PrivateLayersDir      (this workspace's private layers)
//	        ├── artifacts/                          <- ArtifactsDir          (final SquashFS files)
//	        └── tmp/                                <- TmpDir                (scratch space for builds)
//
// The split is deliberate:
//  1. public OCI layers live in one shared node-global cache
//  2. private OCI layers live in a workspace-scoped cache
//  3. final immutable artifacts are always workspace-scoped
//  4. scratch space stays temporary so failed or interrupted builds do not pollute
//     the persistent cache roots
func newArtifactCacheLayout(scope artifactCacheScope) (artifactCacheLayout, error) {
	if strings.TrimSpace(scope.WorkspaceID) == "" {
		return artifactCacheLayout{}, errors.New("workspace id is required for artifact cache scope")
	}
	if scope.SourceVisibility != imageSourcePublic && scope.SourceVisibility != imageSourcePrivate {
		return artifactCacheLayout{}, fmt.Errorf("unsupported image source visibility %q", scope.SourceVisibility)
	}

	root := workloadCacheRootDir
	workspaceRoot := filepath.Join(root, workspaceCacheDirName, scope.WorkspaceID)

	return artifactCacheLayout{
		RootDir:               root,
		WorkspaceID:           scope.WorkspaceID,
		SourceVisibility:      scope.SourceVisibility,
		SharedPublicLayersDir: filepath.Join(root, sharedPublicLayersDirName),
		PrivateLayersDir:      filepath.Join(workspaceRoot, privateLayersDirName),
		ArtifactsDir:          filepath.Join(workspaceRoot, artifactCacheDirName),
		TmpDir:                filepath.Join(workspaceRoot, buildScratchDirName),
	}, nil
}

// layerCacheRoot returns the raw OCI layer cache root that matches the chosen
// image visibility.
func (l artifactCacheLayout) layerCacheRoot() string {
	if l.SourceVisibility == imageSourcePublic {
		return l.SharedPublicLayersDir
	}

	return l.PrivateLayersDir
}

// layerBlobPath returns the absolute on-disk path of the cached compressed
// tarball for one OCI layer. Visibility selects the root via layerCacheRoot;
// the layer digest selects the sharded subpath via digestPath; the leaf
// filename is "blob". Pure path computation — no filesystem access happens here.
//
// Example return value for a public layer with digest sha256:abc123:
//
//	/var/lib/spacescale/workloads/sharedOCIPublicLayers/sha256/abc123/blob
//
// For a private layer under workspace ws-7, the same digest resolves to:
//
//	/var/lib/spacescale/workloads/workspaces/ws-7/privateOCILayers/sha256/abc123/blob
func (l artifactCacheLayout) layerBlobPath(layerDigest string) (string, error) {
	return digestPath(l.layerCacheRoot(), layerDigest, "blob")
}

// artifactPath returns the final immutable artifact path for one full image digest.
func (l artifactCacheLayout) artifactPath(imageDigest string) (string, error) {
	return digestPath(l.ArtifactsDir, imageDigest, artifactFileName)
}

// ensureBaseDirs creates the persistent cache roots before any build work starts.
func (l artifactCacheLayout) ensureBaseDirs() error {
	for _, dir := range []string{l.layerCacheRoot(), l.ArtifactsDir, l.TmpDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create cache dir %q: %w", dir, err)
		}
	}

	return nil
}

// materializeArtifact is the default entry point used by callers that only know
// the image reference and cache scope.
func materializeArtifact(ctx context.Context, scope artifactCacheScope, imageRef string) (materializedArtifact, error) {
	layout, err := newArtifactCacheLayout(scope)
	if err != nil {
		return materializedArtifact{}, err
	}

	return materializeArtifactWithLayout(ctx, layout, imageRef)
}

// materializeArtifactWithLayout orchestrates the full host-side build flow.
//
// Step by step it:
//  1. ensures the scoped cache roots exist
//  2. resolves the selected image and its ordered layers
//  3. plans cache hits and misses for both layers and the final artifact
//  4. returns immediately if the final artifact already exists
//  5. caches any missing raw layer blobs
//  6. applies the ordered layers into a temporary merged rootfs
//  7. builds the final immutable SquashFS artifact in scratch space
//  8. atomically publishes that artifact into the persistent workspace cache
func materializeArtifactWithLayout(ctx context.Context, layout artifactCacheLayout, imageRef string) (materializedArtifact, error) {
	if err := layout.ensureBaseDirs(); err != nil {
		return materializedArtifact{}, err
	}

	resolved, err := resolveImageLayers(ctx, imageRef)
	if err != nil {
		return materializedArtifact{}, err
	}

	plan, err := planMaterialization(layout, resolved)
	if err != nil {
		return materializedArtifact{}, err
	}

	if plan.ArtifactExists {
		return materializedArtifact{
			ImageDigest:  resolved.ImageDigest,
			ArtifactPath: plan.ArtifactPath,
			LayerDigests: collectLayerDigests(resolved.Layers),
		}, nil
	}

	if err := cacheMissingLayers(ctx, layout, resolved.Layers); err != nil {
		return materializedArtifact{}, err
	}

	// Build in a temporary directory first so partially merged root filesystems or
	// partially written artifact files never appear as valid cache entries.
	buildDir, err := os.MkdirTemp(layout.TmpDir, sanitizedDigestPrefix(resolved.ImageDigest)+"-")
	if err != nil {
		return materializedArtifact{}, fmt.Errorf("create build scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()

	// The merged rootfs is the plain directory tree produced after replaying every
	// OCI layer in order. SquashFS is built from this directory later.
	rootfsDir := filepath.Join(buildDir, "rootfs")
	if err := os.MkdirAll(rootfsDir, 0o755); err != nil {
		return materializedArtifact{}, fmt.Errorf("create merged rootfs dir: %w", err)
	}

	if err := applyLayersFromCache(layout, resolved.Layers, rootfsDir); err != nil {
		return materializedArtifact{}, err
	}

	// The final immutable artifact is built in scratch space first, then atomically
	// published into the cache so other launches only ever observe complete files.
	tmpArtifactPath := filepath.Join(buildDir, artifactFileName)
	if err := buildSquashFS(ctx, rootfsDir, tmpArtifactPath); err != nil {
		return materializedArtifact{}, err
	}

	if err := publishImmutableFile(tmpArtifactPath, plan.ArtifactPath); err != nil {
		return materializedArtifact{}, err
	}

	return materializedArtifact{
		ImageDigest:  resolved.ImageDigest,
		ArtifactPath: plan.ArtifactPath,
		LayerDigests: collectLayerDigests(resolved.Layers),
	}, nil
}

// resolveImageLayers resolves the concrete image selected for this image
// reference and returns the ordered layer metadata needed for caching and
// materialization.
//
// This intentionally reuses the shared image-selection helper from the OCI
// config resolver so both launch metadata and artifact materialization agree on
// the exact image digest and platform choice.
func resolveImageLayers(ctx context.Context, imageRef string) (resolvedImageLayers, error) {
	selected, err := resolveSelectedOCIImage(ctx, imageRef)
	if err != nil {
		return resolvedImageLayers{}, err
	}

	// Layers returns the ordered OCI filesystem deltas. We preserve that order
	// because later layer application must replay them from base to top.
	layers, err := selected.image.Layers()
	if err != nil {
		return resolvedImageLayers{}, fmt.Errorf("read image layers: %w", err)
	}

	resolvedLayers := make([]resolvedLayer, 0, len(layers))
	for _, layer := range layers {
		// Digest identifies the compressed blob exactly as stored in the registry.
		// This is the key used for the raw layer cache because many different images
		// can share the same lower compressed layer bytes.
		digest, err := layer.Digest()
		if err != nil {
			return resolvedImageLayers{}, fmt.Errorf("read layer digest: %w", err)
		}

		// DiffID identifies the uncompressed layer content. We keep it for future
		// validation/debugging even though the cache currently keys off compressed digest.
		diffID, err := layer.DiffID()
		if err != nil {
			return resolvedImageLayers{}, fmt.Errorf("read layer diff id for %s: %w", digest.String(), err)
		}

		size, err := layer.Size()
		if err != nil {
			return resolvedImageLayers{}, fmt.Errorf("read layer size for %s: %w", digest.String(), err)
		}

		mediaType, err := layer.MediaType()
		if err != nil {
			return resolvedImageLayers{}, fmt.Errorf("read layer media type for %s: %w", digest.String(), err)
		}

		resolvedLayers = append(resolvedLayers, resolvedLayer{
			Digest:    digest.String(),
			DiffID:    diffID.String(),
			Size:      size,
			MediaType: mediaType,
			source:    layer,
		})
	}

	return resolvedImageLayers{
		ImageRef:    imageRef,
		ImageDigest: selected.imageDigest,
		Layers:      resolvedLayers,
	}, nil
}

// planMaterialization answers what can be reused and what still needs work for
// this image digest inside the current cache scope.
func planMaterialization(layout artifactCacheLayout, resolved resolvedImageLayers) (materializationPlan, error) {
	artifactPath, err := layout.artifactPath(resolved.ImageDigest)
	if err != nil {
		return materializationPlan{}, err
	}

	artifactExists, err := fileExists(artifactPath)
	if err != nil {
		return materializationPlan{}, fmt.Errorf("stat artifact cache: %w", err)
	}

	// Track only the missing raw layer blobs. Existing cached layers can be reused
	// even when the final image digest is new because different images often share
	// common lower layers from the same base image.
	missing := make([]string, 0, len(resolved.Layers))
	for _, layer := range resolved.Layers {
		layerPath, err := layout.layerBlobPath(layer.Digest)
		if err != nil {
			return materializationPlan{}, err
		}

		exists, err := fileExists(layerPath)
		if err != nil {
			return materializationPlan{}, fmt.Errorf("stat layer cache for %s: %w", layer.Digest, err)
		}
		if !exists {
			missing = append(missing, layer.Digest)
		}
	}

	return materializationPlan{
		ImageDigest:         resolved.ImageDigest,
		ArtifactPath:        artifactPath,
		ArtifactExists:      artifactExists,
		Layers:              resolved.Layers,
		MissingLayerDigests: missing,
	}, nil
}

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
			// Directories may already exist from lower layers. ensureDirAt keeps the
			// path a directory and updates mode metadata if needed.
			if err := ensureDirAt(targetPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("create dir %q: %w", rel, err)
			}
			if err := applyOwnership(targetPath, hdr); err != nil {
				return fmt.Errorf("apply dir ownership %q: %w", rel, err)
			}
		case tar.TypeReg, typeRegLegacy:
			// Regular files replace whatever lower layer previously put at this path,
			// so we clear the old entry before streaming the new bytes in.
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir for %q: %w", rel, err)
			}
			if err := removePathForReplace(targetPath); err != nil {
				return fmt.Errorf("remove existing path for %q: %w", rel, err)
			}
			if err := writeRegularFile(targetPath, tr, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("write file %q: %w", rel, err)
			}
			if err := applyOwnership(targetPath, hdr); err != nil {
				return fmt.Errorf("apply file ownership %q: %w", rel, err)
			}
		case tar.TypeSymlink:
			// Symlinks behave like other replacement entries: remove the lower-layer
			// entry first, then create the new link target recorded in this layer.
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir for symlink %q: %w", rel, err)
			}
			if err := removePathForReplace(targetPath); err != nil {
				return fmt.Errorf("remove existing path for symlink %q: %w", rel, err)
			}
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
				return fmt.Errorf("create symlink %q -> %q: %w", rel, hdr.Linkname, err)
			}
			if err := applyOwnership(targetPath, hdr); err != nil {
				return fmt.Errorf("apply symlink ownership %q: %w", rel, err)
			}
		case tar.TypeLink:
			// Hardlinks point at another path inside the merged rootfs, so we first
			// normalize the source path and then link the target path to it.
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
			if err := applyOwnership(targetPath, hdr); err != nil {
				return fmt.Errorf("apply hardlink ownership %q: %w", rel, err)
			}
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			// Extended tar headers carry metadata about following entries but do not
			// become standalone filesystem paths in the merged rootfs.
			continue
		default:
			return fmt.Errorf("unsupported tar entry type %d for %q", hdr.Typeflag, rel)
		}
	}
}

// applyWhiteout handles the OCI delete markers that upper layers use to hide or
// remove filesystem content from lower layers.
func applyWhiteout(rootfsDir, rel string) (bool, error) {
	// rel is the normalized path of the current tar entry relative to the merged
	// rootfs. Splitting it gives us the whiteout filename and the parent directory
	// where that whiteout takes effect.
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
		// Opaque whiteouts tell the upper layer to hide every lower-layer entry in
		// this directory before applying the new upper-layer contents.
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

	// Normal whiteouts remove exactly one lower-layer entry with the same parent
	// directory as the whiteout file itself.
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
	// The cache stores raw compressed blobs. This function rehydrates one cache
	// entry into a tar reader by selecting the decompressor that matches the layer
	// media type advertised by the registry.
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
		}
	}()

	switch mediaType {
	case types.OCILayer, types.DockerLayer:
		return file, nil
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
		Reader: gzr,
		closers: []io.Closer{
			gzr,
			file,
		},
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
		Reader: decoder,
		closers: []io.Closer{
			decoder.IOReadCloser(),
			file,
		},
	}, nil
}

// buildSquashFS turns the merged rootfs directory tree into the final immutable
// artifact file using the host's native SquashFS toolchain.
func buildSquashFS(ctx context.Context, rootfsDir, outPath string) error {
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		return fmt.Errorf("mksquashfs not found: %w", err)
	}

	cmd := exec.CommandContext(
		ctx,
		"mksquashfs",
		rootfsDir,
		outPath,
		"-comp", "zstd",
		"-noappend",
		"-no-progress",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("run mksquashfs: %w", err)
		}
		return fmt.Errorf("run mksquashfs: %w: %s", err, trimmed)
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
		// Another build may have already published the same digest while this build
		// was running. In that case the temp file is redundant and can be discarded.
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
//
// This intentionally tolerates ownership failures for now because artifact
// builds may run in environments that cannot fully reproduce image UID/GID
// metadata during development. That policy can be tightened later on real Linux.
func applyOwnership(path string, hdr *tar.Header) error {
	if hdr == nil {
		return nil
	}

	if err := os.Lchown(path, hdr.Uid, hdr.Gid); err != nil {
		return nil
	}

	return nil
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

// fileExists is a small filesystem predicate used throughout cache planning and
// artifact publication.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// digestPath maps a digest string such as sha256:deadbeef into a stable cache
// path under the supplied root directory.
func digestPath(root, digest, leaf string) (string, error) {
	algo, hex, err := splitDigest(digest)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, algo, hex, leaf), nil
}

// splitDigest validates the usual algo:hex digest form and returns both parts.
func splitDigest(digest string) (string, string, error) {
	algo, hex, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || algo == "" || hex == "" {
		return "", "", fmt.Errorf("invalid digest %q", digest)
	}

	return algo, hex, nil
}

// sanitizedDigestPrefix converts a digest into a filesystem-safe temp-dir prefix.
func sanitizedDigestPrefix(digest string) string {
	algo, hex, err := splitDigest(digest)
	if err != nil {
		return "artifact"
	}

	return algo + "-" + hex
}

// collectLayerDigests projects the ordered layer list into plain digest strings.
func collectLayerDigests(layers []resolvedLayer) []string {
	if len(layers) == 0 {
		return nil
	}

	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		out = append(out, layer.Digest)
	}

	return out
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
