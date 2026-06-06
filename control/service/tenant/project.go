// This file implements project CRUD workflows in the service layer.
// It coordinates validation, defaulting, slug generation, ownership checks,
// and persistence mapping for API handlers.
// It also contains mapping helpers that translate SQLC rows into plain service
// structs consumed by HTTP handlers.
// Keep domain rules here so transport code stays thin and persistence details
// remain isolated behind a single business workflow boundary.

package tenant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spacescale/core/control/db/sqlc"
)

// Project represents a user-owned project.
type Project struct {
	ID          string
	WorkspaceID string
	Name        string
	Slug        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProjectService provides project creation operations.
type ProjectService struct {
	queries *sqlc.Queries
}

// CreateProjectParams contains optional overrides for project creation.
type CreateProjectParams struct {
	Name string
}

type UpdateProjectParams struct {
	Name string
}

// NewProjectService creates a ProjectService bound to the provided query set.
// Construction stays explicit so handlers and tests can wire dependencies
// through one place and keep storage details out of call sites.
func NewProjectService(queries *sqlc.Queries) *ProjectService {
	return &ProjectService{queries: queries}
}

// CreateProject creates a project inside a workspace owned by caller
// It trims request values and generates a fallback name when none is supplied
// by the caller.
// After validation and defaulting, it retries inserts on slug conflicts and
// maps the stored row into the service model returned to handlers.
// Validation failures are normalized to ErrInvalidInput, and exhausted slug
// retries are returned as ErrConflict.
func (s *ProjectService) CreateProject(ctx context.Context, ownerUserID, workspaceID string, p CreateProjectParams) (Project, error) {
	_, workspaceUUID, err := s.authorizeWorkspace(ctx, ownerUserID, workspaceID)
	if err != nil {
		return Project{}, err
	}

	name, ok := normalizeProjectName(p.Name)
	if !ok {
		if p.Name != "" {
			return Project{}, ErrInvalidInput
		}
		name, err = s.generateName(ctx)
		if err != nil {
			return Project{}, err
		}
	}

	project, err := buildProject(workspaceID, name)
	if err != nil {
		return Project{}, ErrInvalidInput
	}

	baseSlug := project.Slug
	for i := 0; i < maxSlugRetries; i++ {
		row, err := s.queries.CreateProject(ctx, sqlc.CreateProjectParams{
			WorkspaceID: workspaceUUID,
			Name:        project.Name,
			Slug:        project.Slug,
		})
		if err == nil {
			return projectFromRow(row), nil
		}
		if !isUniqueViolation(err) {
			return Project{}, err
		}
		suffix, suffixErr := randomSuffix(suffixLength)
		if suffixErr != nil {
			return Project{}, suffixErr
		}
		project.Slug = slugWithSuffix(baseSlug, suffix)
	}

	return Project{}, ErrConflict
}

func (s *ProjectService) ListProjects(ctx context.Context, ownerUserID, workspaceID string) ([]Project, error) {
	ownerUUID, workspaceUUID, err := s.authorizeWorkspace(ctx, ownerUserID, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListProjectsByWorkspaceIDAndOwnerUserID(ctx, sqlc.ListProjectsByWorkspaceIDAndOwnerUserIDParams{
		WorkspaceID: workspaceUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectFromRow(row))
	}
	return out, nil
}

func (s *ProjectService) GetProject(ctx context.Context, ownerUserID, workspaceID, projectID string) (Project, error) {
	ownerUUID, workspaceUUID, err := s.authorizeWorkspace(ctx, ownerUserID, workspaceID)
	if err != nil {
		return Project{}, err
	}
	projectUUID, ok := parseUUID(projectID)
	if !ok {
		return Project{}, ErrInvalidInput
	}
	row, err := s.getOwnedProjectInWorkspace(ctx, ownerUUID, workspaceUUID, projectUUID)
	if err != nil {
		return Project{}, err
	}
	return projectFromRow(row), nil
}

func (s *ProjectService) UpdateProject(
	ctx context.Context,
	ownerUserID, workspaceID, projectID string,
	p UpdateProjectParams,
) (Project, error) {
	ownerUUID, workspaceUUID, err := s.authorizeWorkspace(ctx, ownerUserID, workspaceID)
	if err != nil {
		return Project{}, err
	}
	projectUUID, ok := parseUUID(projectID)
	if !ok {
		return Project{}, ErrInvalidInput
	}
	if _, err := s.getOwnedProjectInWorkspace(ctx, ownerUUID, workspaceUUID, projectUUID); err != nil {
		return Project{}, err
	}
	normalizedName, ok := normalizeProjectName(p.Name)
	if !ok {
		return Project{}, ErrInvalidInput
	}
	row, err := s.queries.UpdateProjectByIDAndOwnerUserID(ctx, sqlc.UpdateProjectByIDAndOwnerUserIDParams{
		ID:          projectUUID,
		OwnerUserID: ownerUUID,
		Name:        normalizedName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrUnauthorized
		}
		return Project{}, err
	}
	return projectFromRow(row), nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, ownerUserID, workspaceID, projectID string) error {
	ownerUUID, workspaceUUID, err := s.authorizeWorkspace(ctx, ownerUserID, workspaceID)
	if err != nil {
		return err
	}
	projectUUID, ok := parseUUID(projectID)
	if !ok {
		return ErrInvalidInput
	}
	if _, err := s.getOwnedProjectInWorkspace(ctx, ownerUUID, workspaceUUID, projectUUID); err != nil {
		return err
	}
	affected, err := s.queries.DeleteProjectByIDAndOwnerUserID(ctx, sqlc.DeleteProjectByIDAndOwnerUserIDParams{
		ID:          projectUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *ProjectService) authorizeWorkspace(ctx context.Context, ownerUserID, workspaceID string) (uuid.UUID, uuid.UUID, error) {
	ownerUUID, ok := parseUUID(ownerUserID)
	if !ok {
		return uuid.Nil, uuid.Nil, ErrInvalidInput
	}
	workspaceUUID, ok := parseUUID(workspaceID)
	if !ok {
		return uuid.Nil, uuid.Nil, ErrInvalidInput
	}

	_, err := s.queries.GetWorkspaceByIDAndOwnerUserID(ctx, sqlc.GetWorkspaceByIDAndOwnerUserIDParams{
		ID:          workspaceUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, ErrUnauthorized
		}
		return uuid.Nil, uuid.Nil, err
	}
	return ownerUUID, workspaceUUID, nil
}

func (s *ProjectService) getOwnedProjectInWorkspace(
	ctx context.Context,
	ownerUUID, workspaceUUID, projectUUID uuid.UUID,
) (sqlc.Project, error) {
	row, err := s.queries.GetProjectByIDAndOwnerUserID(ctx, sqlc.GetProjectByIDAndOwnerUserIDParams{
		ID:          projectUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Project{}, ErrUnauthorized
		}
		return sqlc.Project{}, err
	}
	if row.WorkspaceID != workspaceUUID {
		return sqlc.Project{}, ErrUnauthorized
	}
	return row, nil
}

// buildProject validates raw project fields and derives a normalized slug.
// It requires non-empty workspace and name fields.
// Names that cannot produce a usable slug are rejected as invalid input.
// CreatedAt and UpdatedAt are set in UTC to keep serialization consistent.
func buildProject(workspaceID, name string) (Project, error) {
	if name == "" {
		return Project{}, errors.New("project name is required")
	}

	slug := slugifyProjectName(name)
	if slug == "" {
		return Project{}, errors.New("project name is invalid")
	}

	now := time.Now().UTC()
	return Project{
		ID:          "",
		WorkspaceID: workspaceID,
		Name:        name,
		Slug:        slug,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// generateName fetches one adjective and one noun from storage.
// The resulting adjective-noun format is readable and deterministic enough to
// feed directly into slug generation when no explicit name is provided.
func (s *ProjectService) generateName(ctx context.Context) (string, error) {
	adj, err := s.queries.RandomAdjective(ctx)
	if err != nil {
		return "", err
	}
	noun, err := s.queries.RandomNoun(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", adj, noun), nil
}

// projectFromRow maps a database project row into the service Project model.
// It converts UUID values into plain string identifiers for API-facing models,
// and copies user-facing fields and timestamps as-is.
// Keeping this translation at the service boundary prevents HTTP handlers from
// depending on SQLC-generated types and keeps model conversion rules centralized.
func projectFromRow(r sqlc.Project) Project {
	return Project{
		ID:          r.ID.String(),
		WorkspaceID: r.WorkspaceID.String(),
		Name:        r.Name,
		Slug:        r.Slug,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
