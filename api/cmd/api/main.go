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
	addr := env("ADDR", ":8080")
	databaseURL := env("DATABASE_URL", "")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Open a pgx connection pool and verify connectivity up front.
	dbPool, err := openDB(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}
	defer dbPool.Close()
	queries := pgstore.New(dbPool)

	svc := service.NewProjectService(queries)
	api := http_api.NewServer(svc)

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

// env returns an environment variable or a default value.
// It keeps configuration lookups concise at call sites and ensures defaults are
// explicit near startup logic.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// openDB opens a pgx pool and verifies it with a ping.
// A short ping timeout fails fast during startup so the process does not begin
// serving requests with an unavailable database.
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
