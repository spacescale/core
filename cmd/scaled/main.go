package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	scaled "github.com/spacescale/core/internal/scaled"
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

	d, err := scaled.New(ctx, cfg, log)
	if err != nil {
		log.Error("failed to initialize scaled", "component", "scaled", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := d.Close(); err != nil {
			log.Warn("scaled close failed", "component", "scaled", "error", err)
		}
	}()

	if err := d.Run(ctx); err != nil {
		log.Error("scaled exited with error", "component", "scaled", "error", err)
		os.Exit(1)
	}
}
