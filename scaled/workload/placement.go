// Package workload runs microVM placement, local capacity accounting, and
// targeted launch handling for one scaled node.
//
// The main daemon should not need to know how placement auctions, capacity
// reservations, targeted launch commands, or local Firecracker execution are
// wired together. Runtime owns those subsystem bindings and the periodic node
// heartbeat. The shape policy, capacity ledger, and bidder all live in this
// file because they are tightly coupled: shape drives what the ledger can sell,
// and the bidder is the only place that calls Reserve.
package workload

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/spacescale/core/shared/nats"
	pb "github.com/spacescale/core/shared/pb/v1"
)

// reservationTTL defines the maximum time the node will hold physical resources
// in reserve while waiting for a Launch command before assuming it lost the auction.
const reservationTTL = 500 * time.Millisecond

// reservation holds a temporary claim on node resources while the control plane
// decides whether this node won the auction.
type reservation struct {
	RAMMB     uint64
	VCPU      uint32
	IsPinned  bool
	ExpiresAt time.Time
}

// ErrInvalidMicroVMShape is returned when the control plane sends a shape the
// edge cannot execute.
var ErrInvalidMicroVMShape = errors.New("invalid microvm shape")

// HardwareSpec is the capacity-relevant slice of one resolved microVM shape.
type HardwareSpec struct {
	VCPU uint32
	RAM  uint64

	IsPinned bool
}

const (
	// sharedVCPUOvercommitRatio defines how many shared guest vCPUs may be sold
	// per remaining physical host core. This ratio applies only to the shared
	// pool after host reserved cores and dedicated pinned workloads have already
	// been removed from the node capacity.
	//
	// Since SpaceScale operates on a Physical Core Truth model with SMT disabled,
	// a ratio of 4 ensures that even under maximum contention, each virtual CPU
	// is guaranteed at least 25% of a physical core's execution time. This balances
	// high-density multi-tenancy with deterministic, "snappy" performance.
	sharedVCPUOvercommitRatio uint32 = 4
	// Nodes up to and including this size reserve one host core.
	singleHostReserveCoreCeiling uint32 = 64
	// Smaller nodes keep one core for the host. Very large nodes keep two.
	baseHostReservedCores  uint32 = 1
	largeHostReservedCores uint32 = 2
)

const (
	// Node RAM thresholds in Megabytes. These determine the progressive host
	// memory tax applied before any customer workload capacity is exposed.
	node32GBMB uint64 = 32768
	node64GBMB uint64 = 65536
	// hostTax constants define the exact amount of RAM in Megabytes that is
	// withheld from the customer workload pool. This pays for the Linux kernel,
	// system services, the scaled daemon, network buffers, and microVM overhead.
	hostTax32GBMB     uint64 = 3686
	hostTax64GBMB     uint64 = 5324
	hostTaxOver64GBMB uint64 = 8601
)

func SpecFromShape(shape *pb.MicroVMShape) (HardwareSpec, error) {
	if shape == nil || shape.Vcpu == 0 || shape.RamMb == 0 {
		return HardwareSpec{}, ErrInvalidMicroVMShape
	}

	spec := HardwareSpec{VCPU: shape.Vcpu, RAM: shape.RamMb}
	switch shape.CpuMode {
	case pb.CpuMode_CPU_MODE_SHARED:
		return spec, nil
	case pb.CpuMode_CPU_MODE_PINNED:
		spec.IsPinned = true
		return spec, nil
	default:
		return HardwareSpec{}, ErrInvalidMicroVMShape
	}
}

func CpuModeLogValue(shape *pb.MicroVMShape) string {
	if shape == nil {
		return "unspecified"
	}
	if shape.CpuMode == pb.CpuMode_CPU_MODE_PINNED {
		return "pinned"
	}
	if shape.CpuMode == pb.CpuMode_CPU_MODE_SHARED {
		return "shared"
	}
	return "unspecified"
}

