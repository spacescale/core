//go:build linux

// Package microvm defines the host-side Firecracker launch path and the MMDS
// runtime contract that guestd reads during early boot.
package microvm

// guestRuntimeMetadata is the guest-facing launch contract written into MMDS.
//
// It stays package-local because callers should build launch metadata through
// LaunchRequest rather than manually constructing MMDS payload fragments.
type guestRuntimeMetadata struct {
	ImageRef      string                 `json:"image_ref,omitempty"`
	ImageDigest   string                 `json:"image_digest,omitempty"`
	Command       []string               `json:"command,omitempty"`
	WorkingDir    string                 `json:"working_dir,omitempty"`
	User          string                 `json:"user,omitempty"`
	Env           map[string]string      `json:"env,omitempty"`
	RuntimePort   *uint32                `json:"runtime_port,omitempty"`
	Artifact      *workloadArtifactRef   `json:"artifact,omitempty"`
	WritableState *writableStateContract `json:"writable_state,omitempty"`
}

// workloadArtifactRef reserves the shape that later artifact materialization
// work will use when the guest needs to identify an attached immutable artifact.
type workloadArtifactRef struct {
	Digest string `json:"digest,omitempty"`
	Format string `json:"format,omitempty"`
}

// writableStateContract reserves the guest-visible writable-state attachment
// shape for later filesystem and runtime-state work.
type writableStateContract struct {
	Kind      string `json:"kind,omitempty"`
	MountPath string `json:"mount_path,omitempty"`
}

// runtimeMetadataDocument translates the host-side LaunchRequest into the
// versioned MMDS payload that guestd reads during boot.
func runtimeMetadataDocument(req LaunchRequest) map[string]any {
	runtime := guestRuntimeMetadata{
		ImageRef:    req.ImageRef,
		ImageDigest: req.ImageDigest,
		Command:     cloneStrings(req.Command),
		WorkingDir:  req.WorkingDir,
		User:        req.User,
		Env:         cloneStringMap(req.Env),
	}

	if req.RuntimePort != 0 {
		runtime.RuntimePort = new(req.RuntimePort)
	}

	return map[string]any{
		"version":    uint32(1),
		"microvm_id": req.MicroVMID,
		"runtime":    runtime,
	}
}

// cloneStringMap isolates the MMDS payload from later mutation of the original
// launch request map.
func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}

	return out
}

// cloneStrings isolates MMDS command slices from later mutation of the original
// launch request data.
func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	out := make([]string, len(in))
	copy(out, in)

	return out
}
