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
	pb "github.com/spacescale/core/internal/shared/pb/v1"
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
	MicroVMID    uuid.UUID

	Region      string
	Shape       *pb.MicroVMShape
	ImageRef    string
	Env         map[string]string
	RuntimePort uint32
}

type Winner struct {
	NodeID string
	BootID string
}

func shapeLogAttrs(shape *pb.MicroVMShape) []any {
	if shape == nil {
		return []any{"vcpu", uint32(0), "ram_mb", uint64(0), "cpu_mode", "unspecified", "root_disk_mb", uint64(0), "volume_mb", uint64(0)}
	}

	cpuMode := "unspecified"
	if shape.CpuMode == pb.CpuMode_CPU_MODE_SHARED {
		cpuMode = "shared"
	}
	if shape.CpuMode == pb.CpuMode_CPU_MODE_PINNED {
		cpuMode = "pinned"
	}

	return []any{
		"vcpu", shape.Vcpu,
		"ram_mb", shape.RamMb,
		"cpu_mode", cpuMode,
		"root_disk_mb", shape.RootDiskMb,
		"volume_mb", shape.VolumeMb,
	}
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
