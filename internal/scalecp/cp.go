package scalecp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/api"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/scalecp/fabric/ingress"
	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/spacescale/core/internal/shared/nats"
	"golang.org/x/sync/errgroup"
)

type ControlPlane struct {
	cfg      config.Config
	logger   *slog.Logger
	dbPool   *pgxpool.Pool
	nats     *nats.Client
	services *service.Services
	ingress  *ingress.BootstrapHandler
	api      *api.Server
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*ControlPlane, error) {
	var err error
	cfg, err = cfg.ValidateScalecp()
	if err != nil {
		return nil, fmt.Errorf("invalid scalecp config: %w", err)
	}

	dbPool, err := openDB(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database init failed: %w", err)
	}

	envCipher, err := tenant.NewEnvValueCipher(cfg.EnvEncryptionKeyID, cfg.EnvEncryptionKey)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("env encryption init failed: %w", err)
	}

	queries := sqlc.New(dbPool)
	services := service.NewServices(queries, dbPool, envCipher)

	natsClient, err := nats.New(cfg.NATSURL, "scalecp", logger)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("nats init failed: %w", err)
	}

	bootstrapHandler := ingress.NewBootstrapHandler(services.Node, logger)
	apiServer := api.NewServer(api.ServerDeps{
		Services: services,
		DBPool:   dbPool,
		Config:   cfg,
	})

	return &ControlPlane{
		cfg:      cfg,
		logger:   logger,
		dbPool:   dbPool,
		nats:     natsClient,
		services: services,
		ingress:  bootstrapHandler,
		api:      apiServer,
	}, nil
}

func (cp *ControlPlane) Run(ctx context.Context) error {
	if err := cp.ingress.Register(ctx, cp.nats); err != nil {
		return fmt.Errorf("bootstrap ingress register failed: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		cp.logger.Info("scalecp listening", "component", "scalecp", "addr", cp.cfg.ListenAddr())
		return cp.api.Start()
	})

	g.Go(func() error {
		<-gCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		return cp.api.Shutdown(shutdownCtx)
	})

	return g.Wait()
}

func (cp *ControlPlane) Close() error {
	var closeErr error

	if err := cp.nats.Drain(); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("nats drain failed: %w", err))
	}
	cp.dbPool.Close()

	return closeErr
}

func openDB(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
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
