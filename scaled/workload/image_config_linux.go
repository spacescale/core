//go:build linux

package workload

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	supportedImageOS   = "linux"
	supportedImageArch = "amd64"
)

type resolvedOCIConfig struct {
	ImageRef     string
	ImageDigest  string
	Entrypoint   []string
	Cmd          []string
	Env          map[string]string
	WorkingDir   string
	User         string
	ExposedPorts []uint16
}

// resolveOCIConfig fetches one OCI image reference and normalizes the runtime
// fields that later launch and packaging steps need.
//
// This keeps registry access out of the microVM package. The workload package
// owns turning a user-facing image reference into deterministic launch metadata
// before that data is written into the guest runtime contract or used for
// artifact materialization.
func resolveOCIConfig(ctx context.Context, imageRef string) (resolvedOCIConfig, error) {
	ref, err := name.ParseReference(strings.TrimSpace(imageRef), name.WeakValidation)
	if err != nil {
		return resolvedOCIConfig{}, fmt.Errorf("parse image reference: %w", err)
	}

	desc, err := remote.Get(
		ref,
		remote.WithContext(ctx),
		remote.WithPlatform(gcrv1.Platform{
			OS:           supportedImageOS,
			Architecture: supportedImageArch,
		}),
	)

	if err != nil {
		return resolvedOCIConfig{}, fmt.Errorf("fetch image descriptor: %w", err)
	}

	img, err := desc.Image()
	if err != nil {
		return resolvedOCIConfig{}, fmt.Errorf(
			"resolve image for platform %s/%s: %w",
			supportedImageOS,
			supportedImageArch,
			err,
		)
	}

	digest, err := img.Digest()
	if err != nil {
		return resolvedOCIConfig{}, fmt.Errorf("read image digest: %w", err)
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return resolvedOCIConfig{}, fmt.Errorf("read image config: %w", err)
	}

	return resolvedOCIConfigFromConfigFile(imageRef, digest.String(), cfg)
}

// resolvedOCIConfigFromConfigFile converts one OCI config file into the small,
// package-local shape that the rest of scaled should consume.
//
// Keeping this translation separate from the remote fetch path makes the logic
// easier to test later and keeps config normalization independent of network
// behavior.
func resolvedOCIConfigFromConfigFile(imageRef, imageDigest string, cfg *gcrv1.ConfigFile) (resolvedOCIConfig, error) {
	if cfg == nil {
		return resolvedOCIConfig{}, fmt.Errorf("image config is nil")
	}
	if err := ensureSupportedImagePlatform(cfg); err != nil {
		return resolvedOCIConfig{}, err
	}

	ports, err := parseExposedPorts(cfg.Config.ExposedPorts)
	if err != nil {
		return resolvedOCIConfig{}, err
	}

	return resolvedOCIConfig{
		ImageRef:     imageRef,
		ImageDigest:  imageDigest,
		Entrypoint:   cloneStrings(cfg.Config.Entrypoint),
		Cmd:          cloneStrings(cfg.Config.Cmd),
		Env:          envSliceToMap(cfg.Config.Env),
		WorkingDir:   cfg.Config.WorkingDir,
		User:         cfg.Config.User,
		ExposedPorts: ports,
	}, nil
}

func ensureSupportedImagePlatform(cfg *gcrv1.ConfigFile) error {
	if cfg.OS != supportedImageOS || cfg.Architecture != supportedImageArch {
		return fmt.Errorf(
			"unsupported image platform %s/%s (want %s/%s)",
			cfg.OS,
			cfg.Architecture,
			supportedImageOS,
			supportedImageArch,
		)
	}

	return nil
}

// parseExposedPorts reduces OCI's stringly-typed ExposedPorts map into sorted,
// unique numeric ports.
//
// OCI config stores ports as strings such as "8080/tcp". For later workload
// decisions we only need the numeric port list, while protocol-specific routing
// heuristics can stay as a separate concern.
func parseExposedPorts(exposed map[string]struct{}) ([]uint16, error) {
	if len(exposed) == 0 {
		return nil, nil
	}

	seen := make(map[uint16]struct{}, len(exposed))
	ports := make([]uint16, 0, len(exposed))
	// Normalize "<port>/<proto>" keys, reject invalid values, and collapse
	// duplicate numeric ports so callers can reason about one port list.
	for raw := range exposed {
		portText, _, _ := strings.Cut(strings.TrimSpace(raw), "/")
		if portText == "" {
			return nil, fmt.Errorf("invalid exposed port %q", raw)
		}

		portValue, err := strconv.ParseUint(portText, 10, 16)
		if err != nil || portValue == 0 {
			return nil, fmt.Errorf("invalid exposed port %q", raw)
		}

		port := uint16(portValue)
		if _, exists := seen[port]; exists {
			continue
		}

		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	// Keep the result deterministic so later routing and tests do not depend on
	// Go's random map iteration order.
	sort.Slice(ports, func(i, j int) bool {
		return ports[i] < ports[j]
	})

	return ports, nil
}

// envSliceToMap converts OCI's repeated KEY=VALUE environment list into a map.
//
// The launch contract and later artifact/runtime planning code want direct key
// lookup more than original ordering. Repeated keys keep the last value so the
// result matches how later entries override earlier ones in practice.
func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, item := range env {
		key, value, hasValue := strings.Cut(item, "=")
		if key == "" {
			continue
		}
		if !hasValue {
			value = ""
		}

		out[key] = value
	}
	return out
}

// cloneStrings copies OCI string slices so resolved configs are isolated from
// any later mutation of the source config object.
func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	out := make([]string, len(in))
	copy(out, in)

	return out
}
