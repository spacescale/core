//go:build linux

package workload

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		tt := tt
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
				require.NoError(t, os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(rootfs, "etc", "passwd"), []byte("x"), 0o644))
			},
			wantHandled: false,
			wantRemain:  []string{"etc/passwd"},
		},
	}

	for _, tt := range tests {
		tt := tt
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

// TestOpenCachedLayer covers the compression-aware reopen path used when a
// cached blob needs to be replayed back into tar form.
func TestOpenCachedLayer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plainPath := filepath.Join(root, "plain.layer")
	gzipPath := filepath.Join(root, "gzip.layer")
	zstdPath := filepath.Join(root, "zstd.layer")
	unsupportedPath := filepath.Join(root, "other.layer")

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

	tests := []struct {
		name      string
		path      string
		mediaType types.MediaType
		want      string
		wantErr   string
	}{
		{name: "plain layer", path: plainPath, mediaType: types.OCILayer, want: "plain"},
		{name: "docker layer", path: plainPath, mediaType: types.DockerLayer, want: "plain"},
		{name: "gzip layer", path: gzipPath, mediaType: types.MediaType("application/vnd.example.layer+gzip"), want: "gzip"},
		{name: "zstd layer", path: zstdPath, mediaType: types.OCILayerZStd, want: "zstd"},
		{name: "unsupported media type", path: unsupportedPath, mediaType: types.MediaType("application/vnd.example.unknown"), wantErr: `unsupported layer media type "application/vnd.example.unknown"`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rc, err := openCachedLayer(tt.path, tt.mediaType)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
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
: > "$7"
`, argsFile)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "mkfs.erofs"), []byte(script), 0o755))
	prevPath := os.Getenv("PATH")
	require.NoError(t, os.Setenv("PATH", binDir+string(os.PathListSeparator)+prevPath))
	t.Cleanup(func() {
		_ = os.Setenv("PATH", prevPath)
	})

	err := buildEROFS(context.Background(), rootfs, output)
	require.NoError(t, err)

	_, err = os.Stat(output)
	require.NoError(t, err)

	content, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	args := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Equal(t, []string{"--all-root", "-T", "0", "--all-time", "-z", "lz4", output, rootfs}, args)
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
		tt := tt
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
