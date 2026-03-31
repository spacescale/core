// Package workload provides the top-level orchestration for all microVM
// operations on the edge node.
//
// This package acts as a strict Facade. It hides the internal complexities
// of capacity ledgers, NATS auction bidding, network bridge creation, and
// Firecracker execution from the main scaled daemon.
package workload

import (
	"log/slog"

	"github.com/spacescale/core/internal/scaled/workload/placement"
	"github.com/spacescale/core/internal/shared/nats"
)

// Manager is the root orchestrator for the edge node's workload lifecycle.
// It initializes and binds together the placement engine, execution engine,
// and local hardware state.
type Manager struct {
	logger   *slog.Logger
	capacity *placement.Capacity
	bidder   *placement.Bidder
	executor *placement.Executor
}

// NewManager initializes the workload boundaries using real hardware metrics
// discovered during node boot.
func NewManager(logger *slog.Logger, totalRAM uint64, totalThreads uint32, nodeID, bootID, region string) *Manager {
	cap := placement.NewCapacity(totalRAM, totalThreads)

	return &Manager{
		logger:   logger,
		capacity: cap,
		bidder:   placement.NewBidder(logger, cap, nodeID, bootID, region),
		executor: placement.NewExecutor(logger, cap, bootID),
	}
}

// Start boots the workload subsystem. It registers all necessary NATS
// subscriptions and begins accepting placement auctions from the Control Plane.
//
// By centralizing this initialization, the main daemon remains completely
// ignorant of the underlying NATS subjects and internal workload handlers.
func (m *Manager) Start(nc *nats.Client) error {
	m.logger.Info("starting workload manager")

	if err := m.bidder.Register(nc); err != nil {
		return err
	}

	if err := m.executor.Register(nc); err != nil {
		return err
	}

	return nil
}