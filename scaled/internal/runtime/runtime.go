package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/t0gun/spacescale/apps/scaled/internal/config"
	"github.com/t0gun/spacescale/apps/scaled/internal/controlplane"
)

// Run loads startup configuration and supervises the control-plane client lifecycle.
func Run(ctx context.Context, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("invalid startup config: %w", err)
	}
	client := controlplane.NewClient(cfg, logger.With("component", "control_plane_client"))
	return client.Run(ctx)
}
