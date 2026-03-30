//go:build linux

package scaled

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spacescale/core/internal/scaled/node"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/spacescale/core/internal/shared/nats"
	"golang.org/x/sync/errgroup"
)

const defaultDaemonVersion = "dev" // stub for now needs a standard daemon version from go releaser later

type Daemon struct {
	cfg    config.Config
	logger *slog.Logger
	nats   *nats.Client
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Daemon, error) {
	var err error
	cfg, err = cfg.ValidateScaled()
	if err != nil {
		return nil, fmt.Errorf("invalid scaled config: %w", err)
	}

	natsClient, err := nats.New(cfg.NATSURL, "scaled", logger)
	if err != nil {
		return nil, fmt.Errorf("nats init failed: %w", err)
	}

	return &Daemon{cfg: cfg, logger: logger, nats: natsClient}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	bootID, identity, err := node.Bootstrap(ctx, d.nats, defaultDaemonVersion)
	if err != nil {
		return err
	}
	d.logger.Info("scaled ready", "component", "scaled", "node_id", identity.NodeID, "region", identity.Region)

	heartbeats, err := d.nats.EnsureNodeHeartbeatKV(ctx)
	if err != nil {
		return fmt.Errorf("init heartbeat kv: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		node.Heartbeater(gCtx, heartbeats, identity.NodeID, bootID, d.logger)
		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("run heartbeat loop: %w", err)
	}
	d.logger.Info("scaled stopped", "component", "scaled", "node_id", identity.NodeID)
	return nil
}

func (d *Daemon) Close() error {
	if err := d.nats.Drain(); err != nil {
		return fmt.Errorf("nats drain failed: %w", err)
	}
	return nil
}
