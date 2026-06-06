package service

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/scalecp/service/fleet"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

// Services groups domain services used by HTTP and control-fabric wiring.
type Services struct {
	Tenant TenantServices
	Fleet  FleetServices
}

type Deps struct {
	Queries            *sqlc.Queries
	DBPool             *pgxpool.Pool
	EnvEncryptionKeyID string
	EnvEncryptionKey   []byte
}

// TenantServices groups control plane business logic for user owned resources.
type TenantServices struct {
	Users      *tenant.UserService
	Projects   *tenant.ProjectService
	Workspaces *tenant.WorkspaceService
	Bootstrap  *tenant.BootstrapService
	Apps       *tenant.AppService
}

// FleetServices groups control plane business logic for managed edge fleet lifecycle.
type FleetServices struct {
	Bootstrap *fleet.BootstrapService
}

// NewServices builds all service dependencies from one shared dependency set.
func NewServices(deps Deps) (*Services, error) {
	envCipher, err := tenant.NewEnvValueCipher(deps.EnvEncryptionKeyID, deps.EnvEncryptionKey)
	if err != nil {
		return nil, err
	}

	return &Services{
		Tenant: TenantServices{
			Users:      tenant.NewUserService(deps.Queries),
			Projects:   tenant.NewProjectService(deps.Queries),
			Workspaces: tenant.NewWorkspaceService(deps.Queries),
			Bootstrap:  tenant.NewBootstrapService(deps.Queries),
			Apps:       tenant.NewAppService(deps.Queries, deps.DBPool, envCipher),
		},
		Fleet: FleetServices{
			Bootstrap: fleet.NewBootstrapService(deps.Queries, deps.DBPool),
		},
	}, nil
}
