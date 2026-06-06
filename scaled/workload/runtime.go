//go:build linux

// Runtime is the workload subsystem boundary for one scaled node.
//
// Runtime owns the Bidder (auction handler), Executor (targeted launch
// handler), local Firecracker Launcher, and the periodic node heartbeat. The
// main daemon calls NewRuntime once with the values produced by node.Collect
// and Start once with the NATS client; from that point the workload boundary
// runs itself and stops when the caller's context is cancelled.
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

// Runtime is the root workload coordinator for one scaled process.
//
// It wires the local capacity model, NATS placement handlers, Firecracker
// launcher, and the periodic node heartbeat. The daemon does not need to
// know any of those internals.
type Runtime struct {
	logger   *slog.Logger
	info     node.Info
	bidder   *Bidder
	executor *Executor
	launcher *microvm.Launcher

	stopHeartbeat context.CancelFunc
}

// NewRuntime initializes the workload subsystem from values prepared by
// node.Collect during scaled startup.
//
// The golden image runtime means every path and identity value in info is
// already resolved and validated; NewRuntime just wires them together.
func NewRuntime(logger *slog.Logger, info node.Info) *Runtime {
	logger = logger.With("component", "workload")

	capacity := NewCapacity(info.Snapshot.TotalRAMMb, info.Snapshot.TotalCores)
	launcher := microvm.NewLauncher(logger, info.RuntimePaths, info.JailerIdentity)

	return &Runtime{
		logger:   logger,
		info:     info,
		bidder:   NewBidder(logger, capacity, info.Identity.NodeID, info.Snapshot.BootID, info.Identity.Region),
		executor: NewExecutor(logger, capacity, info.Snapshot.BootID, launcher),
		launcher: launcher,
	}
}

// Start boots the workload subsystem. It registers the NATS subscriptions that
// let this node bid in placement auctions and receive targeted launch commands,
// then begins the periodic node heartbeat.
//
// Keeping those subscriptions and the heartbeat behind Runtime lets the daemon
// start one workload component without knowing the internal NATS subjects,
// handler order, or heartbeat key shape.
func (r *Runtime) Start(ctx context.Context, nc *nats.Client) error {
	auctionSubject, err := r.bidder.Register(nc)
	if err != nil {
		return fmt.Errorf("register bidder: %w", err)
	}

	launchSubject, err := r.executor.Register(nc)
	if err != nil {
		return fmt.Errorf("register executor: %w", err)
	}

	if err := microvm.CleanupStaleState(); err != nil {
		return fmt.Errorf("cleanup stale microvm state: %w", err)
	}

	heartbeats, err := nc.EnsureNodeHeartbeatKV(ctx)
	if err != nil {
		return fmt.Errorf("init heartbeat kv: %w", err)
	}

	heartbeatCtx, cancel := context.WithCancel(ctx)
	r.stopHeartbeat = cancel
	go r.runHeartbeat(heartbeatCtx, heartbeats)

	r.logger.Info("workload runtime ready",
		"auction_subject", auctionSubject,
		"launch_subject", launchSubject,
		"node_id", r.info.Identity.NodeID,
		"region", r.info.Identity.Region,
	)

	return nil
}

// Stop halts the periodic node heartbeat. The NATS subscriptions registered
// in Start are torn down when the caller's context is cancelled, so Stop only
// owns the heartbeat lifecycle.
func (r *Runtime) Stop() {
	if r.stopHeartbeat != nil {
		r.stopHeartbeat()
	}
}

// runHeartbeat publishes the node heartbeat on a fixed interval and is the
// only place that talks to the heartbeat key value store. It is internal to
// Runtime so the daemon does not need to know the key shape or cadence.
func (r *Runtime) runHeartbeat(ctx context.Context, kv nats.KeyValue) {
	key := nats.NodeHeartbeatKey(r.info.Identity.NodeID)
	seqNo := uint64(1)

	r.publishHeartbeat(ctx, kv, key, seqNo)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seqNo++
			r.publishHeartbeat(ctx, kv, key, seqNo)
		}
	}
}

func (r *Runtime) publishHeartbeat(ctx context.Context, kv nats.KeyValue, key string, seqNo uint64) {
	hb := &pb.NodeHeartbeat{
		NodeId:         r.info.Identity.NodeID,
		SeqNo:          seqNo,
		BootId:         r.info.Snapshot.BootID,
		SentAtUnixNano: time.Now().UnixNano(),
	}

	if _, err := nats.PutProtoKV(ctx, kv, key, hb); err != nil {
		r.logger.Warn("heartbeat publish failed",
			"node_id", r.info.Identity.NodeID,
			"error", err,
		)
	}
}
