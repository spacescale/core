// This file is the process entrypoint for the API binary.
//
// Responsibilities in this file:
// - Initialize process-wide structured logging.
// - Load typed startup configuration and fail fast on invalid setup.
// - Open the database pool and verify connectivity before serving traffic.
// - Wire service dependencies and HTTP router/middleware composition.
// - Configure and start the HTTP server with runtime safety limits.
// - Handle graceful shutdown signals and terminate predictably on failure.
//
// Design note:
// - Configuration parsing and normalization now live in config.go so this file
//   stays focused on orchestration and lifecycle management.

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
	// - We intentionally rely on net/http default MaxHeaderBytes (1 MiB).
	// - limitedHandler limits total request body size.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           limitedHandler,
		ReadHeaderTimeout: cfg.HTTPServer.ReadHeaderTimeout,
		IdleTimeout:       cfg.HTTPServer.IdleTimeout,
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
	// Handle both local interrupt  signals.
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	// Block until shutdown is requested.
	<-stop

	// Give active requests a bounded window to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("shutting down", "event", "shutdown")
	// Stop accepting new connections and wait for in-flight handlers until timeout.
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "event", "shutdown_error", "error", err)
		// Force-close listeners when graceful shutdown does not complete so the
		// process can still terminate predictably after logging the failure.
		if closeErr := srv.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			logger.Error("forced close failed", "event", "shutdown_error", "error", closeErr)
		}
	}
}

// openDB opens a pgx pool and verifies it with a ping.
// A short ping timeout fails fast during startup so the process does not begin
// serving requests with an unavailable database.
//
// Caller contract:
// - cfg.URL must be a valid PostgreSQL connection string.
// - cfg connection pool fields should already be normalized by config loading.
func openDB(ctx context.Context, cfg DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, err
	}

	// Apply validated pool tuning from startup configuration.
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
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
