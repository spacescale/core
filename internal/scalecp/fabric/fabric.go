package fabric

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/scalecp/fabric/dispatch"
	"github.com/spacescale/core/internal/scalecp/fabric/ingress"
	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/shared/nats"
)

type Fabric struct {
	ingress    *ingress.Ingress
	dispatcher *dispatch.Dispatcher
}

func New(services *service.Services, queries *sqlc.Queries, pool *pgxpool.Pool, client *nats.Client, logger *slog.Logger) *Fabric {
	return &Fabric{
		ingress:    ingress.New(services.Fleet, logger),
		dispatcher: dispatch.New(queries, pool, client, logger),
	}
}

func (f *Fabric) Register(ctx context.Context, client *nats.Client) error {
	return f.ingress.Register(ctx, client)
}

func (f *Fabric) Dispatcher() *dispatch.Dispatcher {
	return f.dispatcher
}
