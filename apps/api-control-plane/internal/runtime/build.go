package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/config"
	"github.com/t0gun/spacescale/internal/controlplane"
	"github.com/t0gun/spacescale/internal/controlplane/state"
	"github.com/t0gun/spacescale/internal/http_api"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
	"google.golang.org/grpc"
)

type controlPlaneRuntime struct {
	server     *grpc.Server
	listener   net.Listener
	agentStore *state.PersistingAgentStore
}

func buildHTTPServer(cfg config.Config, envKeyring *service.EnvValueKeyring, queries *pgstore.Queries, dbPool *pgxpool.Pool) *http.Server {
	svcs := service.NewServices(queries, dbPool, envKeyring)
	api := http_api.NewServer(http_api.ServerDeps{
		Services: svcs,
		DBPool:   dbPool,
		Config:   cfg.API,
	})

	limitedHandler := http.MaxBytesHandler(api.Router(), cfg.HTTPServer.MaxBodyBytes)
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           limitedHandler,
		ReadHeaderTimeout: cfg.HTTPServer.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPServer.WriteTimeout,
		IdleTimeout:       cfg.HTTPServer.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTPServer.MaxHeaderBytes,
	}
}

func buildControlPlaneRuntime(cfg config.ControlPlaneConfig, queries *pgstore.Queries, logger *slog.Logger) (*controlPlaneRuntime, error) {
	agentStore, err := state.NewPersistingAgentStore(queries, cfg.LastSeenFlush, logger.With("component", "agent_state_store"))
	if err != nil {
		return nil, fmt.Errorf("agent store init failed: %w", err)
	}

	handler, err := controlplane.NewControlPlaneServer(agentStore, controlplane.ControlPlaneConfig{
		AgentToken:        cfg.AgentToken,
		HeartbeatInterval: cfg.HeartbeatInterval,
		LeaseTTL:          cfg.LeaseTTL,
		Version:           "api",
		Logger:            logger.With("component", "control_plane"),
	})
	if err != nil {
		return nil, fmt.Errorf("control plane init failed: %w", err)
	}

	controlPlaneServer := controlplane.NewGRPCServer(handler, cfg.MaxRecvMsgBytes, cfg.MaxSendMsgBytes)
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("control plane listen failed: %w", err)
	}

	return &controlPlaneRuntime{
		server:     controlPlaneServer,
		listener:   listener,
		agentStore: agentStore,
	}, nil
}

func buildEnvEncryptionRuntime(apiCfg config.APIConfig, dbPool *pgxpool.Pool, queries *pgstore.Queries, logger *slog.Logger) (*service.EnvValueKeyring, *service.EnvValueReencryptWorker, error) {
	envKeyring, err := service.NewEnvValueKeyring(apiCfg.EnvEncryption.ActiveKeyID, apiCfg.EnvEncryption.Keys)
	if err != nil {
		return nil, nil, fmt.Errorf("env encryption init failed: %w", err)
	}

	loadedKeyIDs := make([]string, 0, len(apiCfg.EnvEncryption.Keys))
	for keyID := range apiCfg.EnvEncryption.Keys {
		loadedKeyIDs = append(loadedKeyIDs, keyID)
	}
	sort.Strings(loadedKeyIDs)

	reencryptWorker, err := service.NewEnvValueReencryptWorker(service.EnvValueReencryptWorkerConfig{
		Pool:         dbPool,
		Queries:      queries,
		Keyring:      envKeyring,
		ActiveKeyID:  apiCfg.EnvEncryption.ActiveKeyID,
		LoadedKeyIDs: loadedKeyIDs,
		BatchSize:    apiCfg.EnvEncryption.ReencryptBatchSize,
		SweepPeriod:  apiCfg.EnvEncryption.ReencryptSweepPeriod,
		Logger:       logger.With("component", "env_reencrypt_worker"),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("env re-encryption worker init failed: %w", err)
	}

	return envKeyring, reencryptWorker, nil
}

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
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
