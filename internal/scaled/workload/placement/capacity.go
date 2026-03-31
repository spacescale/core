// Package placement provides the local execution and placement engine for microVMs.
//
// This file implements the Capacity ledger which tracks the real time physical
// resources of the bare metal node. It uses optimistic concurrency control
// via temporary reservations to prevent scheduling race conditions such as OOM
// during decentralized NATS auctions.
//
// The Capacity struct is strictly isolated from network transports. It relies on
// fast atomic memory mutations and lazy garbage collection to handle thousands of
// concurrent allocation requests without deadlocking.
package placement

import (
	"sync"
	"time"
)

// sharedVCPUOvercommitRatio dictates the economic and performance model for shared
// compute tiers. It represents how many virtual CPUs can be sold per physical hardware
// thread. A ratio of 4 means 4 virtual CPUs map to 1 physical thread.
//
// This overcommit relies on the statistical probability that most shared workloads
// are idle most of the time allowing the Linux kernel Completely Fair Scheduler
// to rapidly time slice the physical CPU between active workloads. If too many
// virtual CPUs become active simultaneously it causes CPU steal and thrashing.
// A ratio of 4 to 6 is the industry standard sweet spot for general purpose web traffic.
const sharedVCPUOvercommitRatio uint32 = 4

// reservation represents a temporary hold on physical resources while a node
// waits to see if it won a placement auction.
type reservation struct {
	RAMMB     uint64
	VCPU      uint32
	IsPinned  bool
	ExpiresAt time.Time
}

// Capacity tracks the hardware allocation state of the edge daemon.
// It manages the split between dedicated resources and overcommitted shared resources.
type Capacity struct {
	mu                    sync.Mutex
	totalRAMMB            uint64
	usedRAMMB             uint64
	reservedRAMMB         uint64
	totalThreads          uint32
	usedPinnedThreads     uint32
	reservedPinnedThreads uint32
	usedSharedVCPU        uint32
	reservedSharedVCPU    uint32
	reservations          map[string]reservation
}

// NewCapacity initializes the node resource ledger using real hardware metrics.
func NewCapacity(totalRAMMB uint64, totalThreads uint32) *Capacity {
	return &Capacity{
		totalRAMMB:   totalRAMMB,
		totalThreads: totalThreads,
		reservations: make(map[string]reservation),
	}
}

// Reserve optimistically holds resources for a pending auction bid.
// It is mathematically atomic. It checks capacity and deducts it under the
// same lock to prevent Time Of Check to Time Of Use race conditions.
//
// It returns the remaining free RAM which acts as the auction tie breaker
// and true if successful. If a reservation for the machineID already exists
// or if there is insufficient physical capacity it returns false.
func (c *Capacity) Reserve(machineID string, spec HardwareSpec, ttl time.Duration) (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	// Lazy Garbage Collection. Clear out old reservations before making capacity decisions
	// This prevents the need for a separate background ticker goroutine.
	c.releaseExpiredLocked(now)

	// Idempotency check. If we already reserved this machine we cannot reserve it again.
	if _, exists := c.reservations[machineID]; exists {
		return 0, false
	}

	// Fail Fast. Ensure we have enough physical room before mutating anything.
	if !c.canFitLocked(spec) {
		return 0, false
	}

	c.reservations[machineID] = reservation{
		RAMMB:     spec.RAM,
		VCPU:      spec.VCPU,
		IsPinned:  spec.IsPinned,
		ExpiresAt: now.Add(ttl),
	}

	c.reservedRAMMB += spec.RAM
	if spec.IsPinned {
		c.reservedPinnedThreads += spec.VCPU
	} else {
		c.reservedSharedVCPU += spec.VCPU
	}

	return c.freeRAMMBLocked(), true
}

// Commit moves a reservation into permanent usage. This is called when
// the Control Plane explicitly awards the workload to this node via a Launch command.
func (c *Capacity) Commit(machineID string) (HardwareSpec, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.releaseExpiredLocked(time.Now())

	res, ok := c.reservations[machineID]
	if !ok {
		return HardwareSpec{}, false
	}

	delete(c.reservations, machineID)

	c.reservedRAMMB -= res.RAMMB
	c.usedRAMMB += res.RAMMB

	if res.IsPinned {
		c.reservedPinnedThreads -= res.VCPU
		c.usedPinnedThreads += res.VCPU
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
		if c.usedPinnedThreads >= spec.VCPU {
			c.usedPinnedThreads -= spec.VCPU
		} else {
			c.usedPinnedThreads = 0
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
func (c *Capacity) Release(machineID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseLocked(machineID)
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
		return c.freePinnedThreadsLocked() >= spec.VCPU
	}

	return c.freeSharedVCPULocked() >= spec.VCPU
}

func (c *Capacity) freeRAMMBLocked() uint64 {
	allocated := c.usedRAMMB + c.reservedRAMMB
	if allocated > c.totalRAMMB {
		return 0 // Defensive return preventing catastrophic uint64 underflow
	}
	return c.totalRAMMB - allocated
}

func (c *Capacity) freePinnedThreadsLocked() uint32 {
	allocated := c.usedPinnedThreads + c.reservedPinnedThreads
	if allocated > c.totalThreads {
		return 0 // Defensive return preventing catastrophic uint32 underflow
	}
	return c.totalThreads - allocated
}

// freeSharedVCPULocked calculates available virtual capacity.
// It subtracts dedicated physical threads from the total pool then multiplies
// the remaining threads by the overcommit ratio. It then subtracts currently
// allocated shared virtual CPUs to find the remaining pool.
func (c *Capacity) freeSharedVCPULocked() uint32 {
	dedicatedThreads := c.usedPinnedThreads + c.reservedPinnedThreads
	if dedicatedThreads > c.totalThreads {
		return 0 // Defensive guard
	}
	sharedThreads := c.totalThreads - dedicatedThreads

	sharedCapacity := sharedThreads * sharedVCPUOvercommitRatio

	allocatedShared := c.usedSharedVCPU + c.reservedSharedVCPU
	if allocatedShared > sharedCapacity {
		return 0 // Defensive return preventing catastrophic uint32 underflow
	}

	return sharedCapacity - allocatedShared
}

func (c *Capacity) releaseExpiredLocked(now time.Time) {
	for machineID, res := range c.reservations {
		if now.After(res.ExpiresAt) {
			c.releaseLocked(machineID)
		}
	}
}

func (c *Capacity) releaseLocked(machineID string) {
	res, ok := c.reservations[machineID]
	if !ok {
		return
	}

	delete(c.reservations, machineID)

	c.reservedRAMMB -= res.RAMMB
	if res.IsPinned {
		c.reservedPinnedThreads -= res.VCPU
	} else {
		c.reservedSharedVCPU -= res.VCPU
	}
}
