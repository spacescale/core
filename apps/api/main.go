// This file is the process entrypoint for the API binary.
// It owns startup sequence from configuration loading, to database connection
// validation, to service and router wiring before the HTTP server starts.
// It also owns graceful shutdown behavior by listening for termination signals
// and giving in-flight requests a bounded amount of time to finish.
// The small helpers at the bottom keep environment lookup and database startup
// rules centralized so boot behavior stays predictable and easy to reason about.

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/http_api"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

// main starts the API server and waits for a shutdown signal.
// Startup performs configuration loading, database initialization, service and
// router wiring, and HTTP server launch.
// Shutdown listens for process signals and allows in-flight requests to finish
// within a bounded timeout for graceful termination.
func main() {
	// Read runtime config with a sensible default listen address.
	addr := envStr("ADDR", ":8080")
	databaseURL := envStr("DATABASE_URL", "")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	bffJWTSecret := envStr("BFF_JWT_SECRET", "")
	if bffJWTSecret == "" {
		log.Fatal("BFF_JWT_SECRET is required")
	}

	authCfg := http_api.AuthConfig{
		JWTSecret: bffJWTSecret,
		Issuer:    envStr("BFF_JWT_ISSUER", "spacescale-web-bff"), // which issuer should I trust?
		Audience:  envStr("BFF_JWT_AUDIENCE", "spacescale-api"),   // expected audience
	}
	if err := authCfg.Validate(); err != nil {
		log.Fatalf("auth config: %v", err)
	}

	internalAuthSyncSecret := envStr("INTERNAL_AUTH_SYNC_SECRET", "")
	if internalAuthSyncSecret == "" {
		log.Fatal("INTERNAL_AUTH_SYNC_SECRET is required")
	}

	// Open a pgx connection pool and verify connectivity up front.
	dbPool, err := openDB(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}
	defer dbPool.Close()
	queries := pgstore.New(dbPool)

	svc := service.NewProjectService(queries)
	api := http_api.NewServer(svc, authCfg, dbPool, internalAuthSyncSecret)

	// Apply a read-header timeout to reduce exposure to slowloris-style abuse.
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start serving in the background so the main goroutine can block on signals.
	go func() {
		log.Printf("api listening on %s ", addr)
		// ListenAndServe returns on startup failure or after Shutdown is called.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Use a buffered channel so one incoming signal is never dropped.
	stop := make(chan os.Signal, 1)
	// Handle both local interrupt and container orchestrator termination signals.
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	// Block until shutdown is requested.
	<-stop

	// Give active requests a bounded window to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("shutting down...")
	// Stop accepting new connections and wait for in-flight handlers until timeout.
	_ = srv.Shutdown(ctx)
}

// envStr returns a string environment variable or a default value.
// It keeps configuration lookups concise at call sites and ensures defaults are
// explicit near startup logic.
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseEnvInt32 parses an environment variable as int32 with a default fallback.
// Returns default if env var is empty, invalid, or out of int32 range.
func parseEnvInt32(key string, def int32) int32 {
	const (
		int32Max = int64(^uint32(0) >> 1) // 2147483647
		int32Min = -int32Max - 1          // -2147483648
	)
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	if n < int32Min || n > int32Max {
		return def
	}
	return int32(n)
}

// openDB opens a pgx pool and verifies it with a ping.
// A short ping timeout fails fast during startup so the process does not begin
// serving requests with an unavailable database.
func openDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	// tune connections
	cfg.MaxConns = parseEnvInt32("DB_MAX_CONNS", 20)
	cfg.MinConns = parseEnvInt32("DB_MIN_CONNS", 5)
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute // close idle connections after 30 minutes

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
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
