// Command builderd is the SpaceScale artifact builder daemon.
//
// builderd is responsible for materializing deployable workload images from OCI
// container references. It pulls images from public and private registries,
// extracts and merges layers using OCI whiteout semantics, and produces
// compressed read-only EROFS filesystem images that compute nodes download at
// launch time.
//
// builderd connects to the regional NATS fabric and listens for build requests
// dispatched by controlp during workload creation. Completed artifacts are
// published to a shared object store accessible by all compute nodes in the
// region.
//
// This separation of build and compute allows:
//   - Sub-second VM boot times (artifact is pre-built, compute node only downloads)
//   - Cross-region VM mobility (artifact in object store, any region can pull it)
//   - Efficient caching (same image digest produces same artifact, built once)
//   - Isolation of heavy I/O (registry pulls, layer extraction) from the compute path
package main

import "fmt"

func main() {
	fmt.Println("builderd is not yet implemented")
}
