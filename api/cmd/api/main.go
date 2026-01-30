// API service entry point and lifecycle wiring.
// This file boots the HTTP server and validates core dependencies.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/adapters/runtime/docker"
	"github.com/t0gun/spacescale/internal/adapters/store"
	"github.com/t0gun/spacescale/internal/http_api"
	"github.com/t0gun/spacescale/internal/service"
)

// main starts the API server and waits for a shutdown signal.
func main() {
	// Read runtime config from env with defaults so local dev works out of the box.
	addr := env("ADDR", ":8080")
	workerToken := env("WORKER_TOKEN", "")
	baseDomain := env("BASE_DOMAIN", "example.com")

	// Database URL is required
	databaseURL := env("DATABASE_URL", "")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Open a pgx connection pool and verify the DB is reachable.
	// We keep the pool open for future store integration.
	dbPool, err := openDB(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}
	defer dbPool.Close()

	st := store.NewMemoryStore()
	rt, err := docker.New(docker.WithEdge(
		docker.EdgeConfig{
			BaseDomain: baseDomain,
			TraefikNet: env("TRAEFIK_NET", "traefik"),
			Scheme:     env("TRAEFIK_ENTRYPOINT", "web"),
			EnableTLS:  env("ENABLE_TLS", "") == "1",
			// CertResolver optional later:
			// CertResolver: env("CERT_RESOLVER", ""),
		},
	))
	if err != nil {
		log.Fatalf("docker runtime init: %v", err)
	}

	svc := service.NewAppServiceWithRuntime(st, rt)
	api := http_api.NewServer(svc, workerToken)

	// Configure the HTTP server with a read header timeout to avoid slowloris-style abuse.
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start the server in a goroutine so main can wait for shutdown signals.
	go func() {
		log.Printf("api listening on %s (base_domain=%s)", addr, baseDomain)
		// ListenAndServe blocks; it only returns on error or when Shutdown is called.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown
	// Create a buffered channel so a single signal won't be missed.
	stop := make(chan os.Signal, 1)
	// Register for SIGINT (Ctrl+C) and SIGTERM (container/service stop).
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	// Block until we receive a shutdown signal.Although program pauses but server is still running in go routine
	<-stop

	// Give active requests up to 10s to finish before forcing shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("shutting down...")
	// Shutdown stops accepting new connections, and it won't close active requests. we are using contexts to give active
	// requests a deadline either completed or not it would shut down when deadline is met.
	_ = srv.Shutdown(ctx)
}

// env returns an environment variable or a default value.
func env(key, def string) string {
	// Return the env var value if set, otherwise fall back to the default.
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// openDB opens a pgx pool and verifies it with a ping.
func openDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
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
