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
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.Init(cfg.Environment)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cp, err := scalecp.New(ctx, cfg, log)
	if err != nil {
		log.Error("failed to initialize scalecp", "component", "scalecp", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := cp.Close(); err != nil {
			log.Warn("scalecp close failed", "component", "scalecp", "error", err)
		}
	}()

	if err := cp.Run(ctx); err != nil {
		log.Error("scalecp exited with error", "component", "scalecp", "error", err)
		os.Exit(1)
	}
}
