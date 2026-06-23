//go:build linux

package workload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

type testCloser struct {
	closed bool
	err    error
}

func (c *testCloser) Close() error {
	c.closed = true
	return c.err
}

// TestNormalizeTarPath covers path cleaning for regular entries and the root
// entry that should be skipped by callers.
func TestNormalizeTarPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "root entry", input: "", want: ""},
		{name: "dot entry", input: ".", want: ""},
		{name: "plain path", input: "etc/passwd", want: "etc/passwd"},
		{name: "leading slash removed", input: "/etc/passwd", want: "etc/passwd"},
		{name: "dot segments cleaned", input: "./var/./log/app.log", want: "var/log/app.log"},
		{name: "parent segments normalized", input: "usr/bin/../lib/libc.so", want: "usr/lib/libc.so"},
		{name: "leading traversal collapses into root path", input: "../etc/passwd", want: "etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeTarPath(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestMultiCloserClose covers closing every wrapped closer and joining any
// errors that occur.
func TestMultiCloserClose(t *testing.T) {
	t.Parallel()

	t.Run("closes every non-nil closer", func(t *testing.T) {
		t.Parallel()

		first := &testCloser{}
		second := &testCloser{}

		err := multiCloser{closers: []io.Closer{first, nil, second}}.Close()
		require.NoError(t, err)
		require.True(t, first.closed)
		require.True(t, second.closed)
	})

	t.Run("joins closer errors", func(t *testing.T) {
		t.Parallel()

		first := &testCloser{err: errors.New("first close failed")}
		second := &testCloser{err: errors.New("second close failed")}

		err := multiCloser{closers: []io.Closer{first, second}}.Close()
		require.Error(t, err)
		require.ErrorContains(t, err, "first close failed")
		require.ErrorContains(t, err, "second close failed")
		require.True(t, first.closed)
		require.True(t, second.closed)
	})
}

// TestApplyWhiteout covers both normal whiteouts that remove a single entry and
// opaque whiteouts that clear a directory.
func TestApplyWhiteout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rel         string
		setup       func(t *testing.T, rootfs string)
		wantHandled bool
		wantGone    []string
		wantRemain  []string
	}{
		{
			name: "normal whiteout removes target",
			rel:  "etc/.wh.hosts",
			setup: func(t *testing.T, rootfs string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(rootfs, "etc", "hosts"), []byte("old"), 0o644))
			},
			wantHandled: true,
			wantGone:    []string{"etc/hosts"},
		},
		{
			name: "opaque whiteout clears directory",
			rel:  "var/lib/.wh..wh..opq",
			setup: func(t *testing.T, rootfs string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(rootfs, "var", "lib"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(rootfs, "var", "lib", "one"), []byte("1"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(rootfs, "var", "lib", "two"), []byte("2"), 0o644))
			},
			wantHandled: true,
			wantGone:    []string{"var/lib/one", "var/lib/two"},
		},
		{
			name: "non whiteout leaves tree alone",
			rel:  "etc/passwd",
			setup: func(t *testing.T, rootfs string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(rootfs, "etc", "passwd"), []byte("x"), 0o644))
			},
			wantHandled: false,
			wantRemain:  []string{"etc/passwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootfs := t.TempDir()
			tt.setup(t, rootfs)

			handled, err := applyWhiteout(rootfs, tt.rel)
			require.NoError(t, err)
			require.Equal(t, tt.wantHandled, handled)

			for _, rel := range tt.wantGone {
				_, err := os.Stat(filepath.Join(rootfs, filepath.FromSlash(rel)))
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			for _, rel := range tt.wantRemain {
				_, err := os.Stat(filepath.Join(rootfs, filepath.FromSlash(rel)))
				require.NoError(t, err)
			}
		})
	}
}

