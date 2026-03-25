// This file implements bootstrap workflows that span multiple domain entities.
// Bootstrap is intentionally isolated from workspace/project CRUD so future
// onboarding steps (for example free trial setup) can evolve independently.
// Testing note: bootstrap workflows in this file are DB-first operations, so
// behavior is validated by HTTP integration tests instead of service unit tests.

package tenant

import (
	"context"
	"fmt"

	"github.com/spacescale/core/internal/scalecp/db/sqlc"
)

const defaultBootstrapWorkspaceName = "workspace-01"

// BootstrapDefaultsResult is the service response for bootstrap execution.
type BootstrapDefaultsResult struct {
	Created     bool
	WorkspaceID string
	ProjectID   string
}

// BootstrapService orchestrates first-time user default resource provisioning.
type BootstrapService struct {
	queries *sqlc.Queries
}

// NewBootstrapService creates a BootstrapService bound to the query set.
func NewBootstrapService(queries *sqlc.Queries) *BootstrapService {
	return &BootstrapService{queries: queries}
}

// BootstrapDefaults creates default workspace+project for a user that has no
// workspace yet. It is idempotent: returning users become a no-op.
func (s *BootstrapService) BootstrapDefaults(ctx context.Context, ownerUserID string) (BootstrapDefaultsResult, error) {
	ownerUUID, ok := parseUUID(ownerUserID)
	if !ok {
		return BootstrapDefaultsResult{}, ErrInvalidInput
	}

	projectName, err := s.generateDefaultProjectName(ctx)
	if err != nil {
		return BootstrapDefaultsResult{}, err
	}
	baseSlug := slugifyProjectName(projectName)
	if baseSlug == "" {
		return BootstrapDefaultsResult{}, ErrInvalidInput
	}
	// Keep a stable human-readable base slug and only vary suffix on collisions.
	candidateSlug := baseSlug
	for i := 0; i < maxSlugRetries; i++ {
		row, err := s.queries.BootstrapDefaults(ctx, sqlc.BootstrapDefaultsParams{
			OwnerUserID:   ownerUUID,
			WorkspaceName: defaultBootstrapWorkspaceName,
			ProjectName:   projectName,
			ProjectSlug:   candidateSlug,
			ProjectRegion: defaultRegion,
		})
		if err == nil {
			out := BootstrapDefaultsResult{
				Created: row.Created,
			}
			if row.Created {
				out.WorkspaceID = uuidOrEmpty(row.WorkspaceID)
				out.ProjectID = uuidOrEmpty(row.ProjectID)
			}
			return out, nil
		}
		if !isUniqueViolation(err) {
			return BootstrapDefaultsResult{}, err
		}
		suffix, suffixErr := randomSuffix(suffixLength)
		if suffixErr != nil {
			return BootstrapDefaultsResult{}, suffixErr
		}
		candidateSlug = slugWithSuffix(baseSlug, suffix)
	}
	return BootstrapDefaultsResult{}, ErrConflict
}

// generateDefaultProjectName composes one adjective and one noun so bootstrap
// creates readable default project names consistent with project generation.
func (s *BootstrapService) generateDefaultProjectName(ctx context.Context) (string, error) {
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