// hostReservedCores returns how many physical cores must always remain with the
// host operating system and control processes.
//
// The current policy is:
//
// 1. Nodes with up to 64 physical cores reserve 1 host core.
// 2. Nodes above 64 physical cores reserve 2 host cores.
//
// This policy assumes routing and other heavier data plane work is offloaded
// elsewhere, so the node host only needs a small protected CPU slice for
// scaled, systemd, tap and bridge work, and basic operating system activity.
func hostReservedCores(total uint32) uint32 {
	switch {
	case total == 0:
		return 0
	case total > singleHostReserveCoreCeiling:
		return largeHostReservedCores
	default:
		return baseHostReservedCores
	}
}

// sellableCores calculates the final pool of physical host cores available for
// customer workloads after the host reserve is removed.
//
// Callers pass the true physical core count discovered by host preflight, not
// logical thread counts.
func sellableCores(total uint32) uint32 {
	reserved := hostReservedCores(total)
	if total <= reserved {
		return 0
	}
	return total - reserved
}

// hostTaxRAMMB determines the exact amount of RAM in Megabytes that must be
// withheld from the workload pool based on the physical size of the server.
// Larger servers run more microVMs and require a larger networking and
// file descriptor buffer in the kernel.
func hostTaxRAMMB(total uint64) uint64 {
	switch {
	case total <= node32GBMB:
		return hostTax32GBMB
	case total <= node64GBMB:
		return hostTax64GBMB
	default:
		return hostTaxOver64GBMB
	}
}

// sellableRAMMB calculates the final pool of RAM available for customer workloads
// by subtracting the host tax from the total physical RAM.
func sellableRAMMB(total uint64) uint64 {
	tax := hostTaxRAMMB(total)
	if tax >= total {
		return 0
	}
	return total - tax
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

// Bidder handles incoming placement auctions from the control plane.
type Bidder struct {
	logger   *slog.Logger
	capacity *Capacity
	nodeID   string
	bootID   string
	region   string
}

// NewBidder wires the NATS transport to the local capacity ledger.
func NewBidder(logger *slog.Logger, c *Capacity, nodeID, bootID, region string) *Bidder {
	return &Bidder{
		logger:   logger,
		capacity: c,
		nodeID:   nodeID,
		bootID:   bootID,
		region:   region,
	}
}

// Register connects the bidder to the NATS regional broadcast subject.
// This allows the orchestrator to explicitly control when the node begins
// accepting workloads.
func (b *Bidder) Register(client *nats.Client) (string, error) {
	subject := nats.NodeAuctionSubject(b.region)
	_, err := client.Subscribe(subject, func(msg *nats.Msg) error {
		return b.handle(client, msg)
	})
	if err != nil {
		return "", err
	}
	return subject, nil
}

func (b *Bidder) handle(client *nats.Client, msg *nats.Msg) error {
	if msg.Reply == "" {
		return errors.New("auction request missing reply subject")
	}

	var req pb.AuctionRequest
	if err := nats.UnmarshalProto(msg, &req); err != nil {
		return err
	}
	if req.MicrovmId == "" {
		return errors.New("auction request missing microvm id")
	}

	spec, err := SpecFromShape(req.Shape)
	if err != nil {
		return err
	}

	freeRAM, ok := b.capacity.Reserve(req.MicrovmId, spec, reservationTTL)
	if !ok {
		return nil
	}

	b.logger.Info(
		"submitted bid",
		"microvm_id", req.MicrovmId,
		"vcpu", req.GetShape().GetVcpu(),
		"ram_mb", req.GetShape().GetRamMb(),
		"cpu_mode", CpuModeLogValue(req.GetShape()),
		"free_ram_mb", freeRAM,
	)
	_ = freeRAM

	reply := &pb.AuctionReply{
		NodeId: b.nodeID,
		BootId: b.bootID,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		b.capacity.Release(req.MicrovmId)
		return err
	}

	return nil
}
