package service

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	nodesvc "github.com/spacescale/core/internal/scalecp/service/node"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

// Services groups domain services used by HTTP and broker wiring.
type Services struct {
	Tenant TenantServices
	Node   NodeServices
}

// TenantServices groups control plane business logic for user owned resources.
type TenantServices struct {
	Users      *tenant.UserService
	Projects   *tenant.ProjectService
	Workspaces *tenant.WorkspaceService
	Bootstrap  *tenant.BootstrapService
	Apps       *tenant.AppService
}

// NodeServices groups control plane business logic for node lifecycle
type NodeServices struct {
	Registrar *nodesvc.Registrar
	Presence  *nodesvc.PresenceManager
}

// NewServices builds all service dependencies from one shared dependency set.
func NewServices(queries *sqlc.Queries, dbPool *pgxpool.Pool, envCipher *tenant.EnvValueCipher) *Services {
	return &Services{
		Tenant: TenantServices{
			Users:      tenant.NewUserService(queries),
			Projects:   tenant.NewProjectService(queries),
			Workspaces: tenant.NewWorkspaceService(queries),
			Bootstrap:  tenant.NewBootstrapService(queries),
			Apps:       tenant.NewAppService(queries, dbPool, envCipher),
		},
		Node: NodeServices{
			Registrar: nodesvc.NewRegistrar(queries, dbPool),
			Presence:  nodesvc.NewPresenceManager(queries),
		},
	}
}
