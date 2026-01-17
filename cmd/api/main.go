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

	"github.com/t0gun/paas/internal/adapters/runtime/fake"
	"github.com/t0gun/paas/internal/adapters/store"
	"github.com/t0gun/paas/internal/http_api"
	"github.com/t0gun/paas/internal/service"
)

func main() {
	// Read runtime config from env with defaults so local dev works out of the box.
	addr := env("ADDR", ":8080")
	workerToken := env("WORKER_TOKEN", "")
	baseDomain := env("BASE_DOMAIN", "example.com")

	st := store.NewMemoryStore()
	rt := fake.New(baseDomain)
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

func env(key, def string) string {
	// Return the env var value if set, otherwise fall back to the default.
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
