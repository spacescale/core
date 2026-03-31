// Package placement manages the local execution of microVMs on the bare-metal node.
//
// This file defines the physical translation of logical Control Plane Tiers
// into exact hardware allocations.
package placement

import (
	"errors"

	"github.com/spacescale/core/internal/shared/pb/v1"
)

var ErrUnknownTier = errors.New("unknown workload tier")

// HardwareSpec represents the physical resources required to boot a single microVM.
type HardwareSpec struct {
	VCPU uint32
	RAM  uint64 // in Megabytes

	// IsPinned determines if the vCPUs must be locked to dedicated physical
	// host threads, or if they can float in the shared overprovisioned pool.
	IsPinned bool
}

func TranslateTier(tier pb.Tier) (HardwareSpec, error) {
	switch tier {
	case pb.Tier_TIER_STARTER:
		return HardwareSpec{
			VCPU:     2,
			RAM:      4096,  // 4GB
			IsPinned: false, // starter VMS share physical threads
		}, nil

	case pb.Tier_TIER_GROWTH:
		return HardwareSpec{
			VCPU:     4,
			RAM:      8192,  // 8 GB
			IsPinned: false, //
		}, nil

	case pb.Tier_TIER_SCALE:
		return HardwareSpec{
			VCPU:     8,
			RAM:      16384,
			IsPinned: true,
		}, nil

	default:
		// We return an error if the CP sends us a tier we don't understand
		return HardwareSpec{}, ErrUnknownTier
	}
}
