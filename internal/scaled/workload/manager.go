// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

//go:build linux

// Package workload provides the top-level orchestration boundary for workload
// operations on one scaled node.
//
// The main daemon should not need to know how placement auctions, capacity
// reservations, targeted launch commands, or local Firecracker execution are
// wired together. Manager owns those subsystem bindings.
package workload

import (
	"fmt"
	"log/slog"

	scaledruntime "github.com/spacescale/core/internal/scaled/runtime"
	"github.com/spacescale/core/internal/scaled/system"
	"github.com/spacescale/core/internal/scaled/workload/executor"
	"github.com/spacescale/core/internal/scaled/workload/microvm"
	"github.com/spacescale/core/internal/scaled/workload/placement"
	"github.com/spacescale/core/internal/shared/nats"
)

// Manager is the root workload coordinator for one scaled process.
//
// It wires the local capacity model, NATS placement handlers, and Firecracker
// launcher used to turn accepted placements into local microVMs.
type Manager struct {
	logger   *slog.Logger
	bidder   *placement.Bidder
	executor *executor.Executor
}

// NewManager initializes the workload subsystem from values prepared during
// scaled startup.
func NewManager(
	logger *slog.Logger,
	assets scaledruntime.Paths,
	jailerIdentity system.FirecrackerJailerIdentity,
	totalRAM uint64,
	totalCores uint32,
	nodeID, bootID, region string,
) (*Manager, error) {
	capacity := placement.NewCapacity(totalRAM, totalCores)
	if err := microvm.CleanupStaleState(); err != nil {
		return nil, fmt.Errorf("cleanup stale microvm state: %w", err)
	}
	launcher := microvm.NewLauncher(logger, assets, jailerIdentity)
	return &Manager{
		logger:   logger,
		bidder:   placement.NewBidder(logger, capacity, nodeID, bootID, region),
		executor: executor.New(logger, capacity, bootID, launcher),
	}, nil
}

// Start boots the workload subsystem. It registers the NATS subscriptions that
// let this node bid in placement auctions and receive targeted launch commands.
//
// Keeping those subscriptions behind Manager lets the daemon start one workload
// component without knowing the internal NATS subjects or handler order.
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
