//go:build linux

// Package workload is the workload subsystem boundary for one scaled node.
//
// Start wires the bidder, launch handler, local Firecracker launcher, and
// periodic node heartbeat. The main daemon hands it the values
// produced by node.Collect and the process context; from that point the
// workload boundary runs itself until the caller cancels the context.
package workload

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spacescale/core/scaled/node"
	"github.com/spacescale/core/scaled/workload/microvm"
	"github.com/spacescale/core/shared/nats"
	"github.com/spacescale/core/shared/pb/v1"
)

const heartbeatInterval = 5 * time.Second

// Start initializes the workload subsystem from values prepared by
// node.Collect during scaled startup.
//
// The golden image runtime means every path and identity value in info is
// already resolved and validated, so Start just wires the handlers, launcher,
// and heartbeat together behind one package boundary.
func Start(ctx context.Context, logger *slog.Logger, info node.Info, nc *nats.Client) error {
	workloadLog := logger.With("component", "workload")

	capacity := NewCapacity(info.Snapshot.TotalRAMMb, info.Snapshot.TotalThreads)
	microvmLauncher := microvm.NewLauncher(logger, info.RuntimePaths, info.JailerIdentity)
	bidder := NewBidder(workloadLog, capacity, info.Identity.NodeID, info.Snapshot.BootID, info.Identity.Region)
	executor := newExecutor(workloadLog, capacity, info.Snapshot.BootID, microvmLauncher)

	if _, err := bidder.Register(ctx, nc); err != nil {
		return fmt.Errorf("register bidder: %w", err)
	}

	if _, err := executor.register(ctx, nc); err != nil {
		return fmt.Errorf("register launch handler: %w", err)
	}

	if err := microvm.CleanupStaleState(); err != nil {
		return fmt.Errorf("cleanup stale microvm state: %w", err)
	}

	heartbeats, err := nc.EnsureNodeHeartbeatKV(ctx)
	if err != nil {
		return fmt.Errorf("init heartbeat kv: %w", err)
	}

	go runHeartbeat(ctx, workloadLog, info, heartbeats)

	workloadLog.Info("workload ready",
		"node_id", info.Identity.NodeID,
		"region", info.Identity.Region,
	)

	return nil
}

// runHeartbeat publishes the node heartbeat on a fixed interval and is the
// only place that talks to the heartbeat key value store. It stays package
// internal so scaled does not need to know the key shape or cadence.
func runHeartbeat(ctx context.Context, logger *slog.Logger, info node.Info, kv nats.KeyValue) {
	key := nats.NodeHeartbeatKey(info.Identity.NodeID)
	seqNo := uint64(1)

	publishHeartbeat(ctx, logger, info, kv, key, seqNo)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seqNo++
			publishHeartbeat(ctx, logger, info, kv, key, seqNo)
		}
	}
}

func publishHeartbeat(ctx context.Context, logger *slog.Logger, info node.Info, kv nats.KeyValue, key string, seqNo uint64) {
	hb := &pb.NodeHeartbeat{
		NodeId:         info.Identity.NodeID,
		SeqNo:          seqNo,
		BootId:         info.Snapshot.BootID,
		SentAtUnixNano: time.Now().UnixNano(),
	}

	if _, err := nats.PutProtoKV(ctx, kv, key, hb); err != nil {
		logger.Warn("heartbeat publish failed",
			"node_id", info.Identity.NodeID,
			"error", err,
		)
	}
}
