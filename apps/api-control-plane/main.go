// This file is the API process entrypoint.
// It loads startup config, wires process runtime, and coordinates lifecycle.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/t0gun/spacescale/internal/config"
	"github.com/t0gun/spacescale/internal/runtime"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("api process exited with error", "event", "process_exit_error", "error", err)
		os.Exit(1)
	}
}

// run is a thin process orchestrator. Detailed startup/shutdown behavior lives
// in internal/runtime so entrypoint logic stays short and explicit.
func run(logger *slog.Logger) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("invalid startup config: %w", err)
	}

	process, err := runtime.New(context.Background(), cfg, logger)
	if err != nil {
		return err
	}
	serveErrCh := make(chan error, 2)
	process.Start(serveErrCh)

	runErr := runtime.WaitForShutdownTrigger(serveErrCh, logger)
	return errors.Join(runErr, process.Shutdown())
}
