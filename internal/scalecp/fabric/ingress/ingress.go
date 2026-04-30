// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package ingress

import (
	"context"
	"log/slog"

	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/shared/nats"
)

type Ingress struct {
	bootstrap *BootstrapHandler
}

func New(services service.FleetServices, logger *slog.Logger) *Ingress {
	return &Ingress{bootstrap: NewBootstrapHandler(services, logger)}
}

func (i *Ingress) Register(ctx context.Context, client *nats.Client) error {
	return i.bootstrap.Register(ctx, client)
}
