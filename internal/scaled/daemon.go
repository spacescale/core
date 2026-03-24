package scaled

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/spacescale/core/internal/scaled/node"
	"github.com/spacescale/core/internal/scaled/sysinfo"
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
	cfg = cfg.Normalized()
	if err := cfg.ValidateScaled(); err != nil {
		return nil, fmt.Errorf("invalid scaled config: %w", err)
	}

	natsClient, err := nats.New(cfg.NATSURL, "scaled", logger)
	if err != nil {
		return nil, fmt.Errorf("nats init failed: %w", err)
	}

	return &Daemon{cfg: cfg, logger: logger, nats: natsClient}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	bootID, err := sysinfo.ReadBootID()
	if err != nil {
		return fmt.Errorf("read boot id: %w", err)
	}

	memory, err := sysinfo.ReadMemoryStats()
	if err != nil {
		return fmt.Errorf("read memory stats: %w", err)
	}

	disk, err := sysinfo.ReadDiskStats("/")
	if err != nil {
		return fmt.Errorf("read disk stats: %w", err)
	}

	bootstrapInfo := node.BootstrapInfo{
		Version:      defaultDaemonVersion,
		BootID:       bootID,
		TotalThreads: uint32(runtime.NumCPU()),
		TotalRamMb:   memory.TotalMB,
		TotalDiskMb:  disk.TotalMB,
	}

	identity, err := node.LoadOrRegisterIdentity(ctx, d.nats, bootstrapInfo)
	if err != nil {
		return fmt.Errorf("load or register identity: %w", err)
	}
	d.logger.Info("scaled ready", "component", "scaled", "node_id", identity.NodeID, "region", identity.Region)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return node.RunHeartbeatLoop(gCtx, d.nats, identity, bootID, d.logger)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("run heartbeat loop: %w", err)
	}
	d.logger.Info("scaled stopped", "component", "scaled", "node_id", identity.NodeID)
	return nil
}

func (d *Daemon) Close() error {
	if d == nil || d.nats == nil {
		return nil
	}
	if err := d.nats.Drain(); err != nil {
		return fmt.Errorf("nats drain failed: %w", err)
	}
	return nil
}
