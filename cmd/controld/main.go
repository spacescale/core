// Package main starts the controld control-plane process.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spacescale/core/control"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := control.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)

		return 1
	}

	return 0
}
