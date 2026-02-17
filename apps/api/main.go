// This file is the API process entrypoint.
// It loads validated startup config, wires dependencies, starts HTTP serving,
// and handles graceful shutdown lifecycle.

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/http_api"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	cfg, err := loadAppConfig()
	if err != nil {
		logger.Error("invalid startup config", "event", "startup_error", "error", err)
		os.Exit(1)
	}
	dbPool, err := openDB(context.Background(), cfg.Database)
	if err != nil {
		logger.Error("database init failed", "event", "startup_error", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	queries := pgstore.New(dbPool)

	svc := service.NewProjectService(queries)
	api := http_api.NewServer(svc, cfg.Auth, dbPool, cfg.RateLimit, cfg.LogPrivacy)

	// Wrap the router with a global body-size limiter so body reads beyond this
	// cap fail fast and handlers never process unbounded payloads.
	limitedHandler := http.MaxBytesHandler(api.Router(), cfg.HTTPServer.MaxBodyBytes)

	// Apply server-level request bounds to reduce exposure to abuse patterns.
	// - ReadHeaderTimeout limits how long clients may spend sending headers.
	// - WriteTimeout limits how long a response write may block.
	// - MaxHeaderBytes explicitly caps total request header bytes.
	// - limitedHandler limits total request body size.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           limitedHandler,
		ReadHeaderTimeout: cfg.HTTPServer.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPServer.WriteTimeout,
		IdleTimeout:       cfg.HTTPServer.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTPServer.MaxHeaderBytes,
	}

	// Start serving in the background so the main goroutine can block on signals.
	go func() {
		logger.Info("api listening", "event", "startup", "addr", cfg.Addr)
		// ListenAndServe returns on startup failure or after Shutdown is called.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "event", "runtime_error", "error", err)
			os.Exit(1)
		}
	}()

	// Use a buffered channel so one incoming signal is never dropped.
	stop := make(chan os.Signal, 1)
	// Handle both local interrupt and container orchestrator termination signals.
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	// Block until shutdown is requested.
	<-stop

	// Give active requests a bounded window to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("shutting down", "event", "shutdown")
	// Stop accepting new connections and wait for in-flight handlers until timeout.
	shutdownFailed := false
	if err := srv.Shutdown(ctx); err != nil {
		shutdownFailed = true
		logger.Error("graceful shutdown failed", "event", "shutdown_error", "error", err)
		// Force-close listeners when graceful shutdown does not complete so the
		// process can still terminate predictably after logging the failure.
		if closeErr := srv.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			logger.Error("forced close failed", "event", "shutdown_error", "error", closeErr)
		}
	}
	if shutdownFailed {
		// Exit non-zero when graceful shutdown fails so supervisors and CI systems
		// can detect abnormal termination.
		os.Exit(1)
	}
}

// openDB initializes pgx pool from validated config and verifies connectivity.
func openDB(ctx context.Context, cfg DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, err
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// pool.Ping is pgx's standard readiness check.
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
