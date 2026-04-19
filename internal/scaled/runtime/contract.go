// Package runtime owns the small amount of runtime asset logic that scaled
// needs during startup
//
// # The job here is intentionally narrow
//
// scaled needs four files before it can safely move toward real microvm boot
// firecracker
// jailer
// the guest kernel
// scoutd
//
// For the current phase of the project we hardcode the production bucket and we
// hardcode the exact object names we expect to stage locally
//
// This is deliberately less abstract than a generic asset management layer
// because issue twelve already tells us that long term we are moving toward an
// immutable golden image where these files are baked onto the host ahead of time
//
// So this package should stay easy to delete or simplify later
package runtime

import "path/filepath"

const (
	// runtimeBucketBaseURL is the current production source of truth for runtime
	// artifacts
	//
	// We keep this hardcoded for now so there is one clear place where scaled gets
	// its runtime files and no extra environment management is needed
	runtimeBucketBaseURL = "https://spacescale-runtime-assets.s3.eu-west-par.io.cloud.ovh.net"

	// runtimeStateDir is the local cache root for runtime assets
	//
	// If an asset already exists here and passes basic local checks we reuse it on
	// the next daemon boot instead of downloading it again
	runtimeStateDir = "/var/lib/spacescale/runtime"

	firecrackerObjectKey = "firecracker-v1.15.1-x86_64"
	jailerObjectKey      = "jailer-v1.15.1-x86_64"
	kernelObjectKey      = "vmlinux-v6.1.80-x86_64"
	scoutdObjectKey      = "scoutd-v0.1.0-x86_64-linux-musl"
)

// Paths holds the concrete local paths that later runtime code will consume
// after startup reconciliation succeeds
//
// These are the paths scaled should use for the current runtime generation on
// disk
type Paths struct {
	RootDir         string
	FirecrackerPath string
	JailerPath      string
	KernelPath      string
	ScoutdPath      string
}

// currentPaths returns the fixed local cache paths used by the current daemon
// build
//
// We keep the versioned object key in the filename itself so different asset
// generations can coexist without colliding on disk
func currentPaths(root string) Paths {
	return Paths{
		RootDir:         root,
		FirecrackerPath: filepath.Join(root, "host", firecrackerObjectKey),
		JailerPath:      filepath.Join(root, "host", jailerObjectKey),
		KernelPath:      filepath.Join(root, "guest", kernelObjectKey),
		ScoutdPath:      filepath.Join(root, "guest", scoutdObjectKey),
	}
}
