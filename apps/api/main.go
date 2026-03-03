// This file is the API process entrypoint.
// It loads validated startup config, wires dependencies, starts HTTP serving,
// and handles graceful shutdown lifecycle.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/config"
	"github.com/t0gun/spacescale/internal/http_api"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("api process exited with error", "event", "process_exit_error", "error", err)
		os.Exit(1)
	}
}

// run wires process dependencies and coordinates startup/shutdown lifecycles.
//
// Concurrency model:
// - The current goroutine owns process lifecycle decisions.
// - One goroutine runs the HTTP server.
// - One goroutine runs the env re-encryption worker.
//
// Shutdown model:
// - A signal or server runtime failure triggers shutdown.
// - Worker cancellation happens first, with a bounded wait for exit.
// - HTTP server graceful shutdown happens second, with forced-close fallback.
func run(logger *slog.Logger) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("invalid startup config: %w", err)
	}

	dbPool, err := openDB(context.Background(), cfg.Database)
	if err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}
	defer dbPool.Close()
	queries := pgstore.New(dbPool)

	envKeyring, err := service.NewEnvValueKeyring(cfg.API.EnvEncryption.ActiveKeyID, cfg.API.EnvEncryption.Keys)
	if err != nil {
		return fmt.Errorf("env encryption init failed: %w", err)
	}

	loadedKeyIDs := make([]string, 0, len(cfg.API.EnvEncryption.Keys))
	for keyID := range cfg.API.EnvEncryption.Keys {
		loadedKeyIDs = append(loadedKeyIDs, keyID)
	}
	sort.Strings(loadedKeyIDs)

	reencryptWorker, err := service.NewEnvValueReencryptWorker(service.EnvValueReencryptWorkerConfig{
		Pool:         dbPool,
		Queries:      queries,
		Keyring:      envKeyring,
		ActiveKeyID:  cfg.API.EnvEncryption.ActiveKeyID,
		LoadedKeyIDs: loadedKeyIDs,
		BatchSize:    cfg.API.EnvEncryption.ReencryptBatchSize,
		SweepPeriod:  cfg.API.EnvEncryption.ReencryptSweepPeriod,
		Logger:       logger.With("component", "env_reencrypt_worker"),
	})
	if err != nil {
		return fmt.Errorf("env re-encryption worker init failed: %w", err)
	}

	// The worker is process-scoped, so it uses a cancelable background context.
	// There is no deadline here; shutdown explicitly calls workerCancel.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	// workerDone is a completion signal channel.
	// No values are sent on it; closing the channel means "worker exited".
	workerDone := make(chan struct{})
	go func() {
		// In service.EnvValueReencryptWorker.Run, the internal
		// `for { select { case <-ctx.Done(): ...; case <-ticker.C: ... } }`
		// loop is what blocks (waits) this worker goroutine between events.
		// It does not block the run goroutine.
		defer close(workerDone)
		reencryptWorker.Run(workerCtx)
	}()

	svcs := service.NewServices(queries, dbPool, envKeyring)
	api := http_api.NewServer(http_api.ServerDeps{
		Services: svcs,
		DBPool:   dbPool,
		Config:   cfg.API,
	})

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

	// Buffered by 1 so the server goroutine can report one fatal error without
	// requiring the run goroutine to be receiving at that exact instant.
	serveErrCh := make(chan error, 1)

	// Start serving in the background so run can block on signals and server errors.
	go func() {
		logger.Info("api listening", "event", "startup", "addr", cfg.Addr)
		// ListenAndServe returns on startup failure or after Shutdown is called.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	var runErr error
	// This select blocks the run goroutine only (other goroutines keep running)
	// until one shutdown trigger happens:
	// - OS signal via signalCtx.Done
	// - HTTP server runtime failure via serveErrCh
	select {
	case <-signalCtx.Done():
		logger.Info("shutdown requested", "event", "shutdown", "reason", "signal")
	case err := <-serveErrCh:
		runErr = fmt.Errorf("listen failed: %w", err)
		logger.Error("listen failed", "event", "runtime_error", "error", err)
	}

	// Stop the background worker first so it does not continue mutating DB state
	// while the process is draining. Then wait for completion with a hard cap so
	// shutdown cannot hang indefinitely if the worker gets stuck.
	// This select also blocks only the run goroutine while waiting for either:
	// - workerDone channel close (worker exited)
	// - timeout signal from time.After
	workerCancel()
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		logger.Warn("env re-encryption worker stop timed out", "event", "shutdown_warning")
	}

	// Give active requests a bounded window to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("shutting down", "event", "shutdown")
	// Stop accepting new connections and wait for in-flight handlers until timeout.
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "event", "shutdown_error", "error", err)
		shutdownErr := err
		// Force-close listeners when graceful shutdown does not complete so the
		// process can still terminate predictably after logging the failure.
		if closeErr := srv.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			logger.Error("forced close failed", "event", "shutdown_error", "error", closeErr)
			shutdownErr = errors.Join(shutdownErr, closeErr)
		}
		runErr = errors.Join(runErr, fmt.Errorf("http shutdown failed: %w", shutdownErr))
	}

	return runErr
}

// openDB initializes pgx pool from validated config and verifies connectivity.
func openDB(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
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
