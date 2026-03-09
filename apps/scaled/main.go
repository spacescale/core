package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/t0gun/spacescale/apps/scaled/internal/runtime"
)

// main boots the scaled runtime and blocks until shutdown.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runtime.Run(ctx, logger); err != nil {
		logger.Error("scaled process exited with error", "event", "process_exit_error", "error", err)
		os.Exit(1)
	}
}
