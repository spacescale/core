// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spacescale/core/internal/scaled"
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

	d, err := scaled.New(cfg, log)
	if err != nil {
		return fmt.Errorf("initialize scaled: %w", err)
	}
	defer func() {
		if err := d.Close(); err != nil {
			log.Warn("scaled close failed", "component", "scaled", "error", err)
		}
	}()

	if err := d.Run(ctx); err != nil {
		return fmt.Errorf("scaled crashed: %w", err)
	}

	return nil
}
