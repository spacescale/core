//go:build linux

// Package scaled starts the Linux edge daemon and wires startup subsystems.
//
// The package is a thin glue layer. It loads config, runs node.Collect to
// resolve runtime paths and host facts, then starts the workload subsystem.
// workload owns the internal bidding, launch, and heartbeat machinery; scaled
// itself only orchestrates startup and forwards the process context.
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

// Run starts the scaled edge daemon and blocks until the context is canceled
// or a startup/runtime error occurs.
func Run(ctx context.Context) error {
	cfg, err := config.LoadScaled()
	if err != nil {
		return fmt.Errorf("load scaled config: %w", err)
	}

	log := logger.Init(cfg.Environment)

	natsClient, err := nats.New(cfg.NATSURL, "scaled", log)
	if err != nil {
		return fmt.Errorf("nats init failed: %w", err)
	}
	defer func() { _ = natsClient.Drain() }()

	return runDaemon(ctx, log, natsClient)
}

func runDaemon(ctx context.Context, log *slog.Logger, natsClient *nats.Client) error {
	info, err := node.Collect(ctx, log)
	if err != nil {
		return fmt.Errorf("collect node info: %w", err)
	}
	if err := workload.Start(ctx, log, info, natsClient); err != nil {
		return fmt.Errorf("start workload: %w", err)
	}
	log.Info("scaled ready", "node_id", info.Identity.NodeID, "region", info.Identity.Region)
	<-ctx.Done()
	log.Info("scaled stopped", "node_id", info.Identity.NodeID)

	return nil
}
