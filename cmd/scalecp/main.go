package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	scalecp "github.com/spacescale/core/internal/scalecp"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/spacescale/core/internal/shared/logger"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.Init(cfg.Environment)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cp, err := scalecp.New(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("initialize scalecp: %w", err)
	}
	defer func() {
		if err := cp.Close(); err != nil {
			log.Warn("scalecp close failed", "component", "scalecp", "error", err)
		}
	}()

	if err := cp.Run(ctx); err != nil {
		return fmt.Errorf("scalecp crashed: %w", err)
	}

	return nil
}
