//go:build linux

// Package scaled starts the Linux edge daemon and wires startup subsystems.
//
// The package validates config, runs host preflight, loads node identity, and
// starts workload handling. It should orchestrate only; subsystem internals stay
// in node and workload.
package scaled

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spacescale/core/scaled/node"
	"github.com/spacescale/core/scaled/workload"
	"github.com/spacescale/core/shared/config"
	"github.com/spacescale/core/shared/logger"
	"github.com/spacescale/core/shared/nats"
)

// Run starts the scaled edge daemon and blocks until the context is canceled or a startup/runtime error occurs.
func Run(ctx context.Context) error {
	cfg, err := config.LoadScaled()
	if err != nil {
		return fmt.Errorf("load scaled config: %w", err)
	}

	log := logger.Init(cfg.Environment).With("component", "scaled")

	natsClient, err := nats.New(cfg.NATSURL, "scaled", log)
	if err != nil {
		return fmt.Errorf("nats init failed: %w", err)
	}
	defer func() {
		if err := natsClient.Drain(); err != nil {
			log.Warn("nats drain failed", "component", "scaled", "error", err)
		}
	}()

	return runDaemon(ctx, cfg, log, natsClient)
}

func runDaemon(ctx context.Context, cfg config.Config, log *slog.Logger, natsClient *nats.Client) error {
	runtimePaths, err := node.ValidateRuntimePaths(cfg.FirecrackerBin, cfg.JailerBin, cfg.GuestKernelPath, cfg.GuestRootFSPath)
	if err != nil {
		return err
	}

	jailerIdentity, err := node.Preflight(log)
	if err != nil {
		return err
	}

	snapshot, err := node.Read()
	if err != nil {
		return err
	}
	identity, err := node.LoadIdentity()
	if err != nil {
		return fmt.Errorf("load node identity: %w", err)
	}

	heartbeats, err := natsClient.EnsureNodeHeartbeatKV(ctx)
	if err != nil {
		return fmt.Errorf("init heartbeat kv: %w", err)
	}

	workloadRuntime, err := workload.NewRuntime(
		log,
		runtimePaths,
		jailerIdentity,
		snapshot.TotalRAMMb,
		snapshot.TotalCores,
		identity.NodeID,
		snapshot.BootID,
		identity.Region,
	)
	if err != nil {
		return fmt.Errorf("create workload runtime: %w", err)
	}
	if err := workloadRuntime.Start(ctx, natsClient); err != nil {
		return fmt.Errorf("start workload runtime: %w", err)
	}

	go node.Heartbeater(ctx, heartbeats, identity, snapshot, log)

	<-ctx.Done()
	log.Info("scaled stopped", "node_id", identity.NodeID)

	return nil
}
