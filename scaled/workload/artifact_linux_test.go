//go:build linux

package workload

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	gcrv1types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
)

// TestSplitDigest covers parsing valid digest strings and rejecting malformed
// digest inputs used by cache path helpers.
func TestSplitDigest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantAlg string
		wantHex string
		wantErr string
	}{
		{name: "valid digest", input: "sha256:deadbeef", wantAlg: "sha256", wantHex: "deadbeef"},
		{name: "trimmed digest", input: "  sha256:abc123  ", wantAlg: "sha256", wantHex: "abc123"},
		{name: "missing separator", input: "sha256deadbeef", wantErr: `invalid digest "sha256deadbeef"`},
		{name: "empty algorithm", input: ":deadbeef", wantErr: `invalid digest ":deadbeef"`},
		{name: "empty hex", input: "sha256:", wantErr: `invalid digest "sha256:"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAlg, gotHex, err := splitDigest(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantAlg, gotAlg)
			require.Equal(t, tt.wantHex, gotHex)
		})
	}
}

// TestDigestPath covers digest sharding for both layer blobs and final image
// paths.
func TestDigestPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		root    string
		digest  string
		leaf    string
		want    string
		wantErr string
	}{
		{name: "layer blob path", root: "/cache", digest: "sha256:deadbeef", leaf: "blob", want: "/cache/sha256/deadbeef/blob"},
		{name: "artifact path", root: "/cache", digest: "sha256:deadbeef", leaf: "artifact.erofs", want: "/cache/sha256/deadbeef/artifact.erofs"},
		{name: "rejects malformed digest", root: "/cache", digest: "not-a-digest", leaf: "blob", wantErr: `invalid digest "not-a-digest"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := digestPath(tt.root, tt.digest, tt.leaf)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestSanitizedDigestPrefix covers the temp-dir prefix used while building the
// final immutable artifact.
func TestSanitizedDigestPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid digest", input: "sha256:deadbeef", want: "sha256-deadbeef"},
		{name: "invalid digest falls back", input: "not-a-digest", want: "artifact"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, sanitizedDigestPrefix(tt.input))
		})
	}
}

