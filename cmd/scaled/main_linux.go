//go:build linux

// Package main starts the scaled edge daemon process.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spacescale/core/scaled"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := scaled.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)

		return 1
	}

	return 0
}