// TestCacheMissingLayersErrorPaths covers the easy pre-download failure paths:
// invalid digests and layer cache roots that cannot be statted.
func TestCacheMissingLayersErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid digest", func(t *testing.T) {
		t.Parallel()

		layout := artifactCacheLayout{SourceVisibility: imageSourcePublic, SharedPublicLayersDir: t.TempDir()}
		err := cacheMissingLayers(context.Background(), layout, []resolvedLayer{{Digest: "not-a-digest"}})
		require.EqualError(t, err, `invalid digest "not-a-digest"`)
	})

	t.Run("stat layer cache error", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		blocked := filepath.Join(root, "blocked")
		require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

		layout := artifactCacheLayout{SourceVisibility: imageSourcePublic, SharedPublicLayersDir: filepath.Join(blocked, "layers")}
		err := cacheMissingLayers(context.Background(), layout, []resolvedLayer{{Digest: "sha256:deadbeef"}})
		require.ErrorContains(t, err, "stat layer cache for sha256:deadbeef")
	})
}

// TestCacheOneLayerErrorPaths covers the failure branches before a cached blob
// is successfully published.
func TestCacheOneLayerErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("missing source handle", func(t *testing.T) {
		t.Parallel()

		err := cacheOneLayer(context.Background(), resolvedLayer{Digest: "sha256:deadbeef"}, filepath.Join(t.TempDir(), "blob"))
		require.EqualError(t, err, "layer sha256:deadbeef has no source handle")
	})

	t.Run("compressed open error", func(t *testing.T) {
		t.Parallel()

		layer := resolvedLayer{Digest: "sha256:deadbeef", source: &errorLayer{compressedErr: errors.New("boom")}}
		err := cacheOneLayer(context.Background(), layer, filepath.Join(t.TempDir(), "blob"))
		require.ErrorContains(t, err, "open compressed layer sha256:deadbeef")
	})

	t.Run("copy error", func(t *testing.T) {
		t.Parallel()

		layer := resolvedLayer{Digest: "sha256:deadbeef", source: &errorLayer{compressedData: io.NopCloser(errReader{err: errors.New("read failed")})}}
		err := cacheOneLayer(context.Background(), layer, filepath.Join(t.TempDir(), "blob"))
		require.ErrorContains(t, err, "cache compressed layer sha256:deadbeef")
	})
}

