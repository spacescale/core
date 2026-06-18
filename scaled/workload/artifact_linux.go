//go:build linux

// This file owns orchestration for host-side workload materialization.
//
// It takes a selected OCI image, resolves its ordered layers, decides what can
// be reused from cache, and prepares the final workspace-scoped read-only
// workload image path. The lower-level mechanics of replaying layers, handling
// whiteouts, and building the EROFS image live in artifact_apply_linux.go.
//
// The split is intentional:
//  1. this file keeps image selection, cache scope, and artifact planning
//  2. the apply/build file keeps the tar replay and mkfs.erofs mechanics
//  3. both halves stay in the same package so they can share helpers without
//     exporting internal implementation details
package workload

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	workloadCacheRootDir      = "/var/lib/spacescale/workloads"
	sharedPublicLayersDirName = "sharedOCIPublicLayers"
	workspaceCacheDirName     = "workspaces"
	privateLayersDirName      = "privateOCILayers"
	artifactCacheDirName      = "artifacts"
	artifactWorkDirName       = "tmp"
	artifactFileName          = "artifact.erofs"
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

// materializedArtifact is a handle describing the on-disk read-only EROFS
// image produced by this file. The artifact is a digest-addressed,
// workspace-scoped immutable workload image built from the resolved OCI layers
// and ready for later Firecracker attachment; this struct carries only its
// path, source image digest, and ordered layer digests, not the file's bytes.
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
//	        ├── artifacts/                          <- ArtifactsDir          (final read-only workload images)
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
		TmpDir:                filepath.Join(workspaceRoot, artifactWorkDirName),
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
//  7. builds the final immutable read-only workload image in scratch space
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
	// OCI layer in order. The final read-only workload image is built from this
	// directory later.
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
	if err := buildEROFS(ctx, rootfsDir, tmpArtifactPath); err != nil {
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
