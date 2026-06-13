//go:build linux

package microvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeMetadataDocumentIncludesRuntimeFields(t *testing.T) {
	req := LaunchRequest{
		MicroVMID:   "vm-123",
		ImageRef:    "ghcr.io/acme/app:latest",
		Env:         map[string]string{"FOO": "bar"},
		RuntimePort: 3000,
	}

	doc := runtimeMetadataDocument(req)

	require.Equal(t, uint32(1), doc["version"])
	require.Equal(t, req.MicroVMID, doc["microvm_id"])

	runtime, ok := doc["runtime"].(guestRuntimeMetadata)
	require.True(t, ok)
	require.Equal(t, req.ImageRef, runtime.ImageRef)
	require.Equal(t, req.Env, runtime.Env)
	require.NotNil(t, runtime.RuntimePort)
	require.Equal(t, req.RuntimePort, *runtime.RuntimePort)

	req.Env["FOO"] = "changed"
	require.Equal(t, "bar", runtime.Env["FOO"])
}

func TestRuntimeMetadataDocumentOmitsUnknownRuntimePort(t *testing.T) {
	doc := runtimeMetadataDocument(LaunchRequest{MicroVMID: "vm-456"})

	runtime, ok := doc["runtime"].(guestRuntimeMetadata)
	require.True(t, ok)
	require.Nil(t, runtime.RuntimePort)
	require.Nil(t, runtime.Env)
	require.Empty(t, runtime.ImageRef)
}
