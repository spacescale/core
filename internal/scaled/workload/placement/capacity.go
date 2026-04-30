package placement

import (
	"sync"
	"time"
)

// reservation holds a temporary claim on node resources while the control plane
// decides whether this node won the auction.
type reservation struct {
	RAMMB     uint64
	VCPU      uint32
	IsPinned  bool
	ExpiresAt time.Time
}

// Capacity tracks the hardware allocation state of the edge node.
// It keeps dedicated core accounting separate from the shared vcpu pool.
type Capacity struct {
	mu sync.Mutex

	// RAM Allocation (Megabytes)
	sellableRAMMB uint64
	usedRAMMB     uint64
	reservedRAMMB uint64

	// Dedicated host core allocation.
	sellableCores       uint32
	usedPinnedCores     uint32
	reservedPinnedCores uint32

	// Shared CPU Allocation (Overcommitted Virtual CPUs)
	usedSharedVCPU     uint32
	reservedSharedVCPU uint32

	// In flight reservations keyed by microvm id.
	reservations map[string]reservation
}

// NewCapacity initializes the node resource ledger from real host metrics.
func NewCapacity(totalRAMMB uint64, totalCores uint32) *Capacity {
	return &Capacity{
		sellableRAMMB: sellableRAMMB(totalRAMMB),
		sellableCores: sellableCores(totalCores),
		reservations:  make(map[string]reservation),
	}
}

// Reserve holds resources for a pending auction bid.
//
// It returns the remaining free RAM which acts as the auction tiebreaker
// and true if successful. If a reservation for the microvm id already exists
// or if there is insufficient physical capacity it returns false.
func (c *Capacity) Reserve(microvmID string, spec HardwareSpec, ttl time.Duration) (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.releaseExpiredLocked(now)

	if _, exists := c.reservations[microvmID]; exists {
		return 0, false
	}

	if !c.canFitLocked(spec) {
		return 0, false
	}

	c.reservations[microvmID] = reservation{
		RAMMB:     spec.RAM,
		VCPU:      spec.VCPU,
		IsPinned:  spec.IsPinned,
		ExpiresAt: now.Add(ttl),
	}

	c.reservedRAMMB += spec.RAM
	if spec.IsPinned {
		c.reservedPinnedCores += spec.VCPU
	} else {
		c.reservedSharedVCPU += spec.VCPU
	}

	return c.freeRAMMBLocked(), true
}

// Commit moves a reservation into permanent usage. This is called when
// the Control Plane explicitly awards the workload to this node via a Launch command.
func (c *Capacity) Commit(microvmID string) (HardwareSpec, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.releaseExpiredLocked(time.Now())

	res, ok := c.reservations[microvmID]
	if !ok {
		return HardwareSpec{}, false
	}

	delete(c.reservations, microvmID)

	c.reservedRAMMB -= res.RAMMB
	c.usedRAMMB += res.RAMMB

	if res.IsPinned {
		c.reservedPinnedCores -= res.VCPU
		c.usedPinnedCores += res.VCPU
	} else {
		c.reservedSharedVCPU -= res.VCPU
		c.usedSharedVCPU += res.VCPU
	}

	return HardwareSpec{
		VCPU:     res.VCPU,
		RAM:      res.RAMMB,
		IsPinned: res.IsPinned,
	}, true
}

func (c *Capacity) Revert(spec HardwareSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.usedRAMMB >= spec.RAM {
		c.usedRAMMB -= spec.RAM
	} else {
		c.usedRAMMB = 0
	}

	if spec.IsPinned {
		if c.usedPinnedCores >= spec.VCPU {
			c.usedPinnedCores -= spec.VCPU
		} else {
			c.usedPinnedCores = 0
		}
		return
	}

	if c.usedSharedVCPU >= spec.VCPU {
		c.usedSharedVCPU -= spec.VCPU
	} else {
		c.usedSharedVCPU = 0
	}
}

// Release manually drops a temporary hold. This is called if a node bids on an
// auction but the network explicitly fails immediately bypassing the TTL.
func (c *Capacity) Release(microvmID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseLocked(microvmID)
}

// ReleaseExpired sweeps the map for any dead temporary holds.
func (c *Capacity) ReleaseExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseExpiredLocked(now)
}

// FreeRAMMB returns the unallocated RAM without mutating state.
func (c *Capacity) FreeRAMMB() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.freeRAMMBLocked()
}

func (c *Capacity) canFitLocked(spec HardwareSpec) bool {
	if c.freeRAMMBLocked() < spec.RAM {
		return false
	}

	if spec.IsPinned {
		return c.freePinnedCoresLocked() >= spec.VCPU
	}

	return c.freeSharedVCPULocked() >= spec.VCPU
}

func (c *Capacity) freeRAMMBLocked() uint64 {
	allocated := c.usedRAMMB + c.reservedRAMMB
	if allocated > c.sellableRAMMB {
		return 0
	}
	return c.sellableRAMMB - allocated
}

func (c *Capacity) freePinnedCoresLocked() uint32 {
	allocated := c.usedPinnedCores + c.reservedPinnedCores
	if allocated > c.sellableCores {
		return 0
	}
	return c.sellableCores - allocated
}

// freeSharedVCPULocked calculates available virtual capacity.
// It subtracts dedicated physical cores from the total pool then multiplies
// the remaining cores by the overcommit ratio. It then subtracts currently
// allocated shared virtual CPUs to find the remaining pool.
func (c *Capacity) freeSharedVCPULocked() uint32 {
	dedicatedCores := c.usedPinnedCores + c.reservedPinnedCores
	if dedicatedCores > c.sellableCores {
		return 0
	}
	sharedCores := c.sellableCores - dedicatedCores

	sharedCapacity := sharedCores * sharedVCPUOvercommitRatio

	allocatedShared := c.usedSharedVCPU + c.reservedSharedVCPU
	if allocatedShared > sharedCapacity {
		return 0
	}

	return sharedCapacity - allocatedShared
}

// releaseExpiredLocked sweeps the active reservations and deletes any that have passed their TTL.
// It assumes the caller holds the mutex.
func (c *Capacity) releaseExpiredLocked(now time.Time) {
	for microvmID, res := range c.reservations {
		if now.After(res.ExpiresAt) {
			c.releaseLocked(microvmID)
		}
	}
}

// releaseLocked drops a temporary hold and returns the capacity to the free pool.
// It assumes the caller holds the mutex.
func (c *Capacity) releaseLocked(microvmID string) {
	res, ok := c.reservations[microvmID]
	if !ok {
		return
	}

	delete(c.reservations, microvmID)

	c.reservedRAMMB -= res.RAMMB
	if res.IsPinned {
		c.reservedPinnedCores -= res.VCPU
	} else {
		c.reservedSharedVCPU -= res.VCPU
	}
}
