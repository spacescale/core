package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/api"
	"github.com/spacescale/core/internal/scalecp/broker"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/spacescale/core/internal/shared/logger"
	"github.com/spacescale/core/internal/shared/nats"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	Logger := logger.Init(cfg.Environment)
	if err := run(context.Background(), cfg, Logger); err != nil {
		Logger.Error("scalecp exited with error", "component", "scalecp", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, cfg config.Config, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	cfg = cfg.Normalized()
	if err := cfg.ValidateScalecp(); err != nil {
		return fmt.Errorf("invalid scalecp config: %w", err)
	}

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPool, err := openDB(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}
	defer dbPool.Close()

	queries := sqlc.New(dbPool)
	envCipher, err := tenant.NewEnvValueCipher(cfg.EnvEncryptionKeyID, cfg.EnvEncryptionKey)
	if err != nil {
		return fmt.Errorf("env encryption init failed: %w", err)
	}

	services := service.NewServices(queries, dbPool, envCipher)

	// Start Nats
	natsClient, err := nats.New(cfg.NATSURL, "scalecp", logger)
	if err != nil {
		return fmt.Errorf("nats init failed: %w", err)
	}

	defer func() {
		if err := natsClient.Drain(); err != nil {
			logger.Warn("nats drain failed", "component", "scalecp", "error", err)
		}
	}()

	nodeBroker := broker.NewNodeBroker(services.Node, logger)
	if err := nodeBroker.Start(ctx, natsClient); err != nil {
		return fmt.Errorf("node broker start failed: %w", err)
	}

	server := api.NewServer(api.ServerDeps{
		Services: services,
		DBPool:   dbPool,
		Config:   cfg,
	})

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           http.MaxBytesHandler(server.Router(), 1<<20),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErrCh := make(chan error, 1)
	go func() {
		logger.Info("scalecp listening", "component", "scalecp", "addr", cfg.ListenAddr())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- fmt.Errorf("http serve failed: %w", err)
		}
	}()

	shutdown := func() error { return shutdownHTTPServer(httpServer, logger) }

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested", "component", "scalecp", "reason", "signal")
		return shutdown()
	case err := <-serveErrCh:
		logger.Error("scalecp server failure", "component", "scalecp", "error", err)
		return errors.Join(err, shutdown())
	}
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

func shutdownHTTPServer(srv *http.Server, logger *slog.Logger) error {
	if srv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		shutdownErr := err
		if closeErr := srv.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			shutdownErr = errors.Join(shutdownErr, closeErr)
		}
		logger.Error("http shutdown failed", "component", "scalecp", "error", shutdownErr)
		return fmt.Errorf("http shutdown failed: %w", shutdownErr)
	}

	return nil
}