// TestOpenCachedLayer covers the compression-aware reopen path used when a
// cached blob needs to be replayed back into tar form.
func TestOpenCachedLayer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plainPath := filepath.Join(root, "plain.layer")
	gzipPath := filepath.Join(root, "gzip.layer")
	zstdPath := filepath.Join(root, "zstd.layer")
	unsupportedPath := filepath.Join(root, "other.layer")
	badGzipPath := filepath.Join(root, "bad-gzip.layer")

	require.NoError(t, os.WriteFile(plainPath, []byte("plain"), 0o644))

	gw, err := os.Create(gzipPath)
	require.NoError(t, err)
	gzr := gzip.NewWriter(gw)
	_, err = gzr.Write([]byte("gzip"))
	require.NoError(t, err)
	require.NoError(t, gzr.Close())
	require.NoError(t, gw.Close())

	zw, err := os.Create(zstdPath)
	require.NoError(t, err)
	enc, err := zstd.NewWriter(zw)
	require.NoError(t, err)
	_, err = enc.Write([]byte("zstd"))
	require.NoError(t, err)
	require.NoError(t, enc.Close())
	require.NoError(t, zw.Close())

	require.NoError(t, os.WriteFile(unsupportedPath, []byte("other"), 0o644))
	require.NoError(t, os.WriteFile(badGzipPath, []byte("not-gzip"), 0o644))

	tests := []struct {
		name      string
		path      string
		mediaType types.MediaType
		want      string
		wantErr   string
	}{
		{name: "oci layer decompresses gzip", path: gzipPath, mediaType: types.OCILayer, want: "gzip"},
		{name: "docker layer decompresses gzip", path: gzipPath, mediaType: types.DockerLayer, want: "gzip"},
		{name: "oci layer rejects non-gzip content", path: badGzipPath, mediaType: types.OCILayer, wantErr: "EOF"},
		{name: "docker layer rejects non-gzip content", path: badGzipPath, mediaType: types.DockerLayer, wantErr: "EOF"},
		{name: "gzip fallback via media type string", path: gzipPath, mediaType: types.MediaType("application/vnd.example.layer+gzip"), want: "gzip"},
		{name: "zstd layer", path: zstdPath, mediaType: types.OCILayerZStd, want: "zstd"},
		{name: "invalid gzip layer", path: badGzipPath, mediaType: types.MediaType("application/vnd.example.layer+gzip"), wantErr: "unexpected EOF"},
		{name: "unsupported media type", path: unsupportedPath, mediaType: types.MediaType("application/vnd.example.unknown"), wantErr: `unsupported layer media type "application/vnd.example.unknown"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rc, err := openCachedLayer(tt.path, tt.mediaType)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			defer func() { _ = rc.Close() }()

			got, err := io.ReadAll(rc)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

// TestApplyLayersFromCacheErrorPaths covers reopen and tar replay failures from
// cached layer blobs.
func TestApplyLayersFromCacheErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("unsupported media type", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		layout := artifactCacheLayout{SourceVisibility: imageSourcePublic, SharedPublicLayersDir: filepath.Join(root, sharedPublicLayersDirName)}
		layerPath, err := layout.layerBlobPath("sha256:deadbeef")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(layerPath), 0o755))
		require.NoError(t, os.WriteFile(layerPath, []byte("plain"), 0o644))

		err = applyLayersFromCache(layout, []resolvedLayer{{Digest: "sha256:deadbeef", MediaType: types.MediaType("application/vnd.example.unknown")}}, filepath.Join(root, "rootfs"))
		require.ErrorContains(t, err, "open cached layer sha256:deadbeef")
	})

	t.Run("invalid tar data", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		layout := artifactCacheLayout{SourceVisibility: imageSourcePublic, SharedPublicLayersDir: filepath.Join(root, sharedPublicLayersDirName)}
		layerPath, err := layout.layerBlobPath("sha256:deadbeef")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(layerPath), 0o755))
		require.NoError(t, os.WriteFile(layerPath, []byte("not a tar stream"), 0o644))

		err = applyLayersFromCache(layout, []resolvedLayer{{Digest: "sha256:deadbeef", MediaType: types.OCILayer}}, filepath.Join(root, "rootfs"))
		require.ErrorContains(t, err, "open cached layer sha256:deadbeef")
	})
}

// TestApplyLayerTarErrorPaths covers malformed tar streams and unsupported tar
// entry types.
func TestApplyLayerTarErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("malformed tar", func(t *testing.T) {
		t.Parallel()

		err := applyLayerTar(t.TempDir(), bytes.NewReader([]byte("not a tar stream")))
		require.ErrorContains(t, err, "read tar entry")
	})

	t.Run("unsupported tar type", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "weird", Typeflag: tar.TypeChar, Mode: 0o644}))
		require.NoError(t, tw.Close())

		err := applyLayerTar(t.TempDir(), bytes.NewReader(buf.Bytes()))
		require.ErrorContains(t, err, `unsupported tar entry type`)
	})
}

// TestBuildEROFS covers the mkfs.erofs command line wiring and output publish
// behavior without depending on the host toolchain.
func TestBuildEROFS(t *testing.T) {
	rootfs := t.TempDir()
	output := filepath.Join(t.TempDir(), "artifact.erofs")
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args.txt")

	script := fmt.Sprintf(`#!/bin/sh
set -eu
printf '%%s\n' "$@" > %q
: > "$6"
`, argsFile)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mkfs.erofs"), []byte(script), 0o755))
	prevPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+prevPath)

	err := buildEROFS(context.Background(), rootfs, output)
	require.NoError(t, err)

	_, err = os.Stat(output)
	require.NoError(t, err)

	content, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	args := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Equal(t, []string{"--all-root", "-T", "0", "-z", "lz4", output, rootfs}, args)
}

// TestBuildEROFSErrorPaths covers the lookup and command failure branches of
// mkfs.erofs invocation.
func TestBuildEROFSErrorPaths(t *testing.T) {
	t.Run("missing mkfs.erofs", func(t *testing.T) {
		oldPath := os.Getenv("PATH")
		t.Setenv("PATH", t.TempDir()+string(os.PathListSeparator)+oldPath)

		err := buildEROFS(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "artifact.erofs"))
		require.ErrorContains(t, err, "mkfs.erofs not found")
	})

	t.Run("mkfs.erofs command failure", func(t *testing.T) {
		binDir := t.TempDir()
		script := "#!/bin/sh\necho failed >&2\nexit 1\n"
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "mkfs.erofs"), []byte(script), 0o755))
		oldPath := os.Getenv("PATH")
		t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

		err := buildEROFS(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "artifact.erofs"))
		require.ErrorContains(t, err, "build erofs image")
		require.ErrorContains(t, err, "failed")
	})
}

// TestPublishImmutableFile covers the publishing path that makes a temp file
// visible as the final immutable artifact.
func TestPublishImmutableFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		precreate   bool
		wantTmpGone bool
		wantFinal   string
	}{
		{name: "publishes when final missing", precreate: false, wantTmpGone: true, wantFinal: "payload"},
		{name: "keeps existing final file", precreate: true, wantTmpGone: true, wantFinal: "existing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			tmpPath := filepath.Join(root, "build.tmp")
			finalPath := filepath.Join(root, "final.erofs")

			require.NoError(t, os.WriteFile(tmpPath, []byte("payload"), 0o644))
			if tt.precreate {
				require.NoError(t, os.WriteFile(finalPath, []byte("existing"), 0o444))
			}

			require.NoError(t, publishImmutableFile(tmpPath, finalPath))

			if tt.wantTmpGone {
				_, err := os.Stat(tmpPath)
				require.ErrorIs(t, err, os.ErrNotExist)
			}

			content, err := os.ReadFile(finalPath)
			require.NoError(t, err)
			require.Equal(t, tt.wantFinal, string(content))
		})
	}
}

// TestPublishImmutableFileCacheHitRemoveError covers the cache-hit branch where
// removing the redundant temp path itself fails.
func TestPublishImmutableFileCacheHitRemoveError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tmpPath := filepath.Join(root, "tmpdir")
	finalPath := filepath.Join(root, "final.erofs")
	require.NoError(t, os.MkdirAll(filepath.Join(tmpPath, "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpPath, "child", "payload"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(finalPath, []byte("existing"), 0o444))

	err := publishImmutableFile(tmpPath, finalPath)
	require.ErrorContains(t, err, "remove temp file after cache hit")
}

// TestEnsureDirAt covers replacing a non-directory entry and the error branch
// where the parent path shape is invalid.
func TestEnsureDirAt(t *testing.T) {
	t.Parallel()

	t.Run("replaces file with directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := filepath.Join(root, "target")
		require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

		require.NoError(t, ensureDirAt(target, 0o755))
		info, err := os.Stat(target)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	})

	t.Run("invalid parent", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		blocked := filepath.Join(root, "blocked")
		require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

		err := ensureDirAt(filepath.Join(blocked, "child"), 0o755)
		require.Error(t, err)
	})
}

// TestRemovePathForReplace covers both successful removal and the error branch
// when the parent path cannot be traversed.
func TestRemovePathForReplace(t *testing.T) {
	t.Parallel()

	t.Run("removes existing path", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := filepath.Join(root, "target")
		require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

		require.NoError(t, removePathForReplace(target))
		_, err := os.Stat(target)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("invalid parent", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		blocked := filepath.Join(root, "blocked")
		require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

		err := removePathForReplace(filepath.Join(blocked, "child"))
		require.Error(t, err)
	})
}

// TestWriteRegularFileError covers the stream-copy failure branch.
func TestWriteRegularFileError(t *testing.T) {
	t.Parallel()

	err := writeRegularFile(filepath.Join(t.TempDir(), "file"), errReader{err: errors.New("read failed")}, 0o644)
	require.ErrorContains(t, err, "read failed")
}

type errorLayer struct {
	compressedErr  error
	compressedData io.ReadCloser
}

func (l *errorLayer) Digest() (gcrv1.Hash, error) { return gcrv1.Hash{}, nil }
func (l *errorLayer) DiffID() (gcrv1.Hash, error) { return gcrv1.Hash{}, nil }
func (l *errorLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (l *errorLayer) Size() (int64, error)                { return 0, nil }
func (l *errorLayer) MediaType() (types.MediaType, error) { return types.OCILayer, nil }
func (l *errorLayer) Compressed() (io.ReadCloser, error) {
	if l.compressedErr != nil {
		return nil, l.compressedErr
	}
	return l.compressedData, nil
}

type errReader struct{ err error }

func (r errReader) Read(_ []byte) (int, error) { return 0, r.err }
