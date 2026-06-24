// Package control starts the SpaceScale control-plane process.
package control

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/control/api"
	"github.com/spacescale/core/control/db/sqlc"
	"github.com/spacescale/core/control/fabric"
	"github.com/spacescale/core/control/tenant"
	"github.com/spacescale/core/shared/config"
	"github.com/spacescale/core/shared/logger"
	"github.com/spacescale/core/shared/nats"
	"github.com/spacescale/core/shared/secret"
)

const (
	controlDBMaxConns           = 25
	controlDBMinConns           = 5
	controlDBMaxConnLifetime    = 15 * time.Minute
	controlDBMaxConnIdleTime    = 5 * time.Minute
	controlDBPingTimeout        = 5 * time.Second
	controlPlaneShutdownTimeout = 10 * time.Second
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
	envCipher, err := secret.NewBox(cfg.EnvEncryptionKeyID, cfg.EnvEncryptionKey)
	if err != nil {
		return fmt.Errorf("service init failed: %w", err)
	}
	users := tenant.NewUserService(queries)
	projects := tenant.NewProjectService(queries)
	workspaces := tenant.NewWorkspaceService(queries)
	bootstrap := tenant.NewBootstrapService(queries)
	workloads := tenant.NewWorkloadService(queries, dbPool, envCipher)

	natsClient, err := nats.New(cfg.NATSURL, "controlp", log)
	if err != nil {
		return fmt.Errorf("nats init failed: %w", err)
	}
	defer func() { _ = natsClient.Drain() }()

	apiServer := api.NewServer(api.ServerDeps{
		Users:      users,
		Projects:   projects,
		Workspaces: workspaces,
		Bootstrap:  bootstrap,
		Workloads:  workloads,
		DBPool:     dbPool,
		Config:     cfg,
		Dispatcher: fabric.NewDispatcher(workloads, natsClient, log),
	})

	return runControlPlane(ctx, log, cfg.ListenAddr, apiServer)
}

func openDB(ctx context.Context, cfg config.Control) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	poolCfg.MaxConns = controlDBMaxConns
	poolCfg.MinConns = controlDBMinConns
	poolCfg.MaxConnLifetime = controlDBMaxConnLifetime
	poolCfg.MaxConnIdleTime = controlDBMaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, controlDBPingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()

		return nil, err
	}

	return pool, nil
}

func runControlPlane(ctx context.Context, log *slog.Logger, listenAddr string, apiServer *api.Server) error {
	serverErr := make(chan error, 1)
	go func() {
	log.Info("controlp listening", "addr", listenAddr)
		serverErr <- apiServer.Start()
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlPlaneShutdownTimeout)
		defer cancel()

		shutdownErr := apiServer.Shutdown(shutdownCtx)
		if err := <-serverErr; err != nil {
			return err
		}
		return shutdownErr
	}
}
