// This file defines service-layer dependency wiring helpers.
// It centralizes construction of domain services from shared dependencies
// (queries, db pool, and encryption primitives) so startup and tests can wire
// service graphs with one call.

package service

import (
	"github.com/jackc/pgx/v5/pgxpool"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

// Services groups domain services used by HTTP handlers and startup wiring.
// This container is for dependency injection only; business behavior remains in
// dedicated service types.
type Services struct {
	Users      *UserService
	Projects   *ProjectService
	Workspaces *WorkspaceService
	Bootstrap  *BootstrapService
	Apps       *AppService
}

// NewServices builds all service dependencies from one shared dependency set.
func NewServices(queries *pgstore.Queries, dbPool *pgxpool.Pool, envCipher *EnvValueCipher) *Services {
	return &Services{
		Users:      NewUserService(queries),
		Projects:   NewProjectService(queries),
		Workspaces: NewWorkspaceService(queries),
		Bootstrap:  NewBootstrapService(queries),
		Apps:       NewAppService(queries, dbPool, envCipher),
	}
}