// TestNewArtifactCacheLayout covers cache root selection for public and private
// image scopes and validates the input guards.
func TestNewArtifactCacheLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   artifactCacheScope
		wantErr string
		want    artifactCacheLayout
	}{
		{
			name:  "public scope",
			scope: artifactCacheScope{WorkspaceID: "ws-1", SourceVisibility: imageSourcePublic},
			want: artifactCacheLayout{
				RootDir:               workloadCacheRootDir,
				WorkspaceID:           "ws-1",
				SourceVisibility:      imageSourcePublic,
				SharedPublicLayersDir: filepath.Join(workloadCacheRootDir, sharedPublicLayersDirName),
				PrivateLayersDir:      filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-1", privateLayersDirName),
				ArtifactsDir:          filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-1", artifactCacheDirName),
				TmpDir:                filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-1", artifactWorkDirName),
			},
		},
		{
			name:  "private scope",
			scope: artifactCacheScope{WorkspaceID: "ws-2", SourceVisibility: imageSourcePrivate},
			want: artifactCacheLayout{
				RootDir:               workloadCacheRootDir,
				WorkspaceID:           "ws-2",
				SourceVisibility:      imageSourcePrivate,
				SharedPublicLayersDir: filepath.Join(workloadCacheRootDir, sharedPublicLayersDirName),
				PrivateLayersDir:      filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-2", privateLayersDirName),
				ArtifactsDir:          filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-2", artifactCacheDirName),
				TmpDir:                filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-2", artifactWorkDirName),
			},
		},
		{name: "missing workspace", scope: artifactCacheScope{SourceVisibility: imageSourcePublic}, wantErr: "workspace id is required for artifact cache scope"},
		{name: "invalid visibility", scope: artifactCacheScope{WorkspaceID: "ws-1", SourceVisibility: "unknown"}, wantErr: `unsupported image source visibility "unknown"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := newArtifactCacheLayout(tt.scope)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			if tt.scope.SourceVisibility == imageSourcePublic {
				require.Equal(t, got.SharedPublicLayersDir, got.layerCacheRoot())
			} else {
				require.Equal(t, got.PrivateLayersDir, got.layerCacheRoot())
			}
		})
	}
}

// TestArtifactCacheLayoutPathHelpers covers layer and final artifact path
// derivation for the cache layout.
func TestArtifactCacheLayoutPathHelpers(t *testing.T) {
	t.Parallel()

	layout, err := newArtifactCacheLayout(artifactCacheScope{WorkspaceID: "ws-7", SourceVisibility: imageSourcePublic})
	require.NoError(t, err)

	layerPath, err := layout.layerBlobPath("sha256:deadbeef")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(workloadCacheRootDir, sharedPublicLayersDirName, "sha256", "deadbeef", "blob"), layerPath)

	artifactPath, err := layout.artifactPath("sha256:cafebabe")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(workloadCacheRootDir, workspaceCacheDirName, "ws-7", artifactCacheDirName, "sha256", "cafebabe", artifactFileName), artifactPath)
}

// TestFileExists covers the filesystem predicate used by cache planning and
// artifact publication.
func TestFileExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	existingPath := filepath.Join(root, "present.txt")
	require.NoError(t, os.WriteFile(existingPath, []byte("ok"), 0o644))

	exists, err := fileExists(existingPath)
	require.NoError(t, err)
	require.True(t, exists)

	missing, err := fileExists(filepath.Join(root, "missing.txt"))
	require.NoError(t, err)
	require.False(t, missing)

	blockedParent := filepath.Join(root, "blocked")
	require.NoError(t, os.WriteFile(blockedParent, []byte("x"), 0o644))

	_, err = fileExists(filepath.Join(blockedParent, "child"))
	require.Error(t, err)
}

// TestCollectLayerDigests covers ordered projection of resolved layers into
// plain digest strings.
func TestCollectLayerDigests(t *testing.T) {
	t.Parallel()

	require.Nil(t, collectLayerDigests(nil))
	require.Nil(t, collectLayerDigests([]resolvedLayer{}))
	require.Equal(t, []string{"sha256:one", "sha256:two"}, collectLayerDigests([]resolvedLayer{{Digest: "sha256:one"}, {Digest: "sha256:two"}}))
}

// TestPlanMaterialization covers cache-hit and cache-miss planning for a
// workspace-scoped artifact path.
func TestPlanMaterialization(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := artifactCacheLayout{
		RootDir:               root,
		WorkspaceID:           "ws-1",
		SourceVisibility:      imageSourcePublic,
		SharedPublicLayersDir: filepath.Join(root, sharedPublicLayersDirName),
		PrivateLayersDir:      filepath.Join(root, workspaceCacheDirName, "ws-1", privateLayersDirName),
		ArtifactsDir:          filepath.Join(root, workspaceCacheDirName, "ws-1", artifactCacheDirName),
		TmpDir:                filepath.Join(root, workspaceCacheDirName, "ws-1", artifactWorkDirName),
	}
	require.NoError(t, layout.ensureBaseDirs())

	resolved := resolvedImageLayers{
		ImageDigest: "sha256:deadbeef",
		Layers: []resolvedLayer{
			{Digest: "sha256:layer-a"},
			{Digest: "sha256:layer-b"},
		},
	}

	artifactPath, err := layout.artifactPath(resolved.ImageDigest)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("cached"), 0o444))

	layerPath, err := layout.layerBlobPath("sha256:layer-a")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(layerPath), 0o755))
	require.NoError(t, os.WriteFile(layerPath, []byte("layer-a"), 0o444))

	plan, err := planMaterialization(layout, resolved)
	require.NoError(t, err)
	require.True(t, plan.ArtifactExists)
	require.Equal(t, artifactPath, plan.ArtifactPath)
	require.Equal(t, []string{"sha256:layer-b"}, plan.MissingLayerDigests)
	require.Len(t, plan.Layers, 2)
	require.Equal(t, resolved.Layers, plan.Layers)
}

// TestPlanMaterializationStatErrors covers planning failures when the cache
// layout points into an invalid filesystem shape.
func TestPlanMaterializationStatErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

	resolved := resolvedImageLayers{ImageDigest: "sha256:deadbeef", Layers: []resolvedLayer{{Digest: "sha256:layer-a"}}}

	t.Run("artifact stat error", func(t *testing.T) {
		t.Parallel()

		layout := artifactCacheLayout{
			SourceVisibility:      imageSourcePublic,
			SharedPublicLayersDir: filepath.Join(root, sharedPublicLayersDirName),
			ArtifactsDir:          filepath.Join(blocked, "artifacts"),
		}

		_, err := planMaterialization(layout, resolved)
		require.ErrorContains(t, err, "stat artifact cache")
	})

	t.Run("layer stat error", func(t *testing.T) {
		t.Parallel()

		layout := artifactCacheLayout{
			SourceVisibility:      imageSourcePublic,
			SharedPublicLayersDir: filepath.Join(blocked, "layers"),
			ArtifactsDir:          filepath.Join(root, "artifacts"),
		}

		_, err := planMaterialization(layout, resolved)
		require.ErrorContains(t, err, "stat layer cache for sha256:layer-a")
	})
}

// TestEnsureBaseDirsError covers directory creation failures when a cache root
// resolves underneath a regular file.
func TestEnsureBaseDirsError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

	layout := artifactCacheLayout{
		SourceVisibility:      imageSourcePublic,
		SharedPublicLayersDir: filepath.Join(blocked, "layers"),
		ArtifactsDir:          filepath.Join(root, "artifacts"),
		TmpDir:                filepath.Join(root, "tmp"),
	}

	err := layout.ensureBaseDirs()
	require.ErrorContains(t, err, `create cache dir`)
}

// TestMaterializeArtifactRejectsInvalidScope covers the top-level wrapper
// branch that fails before image resolution starts.
func TestMaterializeArtifactRejectsInvalidScope(t *testing.T) {
	t.Parallel()

	_, err := materializeArtifact(context.Background(), artifactCacheScope{}, "ghcr.io/acme/app:latest")
	require.EqualError(t, err, "workspace id is required for artifact cache scope")
}

// TestResolveImageLayersRejectsInvalidImageReference covers the image
// resolution error path before any registry interaction succeeds.
func TestResolveImageLayersRejectsInvalidImageReference(t *testing.T) {
	t.Parallel()

	_, err := resolveImageLayers(context.Background(), "://not-a-ref")
	require.ErrorContains(t, err, "fetch image descriptor")
}

// TestMaterializeArtifactWithLayout exercises the full happy path from OCI
// image resolution through cached layer replay and final EROFS publication.
func TestMaterializeArtifactWithLayout(t *testing.T) {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	refText := strings.TrimPrefix(server.URL, "http://") + "/acme/app:latest"
	ref, err := name.ParseReference(refText, name.WeakValidation, name.Insecure)
	require.NoError(t, err)

	layer1 := mustLayerFromEntries(t, []tarEntry{
		{header: tar.Header{Name: "etc", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: "etc/hosts", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("lower-hosts\n"))}, content: []byte("lower-hosts\n")},
		{header: tar.Header{Name: "usr", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: "usr/bin", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: "usr/bin/tool", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("tool\n"))}, content: []byte("tool\n")},
		{header: tar.Header{Name: "usr/bin/tool-link", Typeflag: tar.TypeSymlink, Linkname: "tool", Mode: 0o777}},
		{header: tar.Header{Name: "usr/bin/tool-hard", Typeflag: tar.TypeLink, Linkname: "usr/bin/tool", Mode: 0o755}},
		{header: tar.Header{Name: "var", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: "var/lib", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: "var/lib/old", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("old\n"))}, content: []byte("old\n")},
	})

	layer2 := mustLayerFromEntries(t, []tarEntry{
		{header: tar.Header{Name: "etc/.wh.hosts", Typeflag: tar.TypeReg, Mode: 0o000}},
		{header: tar.Header{Name: "var/lib/.wh..wh..opq", Typeflag: tar.TypeReg, Mode: 0o000}},
		{header: tar.Header{Name: "var/lib/new", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("new\n"))}, content: []byte("new\n")},
	})

	img, err := mutate.Append(empty.Image, mutate.Addendum{Layer: layer1}, mutate.Addendum{Layer: layer2})
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))

	root := t.TempDir()
	layout := artifactCacheLayout{
		RootDir:               root,
		WorkspaceID:           "ws-1",
		SourceVisibility:      imageSourcePublic,
		SharedPublicLayersDir: filepath.Join(root, sharedPublicLayersDirName),
		PrivateLayersDir:      filepath.Join(root, workspaceCacheDirName, "ws-1", privateLayersDirName),
		ArtifactsDir:          filepath.Join(root, workspaceCacheDirName, "ws-1", artifactCacheDirName),
		TmpDir:                filepath.Join(root, workspaceCacheDirName, "ws-1", artifactWorkDirName),
	}

	binDir := t.TempDir()
	counterFile := filepath.Join(t.TempDir(), "mkfs-count.txt")
	script := "#!/bin/sh\nset -eu\nrootfs=\"$8\"\noutput=\"$7\"\n{\n  find \"$rootfs\" -mindepth 1 | sort\n  printf 'symlink:%s\\n' \"$(readlink \"$rootfs/usr/bin/tool-link\")\"\n  printf 'tool_inode:%s\\n' \"$(stat -c '%i' \"$rootfs/usr/bin/tool\")\"\n  printf 'hard_inode:%s\\n' \"$(stat -c '%i' \"$rootfs/usr/bin/tool-hard\")\"\n} > \"$output\"\nprintf 'run\\n' >> " + quoteShellPath(counterFile) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mkfs.erofs"), []byte(script), 0o755))
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	got, err := materializeArtifactWithLayout(context.Background(), layout, refText)
	require.NoError(t, err)
	require.NotEmpty(t, got.ImageDigest)
	require.Len(t, got.LayerDigests, 2)

	artifactPath, err := layout.artifactPath(got.ImageDigest)
	require.NoError(t, err)
	require.Equal(t, artifactPath, got.ArtifactPath)

	content, err := os.ReadFile(artifactPath)
	require.NoError(t, err)
	text := string(content)
	require.Contains(t, text, "rootfs/etc")
	require.NotContains(t, text, "rootfs/etc/hosts")
	require.Contains(t, text, "rootfs/usr/bin/tool")
	require.Contains(t, text, "rootfs/usr/bin/tool-link")
	require.Contains(t, text, "rootfs/usr/bin/tool-hard")
	require.Contains(t, text, "rootfs/var/lib/new")
	require.NotContains(t, text, "rootfs/var/lib/old")
	require.Contains(t, text, "symlink:tool")
	require.Equal(t, extractLineValue(t, text, "tool_inode:"), extractLineValue(t, text, "hard_inode:"))

	runs, err := os.ReadFile(counterFile)
	require.NoError(t, err)
	require.Equal(t, "run\n", string(runs))

	got2, err := materializeArtifactWithLayout(context.Background(), layout, refText)
	require.NoError(t, err)
	require.Equal(t, got.ArtifactPath, got2.ArtifactPath)
	require.Equal(t, got.ImageDigest, got2.ImageDigest)

	runs, err = os.ReadFile(counterFile)
	require.NoError(t, err)
	require.Equal(t, "run\n", string(runs))
}

type tarEntry struct {
	header  tar.Header
	content []byte
}

func mustLayerFromEntries(t *testing.T, entries []tarEntry) gcrv1.Layer {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		hdr := entry.header
		// Treat Mode==0 as "unspecified" in test fixtures and default to a
		// regular file mode. Some callers intentionally pass Mode: 0 and rely
		// on this helper to apply 0o644.
		if hdr.Mode == 0 {
			hdr.Mode = 0o644
		}
		if len(entry.content) > 0 && hdr.Size == 0 {
			hdr.Size = int64(len(entry.content))
		}
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatalf("write tar header %q: %v", hdr.Name, err)
		}
		if len(entry.content) > 0 {
			if _, err := tw.Write(entry.content); err != nil {
				t.Fatalf("write tar content %q: %v", hdr.Name, err)
			}
		}
	}
	require.NoError(t, tw.Close())

	digest, _, err := gcrv1.SHA256(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	return &testLayer{
		digest:    digest,
		diffID:    digest,
		data:      append([]byte(nil), buf.Bytes()...),
		mediaType: gcrv1types.OCILayer,
	}
}

func extractLineValue(t *testing.T, text, prefix string) string {
	t.Helper()

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("missing line with prefix %q", prefix)
	return ""
}

func quoteShellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

type testLayer struct {
	digest    gcrv1.Hash
	diffID    gcrv1.Hash
	data      []byte
	mediaType gcrv1types.MediaType
}

func (l *testLayer) Digest() (gcrv1.Hash, error) { return l.digest, nil }

func (l *testLayer) DiffID() (gcrv1.Hash, error) { return l.diffID, nil }
func (l *testLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.data)), nil
}
func (l *testLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.data)), nil
}
func (l *testLayer) Size() (int64, error)                     { return int64(len(l.data)), nil }
func (l *testLayer) MediaType() (gcrv1types.MediaType, error) { return l.mediaType, nil }
