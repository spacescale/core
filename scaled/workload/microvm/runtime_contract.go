//go:build linux

// Package microvm defines the host-side Firecracker launch path and the MMDS
// runtime contract that guestd reads during early boot.
package microvm

type guestRuntimeMetadata struct {
	ImageRef      string                 `json:"image_ref,omitempty"`
	Entrypoint    []string               `json:"entrypoint,omitempty"`
	Cmd           []string               `json:"cmd,omitempty"`
	WorkingDir    string                 `json:"working_dir,omitempty"`
	User          string                 `json:"user,omitempty"`
	Env           map[string]string      `json:"env,omitempty"`
	RuntimePort   *uint32                `json:"runtime_port,omitempty"`
	Artifact      *workloadArtifactRef   `json:"artifact,omitempty"`
	WritableState *writableStateContract `json:"writable_state,omitempty"`
}

type workloadArtifactRef struct {
	Digest string `json:"digest,omitempty"`
	Format string `json:"format,omitempty"`
}

type writableStateContract struct {
	Kind      string `json:"kind,omitempty"`
	MountPath string `json:"mount_path,omitempty"`
}

func runtimeMetadataDocument(req LaunchRequest) map[string]any {
	runtime := guestRuntimeMetadata{
		ImageRef: req.ImageRef,
		Env:      cloneStringMap(req.Env),
	}

	if req.RuntimePort != 0 {
		port := req.RuntimePort
		runtime.RuntimePort = &port
	}

	return map[string]any{
		"version":    uint32(1),
		"microvm_id": req.MicroVMID,
		"runtime":    runtime,
	}
}

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
