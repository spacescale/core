//go:build linux

package scaled

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spacescale/core/internal/scaled/node"
	"github.com/spacescale/core/internal/scaled/runtime"
	"github.com/spacescale/core/internal/scaled/system"
	"github.com/spacescale/core/internal/scaled/workload"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/spacescale/core/internal/shared/nats"
	"golang.org/x/sync/errgroup"
)

type Daemon struct {
	cfg    config.Config
	logger *slog.Logger
	nats   *nats.Client
	assets runtime.Paths
}

func New(cfg config.Config, logger *slog.Logger) (*Daemon, error) {
	var err error
	cfg, err = cfg.ValidateScaled()
	if err != nil {
		return nil, fmt.Errorf("invalid scaled config: %w", err)
	}

	logger = logger.With("component", "scaled")

	natsClient, err := nats.New(cfg.NATSURL, "scaled", logger)
	if err != nil {
		return nil, fmt.Errorf("nats init failed: %w", err)
	}

	return &Daemon{cfg: cfg, logger: logger, nats: natsClient}, nil
}

func (d *Daemon) Run(ctx context.Context) error {

	if err := system.Preflight(d.logger); err != nil {
		return err
	}

	// Reconcile runtime assets before the node is allowed to become ready.
	//
	// This is the startup boundary where scaled proves that the local host now
	// has every file it will need for the first Firecracker launch path.
	// If any required asset is missing or broken, startup stops here and the
	// node never joins bootstrap readiness or workload handling.
	resolver := runtime.NewResolver(d.logger)
	assets, err := resolver.Reconcile(ctx)
	if err != nil {
		return fmt.Errorf("reconcile runtime assets: %w", err)
	}

	d.assets = assets

	snapshot, identity, err := node.Bootstrap(ctx, d.nats)
	if err != nil {
		return err
	}
	d.logger.Info("scaled ready", "node_id", identity.NodeID, "region", identity.Region)

	heartbeats, err := d.nats.EnsureNodeHeartbeatKV(ctx)
	if err != nil {
		return fmt.Errorf("init heartbeat kv: %w", err)
	}

	manager, err := workload.NewManager(
		d.logger,
		assets,
		snapshot.TotalRamMb,
		snapshot.TotalCores,
		identity.NodeID,
		snapshot.BootID,
		identity.Region,
	)
	if err != nil {
		return fmt.Errorf("create workload manager: %w", err)
	}
	if err := manager.Start(d.nats); err != nil {
		return fmt.Errorf("start workload manager: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		node.Heartbeater(gCtx, heartbeats, identity.NodeID, snapshot.BootID, d.logger)
		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("run heartbeat loop: %w", err)
	}
	d.logger.Info("scaled stopped", "node_id", identity.NodeID)
	return nil
}

func (d *Daemon) Close() error {
	if err := d.nats.Drain(); err != nil {
		return fmt.Errorf("nats drain failed: %w", err)
	}
	return nil
}
