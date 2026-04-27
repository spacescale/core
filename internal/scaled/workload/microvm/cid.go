package microvm

import (
	"errors"
	"sync"
)

// firstGuestCID defines the lowest legally assignable virtio-vsock Context ID (CID)
// for a Firecracker microVM.
//
// In the Linux AF_VSOCK implementation, the CID space is strictly regulated:
// - CID 0: Reserved (Hypervisor internal)
// - CID 1: Reserved (Local loopback)
// - CID 2: Hardcoded to the physical host machine (the scaled daemon)
//
// Therefore, all guest microVM numbering MUST mathematically begin at 3 to
// avoid kernel-level socket binding collisions.
const firstGuestCID uint32 = 3

// lastGuestCID defines the absolute upper ceiling of the AF_VSOCK address space.
//
// By performing a bitwise NOT on 0 (^uint32(0)), we derive the maximum theoretical
// value of a 32-bit unsigned integer (4,294,967,295). If the CID allocator reaches
// this mathematical limit, it must wrap around and hunt for previously reclaimed
// CIDs starting back at firstGuestCID.
const lastGuestCID uint32 = ^uint32(0)

// errNoGuestCIDAvailable is a fatal exhaustion error. It indicates that the
// node has completely saturated its entire 32-bit vsock address space and cannot
// legally provision another host-to-guest tether.
//
// In practice, hitting this error on a single bare-metal node implies either
// billions of concurrently running microvms, or a severe memory leak in the
// allocator's lifecycle reclamation logic.
var errNoGuestCIDAvailable = errors.New("no guest vsock cid available")

// cidAllocator manages guest vsock CIDs for the current scaled process.
//
// It is intentionally in-memory. If scaled restarts, local microVMs are treated
// as invalid and stale local state should be cleaned before accepting launches.
type cidAllocator struct {
	mu   sync.Mutex
	next uint32
	used map[uint32]struct{}
}

// newCIDAllocator creates the simple in-memory allocator used by the launcher.
func newCIDAllocator() *cidAllocator {
	return &cidAllocator{
		next: firstGuestCID,
		used: make(map[uint32]struct{}),
	}
}

// Acquire reserves the next free guest CID.
//
// The allocator walks upward from the current next pointer and wraps when it
// reaches the end of the uint32 range. Released lower CIDs can be reused later.
func (a *cidAllocator) Acquire() (uint32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If allocation wraps back to start, the whole guest CID space is occupied.
	start := a.next

	for {
		cid := a.next

		if cid < firstGuestCID {
			cid = firstGuestCID
			a.next = cid
		}

		if _, exists := a.used[cid]; !exists {
			a.used[cid] = struct{}{}
			a.advanceLocked()
			return cid, nil
		}

		a.advanceLocked()

		if a.next == start {
			return 0, errNoGuestCIDAvailable
		}
	}
}

// Release returns a previously reserved CID back to the allocator.
//
// Releasing an unknown CID is harmless. That keeps cleanup paths simpler and
// makes defensive release calls safe during launch failure handling.
func (a *cidAllocator) Release(cid uint32) {
	if cid < firstGuestCID {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.used, cid)
	// Prefer reusing lower released CIDs sooner so the allocator does not drift
	// upward forever during repeated create and destroy cycles.
	if cid < a.next {
		a.next = cid
	}
}

// advanceLocked shifts the allocation pointer forward, handling integer wrap-around.
func (a *cidAllocator) advanceLocked() {
	if a.next == lastGuestCID {
		a.next = firstGuestCID
		return
	}

	a.next++
	if a.next < firstGuestCID {
		a.next = firstGuestCID
	}
}
