// This file implements workspace CRUD workflows in the service layer.
// It validates input, enforces ownership-scoped operations, maps DB rows into
// service models, and normalizes persistence errors into service sentinel errors.

package tenant

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spacescale/core/control/db/sqlc"
)

// Workspace represents one user-owned workspace.
type Workspace struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WorkspaceService provides workspace CRUD operations.
type WorkspaceService struct {
	queries *sqlc.Queries
}

// CreateWorkspaceParams holds create payload fields.
type CreateWorkspaceParams struct {
	Name string
}

// UpdateWorkspaceParams holds update payload fields.
type UpdateWorkspaceParams struct {
	Name string
}

// NewWorkspaceService creates a WorkspaceService bound to the provided query set.
func NewWorkspaceService(queries *sqlc.Queries) *WorkspaceService {
	return &WorkspaceService{
		queries: queries,
	}
}

// CreateWorkspace creates a workspace for the given owner user.
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, ownerUserID string, p CreateWorkspaceParams) (Workspace, error) {
	ownerID, ok := parseUUID(ownerUserID)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}

	name, ok := normalizeWorkspaceName(p.Name)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}
	row, err := s.queries.CreateWorkspace(ctx, sqlc.CreateWorkspaceParams{OwnerUserID: ownerID, Name: name})
	if err != nil {
		if isUniqueViolation(err) {
			return Workspace{}, ErrConflict
		}
		return Workspace{}, err
	}
	return workspaceFromRow(row), nil
}

// ListWorkspaces returns all workspaces owned by the given user.
func (s *WorkspaceService) ListWorkspaces(ctx context.Context, ownerUserID string) ([]Workspace, error) {
	ownerID, ok := parseUUID(ownerUserID)
	if !ok {
		return []Workspace{}, ErrInvalidInput
	}

	rows, err := s.queries.ListWorkspacesByOwnerUserID(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	items := make([]Workspace, 0, len(rows))
	for _, row := range rows {
		items = append(items, workspaceFromRow(row))
	}
	return items, nil
}

// GetWorkspace returns one workspace if it belongs to the given owner user.
func (s *WorkspaceService) GetWorkspace(ctx context.Context, ownerUserID, workspaceID string) (Workspace, error) {
	ownerID, ok := parseUUID(ownerUserID)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}
	wsID, ok := parseUUID(workspaceID)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}

	row, err := s.queries.GetWorkspaceByIDAndOwnerUserID(ctx, sqlc.GetWorkspaceByIDAndOwnerUserIDParams{
		ID:          wsID,
		OwnerUserID: ownerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Workspace{}, ErrUnauthorized
		}
		return Workspace{}, err
	}
	return workspaceFromRow(row), nil
}

// UpdateWorkspace updates one workspace if it belongs to the given owner user.
func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, ownerUserID, workspaceID string, p UpdateWorkspaceParams) (Workspace, error) {
	ownerID, ok := parseUUID(ownerUserID)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}
	wsID, ok := parseUUID(workspaceID)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}

	name, ok := normalizeWorkspaceName(p.Name)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}
	row, err := s.queries.UpdateWorkspaceByIDAndOwnerUserID(ctx, sqlc.UpdateWorkspaceByIDAndOwnerUserIDParams{
		ID:          wsID,
		OwnerUserID: ownerID,
		Name:        name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Workspace{}, ErrUnauthorized
		}
		if isUniqueViolation(err) {
			return Workspace{}, ErrConflict
		}
		return Workspace{}, err
	}
	return workspaceFromRow(row), nil
}

// DeleteWorkspace deletes one workspace if it belongs to the given owner user.
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, ownerUserID, workspaceID string) error {
	ownerID, ok := parseUUID(ownerUserID)
	if !ok {
		return ErrInvalidInput
	}
	wsID, ok := parseUUID(workspaceID)
	if !ok {
		return ErrInvalidInput
	}

	affected, err := s.queries.DeleteWorkspaceByIDAndOwnerUserID(ctx, sqlc.DeleteWorkspaceByIDAndOwnerUserIDParams{
		ID:          wsID,
		OwnerUserID: ownerID,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUnauthorized
	}
	return nil
}

func workspaceFromRow(r sqlc.Workspace) Workspace {
	return Workspace{
		ID:        r.ID.String(),
		Name:      r.Name,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
