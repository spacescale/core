// This file defines service-layer dependency wiring helpers.
// It centralizes construction of domain services from shared query dependencies
// so startup and tests can wire service graphs with one call.

package service

import pgstore "github.com/t0gun/spacescale/internal/postgres/gen"

// Services groups domain services used by HTTP handlers and startup wiring.
// This container is for dependency injection only; business behavior remains in
// dedicated service types.
type Services struct {
	Users      *UserService
	Projects   *ProjectService
	Workspaces *WorkspaceService
}

// NewServices builds all service dependencies from one shared query set.
func NewServices(queries *pgstore.Queries) *Services {
	return &Services{
		Users:      NewUserService(queries),
		Projects:   NewProjectService(queries),
		Workspaces: NewWorkspaceService(queries),
	}
}
