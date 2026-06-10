// Package control starts the SpaceScale control-plane process.
package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/control/api"
	"github.com/spacescale/core/control/db/sqlc"
	"github.com/spacescale/core/control/fabric"
	"github.com/spacescale/core/control/service"
	"github.com/spacescale/core/shared/config"
	"github.com/spacescale/core/shared/logger"
	"github.com/spacescale/core/shared/nats"
	"golang.org/x/sync/errgroup"
)

// Run starts the control plane and blocks until the context is canceled or startup fails.
func Run(ctx context.Context) error {
	cfg, err := config.LoadControl()
	if err != nil {
		return fmt.Errorf("load control config: %w", err)
	}

	log := logger.Init(cfg.Environment)
	dbPool, err := openDB(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}
	defer dbPool.Close()

	queries := sqlc.New(dbPool)
	services, err := service.NewServices(service.Deps{
		Queries:            queries,
		DBPool:             dbPool,
		EnvEncryptionKeyID: cfg.EnvEncryptionKeyID,
		EnvEncryptionKey:   cfg.EnvEncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("service init failed: %w", err)
	}

	natsClient, err := nats.New(cfg.NATSURL, "controlp", log)
	if err != nil {
		return fmt.Errorf("nats init failed: %w", err)
	}
	defer func() { _ = natsClient.Drain() }()

	apiServer := api.NewServer(api.ServerDeps{
		Services: services,
		DBPool:   dbPool,
		Config:   cfg,

		Dispatcher: fabric.NewDispatcher(services.Tenant.Apps, natsClient, log),
	})

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		log.Info("controlp listening", "component", "controlp", "addr", cfg.ListenAddr)

		return apiServer.Start()
	})

	group.Go(func() error {
		<-groupCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(groupCtx), 10*time.Second)
		defer cancel()

		return apiServer.Shutdown(shutdownCtx)
	})

	if err := group.Wait(); err != nil {
		return errors.Join(err, errors.New("control plane exited"))
	}

	return nil
}

func openDB(ctx context.Context, cfg config.Control) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 15 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()

		return nil, err
	}

	return pool, nil
}
