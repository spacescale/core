// Package runtime owns process lifecycle ordering for API HTTP, control-plane
// gRPC, and background workers.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/config"
	"github.com/t0gun/spacescale/internal/controlplane/state"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
	"google.golang.org/grpc"
)

// Process holds long-lived runtime dependencies for a single API process.
type Process struct {
	cfg    config.Config
	logger *slog.Logger

	dbPool        *pgxpool.Pool
	httpServer    *http.Server
	controlPlane  *controlPlaneRuntime
	reencryptWkr  *service.EnvValueReencryptWorker
	workerCancel  context.CancelFunc
	workerDone    <-chan struct{}
	sweeperCancel context.CancelFunc
	sweeperDone   <-chan struct{}
}

// New builds runtime dependencies without starting background goroutines.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Process, error) {
	if logger == nil {
		logger = slog.Default()
	}

	dbPool, err := openDB(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("database init failed: %w", err)
	}

	queries := pgstore.New(dbPool)
	envKeyring, reencryptWorker, err := buildEnvEncryptionRuntime(cfg.API, dbPool, queries, logger)
	if err != nil {
		dbPool.Close()
		return nil, err
	}

	httpServer := buildHTTPServer(cfg, envKeyring, queries, dbPool)
	controlPlane, err := buildControlPlaneRuntime(cfg.ControlPlane, queries, logger)
	if err != nil {
		dbPool.Close()
		return nil, err
	}

	return &Process{
		cfg:          cfg,
		logger:       logger,
		dbPool:       dbPool,
		httpServer:   httpServer,
		controlPlane: controlPlane,
		reencryptWkr: reencryptWorker,
	}, nil
}

// Start begins serving network traffic and background workers.
func (p *Process) Start(serveErrCh chan<- error) {
	startHTTPServer(p.httpServer, p.cfg.Addr, p.logger, serveErrCh)
	startControlPlaneServer(p.controlPlane, p.logger, serveErrCh)

	p.workerCancel, p.workerDone = startEnvReencryptWorker(p.reencryptWkr)
	p.sweeperCancel, p.sweeperDone = startAgentLeaseSweeper(
		p.controlPlane.agentStore,
		p.cfg.ControlPlane.HeartbeatInterval,
		p.cfg.ControlPlane.LeaseTTL,
		p.logger.With("component", "agent_lease_sweeper"),
	)
}

// Shutdown drains workers first, then gRPC/HTTP servers, and finally closes DB.
func (p *Process) Shutdown() error {
	stopTask("env re-encryption worker", p.workerCancel, p.workerDone, 2*time.Second, p.logger)
	stopTask("agent lease sweeper", p.sweeperCancel, p.sweeperDone, 2*time.Second, p.logger)

	shutdownErr := errors.Join(
		shutdownControlPlane(p.controlPlane, p.logger),
		shutdownHTTPServer(p.httpServer, p.logger),
	)

	if p.dbPool != nil {
		p.dbPool.Close()
	}

	return shutdownErr
}

// WaitForShutdownTrigger blocks until either an OS signal arrives or a server
// goroutine reports a fatal runtime error.
func WaitForShutdownTrigger(errCh <-chan error, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown requested", "event", "shutdown", "reason", "signal")
		return nil
	case err := <-errCh:
		logger.Error("runtime server failure", "event", "runtime_error", "error", err)
		return err
	}
}

func startEnvReencryptWorker(worker *service.EnvValueReencryptWorker) (context.CancelFunc, <-chan struct{}) {
	if worker == nil {
		done := make(chan struct{})
		close(done)
		return func() {}, done
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	return cancel, done
}

func startAgentLeaseSweeper(agentStore *state.PersistingAgentStore, interval, leaseTTL time.Duration, logger *slog.Logger) (context.CancelFunc, <-chan struct{}) {
	if agentStore == nil {
		done := make(chan struct{})
		close(done)
		return func() {}, done
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				changed := agentStore.SweepOffline(leaseTTL, now.UTC())
				if changed > 0 {
					logger.Warn("agents marked offline by lease sweep", "event", "agent_ttl_sweep", "count", changed)
				}
			}
		}
	}()

	return cancel, done
}

func startHTTPServer(srv *http.Server, addr string, logger *slog.Logger, errCh chan<- error) {
	if srv == nil {
		return
	}

	go func() {
		logger.Info("api listening", "event", "startup", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http serve failed: %w", err)
		}
	}()
}

func startControlPlaneServer(cp *controlPlaneRuntime, logger *slog.Logger, errCh chan<- error) {
	if cp == nil || cp.server == nil || cp.listener == nil {
		return
	}

	go func() {
		logger.Info("control plane listening", "event", "startup", "addr", cp.listener.Addr().String())
		if err := cp.server.Serve(cp.listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("control plane serve failed: %w", err)
		}
	}()
}

func stopTask(name string, cancel context.CancelFunc, done <-chan struct{}, timeout time.Duration, logger *slog.Logger) {
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return
	}

	select {
	case <-done:
	case <-time.After(timeout):
		logger.Warn("background task stop timed out", "event", "shutdown_warning", "task", name)
	}
}

func shutdownControlPlane(cp *controlPlaneRuntime, logger *slog.Logger) error {
	if cp == nil || cp.server == nil {
		return nil
	}

	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		cp.server.GracefulStop()
	}()

	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		logger.Warn("control plane graceful stop timed out", "event", "shutdown_warning")
		cp.server.Stop()
		<-stopDone
	}

	if cp.listener != nil {
		if err := cp.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("control plane listener close failed: %w", err)
		}
	}

	return nil
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
		logger.Error("http shutdown failed", "event", "shutdown_error", "error", shutdownErr)
		return fmt.Errorf("http shutdown failed: %w", shutdownErr)
	}

	return nil
}
