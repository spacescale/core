// Package dispatch implements the Control Plane's outbound scheduling engine.
// It acts as the "Brain" of the decentralized auction, broadcasting workload
// requirements over the NATS fabric and executing targeted placement commands.
package dispatch

import (
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/shared/nats"
	"github.com/spacescale/core/internal/shared/pb/v1"
)

type Dispatcher struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
	nats    *nats.Client
	logger  *slog.Logger
}

type Request struct {
	AppID        uuid.UUID
	DeploymentID uuid.UUID
	MachineID    uuid.UUID

	Region   string
	Tier     pb.Tier
	ImageRef string
	Env      map[string]string
}

type Winner struct {
	NodeID    string
	BootID    string
	FreeRamMB uint64
}

// New creates a stateless Dispatcher bound to the Postgres intent layer
// and the NATS communication fabric.
func New(queries *sqlc.Queries, pool *pgxpool.Pool, client *nats.Client, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		queries: queries,
		pool:    pool,
		nats:    client,
		logger:  logger,
	}
}
