//go:build linux

package workload

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func TestResolveOCIConfig(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	refText := strings.TrimPrefix(server.URL, "http://") + "/acme/app:latest"
	ref, err := name.ParseReference(refText, name.WeakValidation, name.Insecure)
	require.NoError(t, err)

	cfg := &gcrv1.ConfigFile{
		Architecture: supportedImageArch,
		OS:           supportedImageOS,
		Config: gcrv1.Config{
			Entrypoint: []string{"node", "server.js"},
			Cmd:        []string{"--port", "3000"},
			Env: []string{
				"FOO=bar",
				"EMPTY",
				"FOO=baz",
			},
			WorkingDir: "/app",
			User:       "node",
			ExposedPorts: map[string]struct{}{
				"8080/tcp": {},
				"3000/tcp": {},
				"8080/udp": {},
			},
		},
	}

	img, err := mutate.ConfigFile(empty.Image, cfg)
	require.NoError(t, err)

	err = remote.Write(ref, img)
	require.NoError(t, err)

	got, err := resolveOCIConfig(context.Background(), refText)
	require.NoError(t, err)

	require.Equal(t, refText, got.ImageRef)
	require.NotEmpty(t, got.ImageDigest)
	require.Equal(t, []string{"node", "server.js"}, got.Entrypoint)
	require.Equal(t, []string{"--port", "3000"}, got.Cmd)
	require.Equal(t, map[string]string{"FOO": "baz", "EMPTY": ""}, got.Env)
	require.Equal(t, "/app", got.WorkingDir)
	require.Equal(t, "node", got.User)
	require.Equal(t, []uint16{3000, 8080}, got.ExposedPorts)
}

func TestResolveOCIConfigRejectsInvalidImageReference(t *testing.T) {
	t.Parallel()

	_, err := resolveOCIConfig(context.Background(), "://not a ref")
	require.ErrorContains(t, err, "parse image reference")
}

func TestResolveOCIConfigRejectsUnreachableRegistry(t *testing.T) {
	t.Parallel()

	_, err := resolveOCIConfig(context.Background(), "127.0.0.1:1/acme/app:latest")
	require.ErrorContains(t, err, "fetch image descriptor")
}

func TestResolveOCIConfigRejectsMissingRequestedPlatformInIndex(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	refText := strings.TrimPrefix(server.URL, "http://") + "/acme/app:latest"
	ref, err := name.ParseReference(refText, name.WeakValidation, name.Insecure)
	require.NoError(t, err)

	arm64Img, err := mutate.ConfigFile(empty.Image, &gcrv1.ConfigFile{
		Architecture: "arm64",
		OS:           supportedImageOS,
		Config:       gcrv1.Config{},
	})
	require.NoError(t, err)

	idx := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
		Add: arm64Img,
		Descriptor: gcrv1.Descriptor{
			Platform: &gcrv1.Platform{
				OS:           supportedImageOS,
				Architecture: "arm64",
			},
		},
	})

	err = remote.WriteIndex(ref, idx)
	require.NoError(t, err)

	_, err = resolveOCIConfig(context.Background(), refText)
	require.ErrorContains(t, err, "resolve image for platform linux/amd64")
}

func TestResolvedOCIConfigFromConfigFileResolvesFields(t *testing.T) {
	t.Parallel()

	cfg := &gcrv1.ConfigFile{
		Architecture: supportedImageArch,
		OS:           supportedImageOS,
		Config: gcrv1.Config{
			Entrypoint: []string{"node", "server.js"},
			Cmd:        []string{"--port", "3000"},
			Env: []string{
				"FOO=bar",
				"EMPTY",
				"FOO=baz",
			},
			WorkingDir: "/app",
			User:       "node",
			ExposedPorts: map[string]struct{}{
				"8080/tcp": {},
				"3000/tcp": {},
				"8080/udp": {},
			},
		},
	}

	got, err := resolvedOCIConfigFromConfigFile("ghcr.io/acme/app:latest", "sha256:abc123", cfg)
	require.NoError(t, err)
	assertResolvedConfig(t, got)

	cfg.Config.Entrypoint[0] = "changed"
	require.Equal(t, []string{"node", "server.js"}, got.Entrypoint)
}

func TestResolvedOCIConfigFromConfigFileRejectsNilConfig(t *testing.T) {
	t.Parallel()

	_, err := resolvedOCIConfigFromConfigFile("ghcr.io/acme/app:latest", "sha256:abc123", nil)
	require.EqualError(t, err, "image config is nil")
}

func TestResolvedOCIConfigFromConfigFileRejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	cfg := &gcrv1.ConfigFile{
		Architecture: "arm64",
		OS:           supportedImageOS,
	}

	_, err := resolvedOCIConfigFromConfigFile("ghcr.io/acme/app:latest", "sha256:abc123", cfg)
	require.EqualError(t, err, "unsupported image platform linux/arm64 (want linux/amd64)")
}

func TestResolvedOCIConfigFromConfigFileAllowsNoExposedPorts(t *testing.T) {
	t.Parallel()

	cfg := &gcrv1.ConfigFile{
		Architecture: supportedImageArch,
		OS:           supportedImageOS,
		Config:       gcrv1.Config{},
	}

	got, err := resolvedOCIConfigFromConfigFile("ghcr.io/acme/worker:latest", "sha256:def456", cfg)
	require.NoError(t, err)
	require.Nil(t, got.ExposedPorts)
}

func TestParseExposedPortsRejectsInvalidPort(t *testing.T) {
	t.Parallel()

	_, err := parseExposedPorts(map[string]struct{}{"not-a-port": {}})
	require.EqualError(t, err, `invalid exposed port "not-a-port"`)
}

func TestParseExposedPortsRejectsEmptyPort(t *testing.T) {
	t.Parallel()

	_, err := parseExposedPorts(map[string]struct{}{"/tcp": {}})
	require.EqualError(t, err, `invalid exposed port "/tcp"`)
}

func TestEnvSliceToMapReturnsNilForEmptyInput(t *testing.T) {
	t.Parallel()

	require.Nil(t, envSliceToMap(nil))
	require.Nil(t, envSliceToMap([]string{}))
}

func TestCloneStringsReturnsIndependentCopy(t *testing.T) {
	t.Parallel()

	original := []string{"node", "server.js"}
	cloned := cloneStrings(original)
	original[0] = "changed"

	require.Equal(t, []string{"node", "server.js"}, cloned)
}

func TestCloneStringsReturnsNilForEmptyInput(t *testing.T) {
	t.Parallel()

	require.Nil(t, cloneStrings(nil))
	require.Nil(t, cloneStrings([]string{}))
}

func TestEnsureSupportedImagePlatformAcceptsLinuxAMD64(t *testing.T) {
	t.Parallel()

	err := ensureSupportedImagePlatform(&gcrv1.ConfigFile{
		Architecture: supportedImageArch,
		OS:           supportedImageOS,
	})
	require.NoError(t, err)
}

func assertResolvedConfig(t *testing.T, got resolvedOCIConfig) {
	t.Helper()

	require.Equal(t, "ghcr.io/acme/app:latest", got.ImageRef)
	require.Equal(t, "sha256:abc123", got.ImageDigest)
	require.Equal(t, []string{"node", "server.js"}, got.Entrypoint)
	require.Equal(t, []string{"--port", "3000"}, got.Cmd)
	require.Equal(t, map[string]string{"FOO": "baz", "EMPTY": ""}, got.Env)
	require.Equal(t, "/app", got.WorkingDir)
	require.Equal(t, "node", got.User)
	require.Equal(t, []uint16{3000, 8080}, got.ExposedPorts)
}
