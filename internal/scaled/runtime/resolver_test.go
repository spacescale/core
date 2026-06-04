package runtime

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentPaths(t *testing.T) {
	paths := currentPaths("/var/lib/spacescale/runtime")

	assert.Equal(t, "/var/lib/spacescale/runtime/host/firecracker-v1.15.1-x86_64", paths.FirecrackerPath)
	assert.Equal(t, "/var/lib/spacescale/runtime/host/jailer-v1.15.1-x86_64", paths.JailerPath)
	assert.Equal(t, "/var/lib/spacescale/runtime/guest/vmlinux-v6.1.169-spacescale4-x86_64", paths.KernelPath)
	assert.Equal(t, "/var/lib/spacescale/runtime/guest/guestd-rootfs-v0.1.3-x86_64-ext4", paths.RootFSPath)
}

func TestResolverReconcileDownloadsMissingAssets(t *testing.T) {
	hits := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[1:]
		hits[key]++

		switch key {
		case firecrackerObjectKey:
			_, _ = w.Write([]byte("firecracker"))
		case jailerObjectKey:
			_, _ = w.Write([]byte("jailer"))
		case kernelObjectKey:
			_, _ = w.Write([]byte("kernel"))
		case rootfsObjectKey:
			_, _ = w.Write([]byte("rootfs"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver := &Resolver{
		logger:  logger,
		client:  server.Client(),
		baseURL: server.URL,
		rootDir: t.TempDir(),
	}

	paths, err := resolver.Reconcile(context.Background())
	require.NoError(t, err)

	assert.FileExists(t, paths.FirecrackerPath)
	assert.FileExists(t, paths.JailerPath)
	assert.FileExists(t, paths.KernelPath)
	assert.FileExists(t, paths.RootFSPath)

	firecrackerInfo, err := os.Stat(paths.FirecrackerPath)
	require.NoError(t, err)
	assert.NotZero(t, firecrackerInfo.Mode()&0o111)

	jailerInfo, err := os.Stat(paths.JailerPath)
	require.NoError(t, err)
	assert.NotZero(t, jailerInfo.Mode()&0o111)

	kernelInfo, err := os.Stat(paths.KernelPath)
	require.NoError(t, err)
	assert.Zero(t, kernelInfo.Mode()&0o111)

	rootfsInfo, err := os.Stat(paths.RootFSPath)
	require.NoError(t, err)
	assert.Zero(t, rootfsInfo.Mode()&0o111)

	assert.Equal(t, 1, hits[firecrackerObjectKey])
	assert.Equal(t, 1, hits[jailerObjectKey])
	assert.Equal(t, 1, hits[kernelObjectKey])
	assert.Equal(t, 1, hits[rootfsObjectKey])
}

func TestResolverReconcileReusesCachedAssets(t *testing.T) {
	hits := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[1:]
		hits[key]++

		switch key {
		case firecrackerObjectKey:
			_, _ = w.Write([]byte("firecracker"))
		case jailerObjectKey:
			_, _ = w.Write([]byte("jailer"))
		case kernelObjectKey:
			_, _ = w.Write([]byte("kernel"))
		case rootfsObjectKey:
			_, _ = w.Write([]byte("rootfs"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver := &Resolver{
		logger:  logger,
		client:  server.Client(),
		baseURL: server.URL,
		rootDir: t.TempDir(),
	}

	_, err := resolver.Reconcile(context.Background())
	require.NoError(t, err)

	_, err = resolver.Reconcile(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, hits[firecrackerObjectKey])
	assert.Equal(t, 1, hits[jailerObjectKey])
	assert.Equal(t, 1, hits[kernelObjectKey])
	assert.Equal(t, 1, hits[rootfsObjectKey])
}

func TestValidateLocalAssetReturnsFalseWhenMissing(t *testing.T) {
	ready, err := validateLocalAsset(t.TempDir()+"/missing", true)
	require.NoError(t, err)
	assert.False(t, ready)
}
