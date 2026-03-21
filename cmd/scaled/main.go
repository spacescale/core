package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spacescale/core/internal/shared/config"
	"github.com/spacescale/core/internal/shared/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	Logger := logger.Init(cfg.Environment)
	if err := run(context.Background(), cfg, Logger); err != nil {
		Logger.Error("scaled exited with error", "component", "scaled", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, cfg config.Config, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	cfg = cfg.Normalized()
	if err := cfg.ValidateScaled(); err != nil {
		return fmt.Errorf("invalid scaled config: %w", err)
	}

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("scaled ready", "component", "scaled")
	<-ctx.Done()
	logger.Info("scaled stopped", "component", "scaled")
	return nil
}
