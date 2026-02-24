// This file implements workspace CRUD workflows in the service layer.
// It validates input, enforces ownership-scoped operations, maps DB rows into
// service models, and normalizes persistence errors into service sentinel errors.

package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

const maxWorkspaceNameChars = 255

// Workspace represents one user-owned workspace.
type Workspace struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WorkspaceService provides workspace CRUD operations.
type WorkspaceService struct {
	queries *pgstore.Queries
}

// CreateWorkspaceParams holds create payload fields.
type CreateWorkspaceParams struct {
	Name string
}

// UpdateWorkspaceParams holds update payload fields.
type UpdateWorkspaceParams struct {
	Name string
}

func NewWorkspaceService(queries *pgstore.Queries) *WorkspaceService {
	return &WorkspaceService{
		queries: queries,
	}
}

// CreateWorkspace creates a workspace for the given owner user.
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, ownerUserID string, p CreateWorkspaceParams) (Workspace, error) {
	ownerID, err := uuid.Parse(strings.TrimSpace(ownerUserID))
	if err != nil {
		return Workspace{}, ErrInvalidInput
	}
	name, ok := normalizeWorkspaceName(p.Name)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}
	row, err := s.queries.CreateWorkspace(ctx, pgstore.CreateWorkspaceParams{OwnerUserID: ownerID, Name: name})
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
	ownerID, err := uuid.Parse(strings.TrimSpace(ownerUserID))
	if err != nil {
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
	ownerID, err := uuid.Parse(strings.TrimSpace(ownerUserID))
	if err != nil {
		return Workspace{}, ErrInvalidInput
	}
	wsID, err := uuid.Parse(strings.TrimSpace(workspaceID))
	if err != nil {
		return Workspace{}, ErrInvalidInput
	}
	row, err := s.queries.GetWorkspaceByIDAndOwnerUserID(ctx, pgstore.GetWorkspaceByIDAndOwnerUserIDParams{
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
	ownerID, err := uuid.Parse(strings.TrimSpace(ownerUserID))
	if err != nil {
		return Workspace{}, ErrInvalidInput
	}
	wsID, err := uuid.Parse(strings.TrimSpace(workspaceID))
	if err != nil {
		return Workspace{}, ErrInvalidInput
	}
	name, ok := normalizeWorkspaceName(p.Name)
	if !ok {
		return Workspace{}, ErrInvalidInput
	}
	row, err := s.queries.UpdateWorkspaceByIDAndOwnerUserID(ctx, pgstore.UpdateWorkspaceByIDAndOwnerUserIDParams{
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
	ownerID, err := uuid.Parse(strings.TrimSpace(ownerUserID))
	if err != nil {
		return ErrInvalidInput
	}
	wsID, err := uuid.Parse(strings.TrimSpace(workspaceID))
	if err != nil {
		return ErrInvalidInput
	}

	affected, err := s.queries.DeleteWorkspaceByIDAndOwnerUserID(ctx, pgstore.DeleteWorkspaceByIDAndOwnerUserIDParams{
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

func workspaceFromRow(r pgstore.Workspace) Workspace {
	return Workspace{
		ID:        r.ID.String(),
		Name:      r.Name,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func normalizeWorkspaceName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", false
	}
	if utf8.RuneCountInString(name) > maxWorkspaceNameChars {
		return "", false
	}
	return name, true
}
