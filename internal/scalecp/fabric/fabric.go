package fabric

import (
	"context"
	"log/slog"

	"github.com/spacescale/core/internal/scalecp/fabric/ingress"
	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/shared/nats"
)

type Fabric struct {
	ingress *ingress.Ingress
}

func New(services *service.Services, logger *slog.Logger) *Fabric {
	return &Fabric{ingress: ingress.New(services.Fleet, logger)}
}

func (f *Fabric) Register(ctx context.Context, client *nats.Client) error {
	return f.ingress.Register(ctx, client)
}
