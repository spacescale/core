// This file implements the project-creation workflow in the service layer.
// It coordinates project-shape validation, request normalization, defaulting,
// slug generation, and conflict-retry behavior.
// It also contains mapping helpers that translate SQLC rows into plain service
// structs consumed by HTTP handlers.
// Keep domain rules here so transport code stays thin and persistence details
// remain isolated behind a single business workflow boundary.

package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

const (
	defaultRegion  = "global"
	maxSlugRetries = 8
	suffixLength   = 6
)

// Project represents a user-owned project.
type Project struct {
	ID          string
	WorkspaceID string
	Name        string
	Slug        string
	Region      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProjectService provides project creation operations.
type ProjectService struct {
	queries *pgstore.Queries
}

// CreateProjectParams contains optional overrides for project creation.
type CreateProjectParams struct {
	Name   string
	Region string
}

// NewProjectService creates a ProjectService bound to the provided query set.
// Construction stays explicit so handlers and tests can wire dependencies
// through one place and keep storage details out of call sites.
func NewProjectService(queries *pgstore.Queries) *ProjectService {
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
	ownerUserID = strings.TrimSpace(ownerUserID)
	workspaceID = strings.TrimSpace(workspaceID)

	if ownerUserID == "" || workspaceID == "" {
		return Project{}, ErrInvalidInput
	}

	ownerUUID, err := uuid.Parse(ownerUserID)
	if err != nil {
		return Project{}, ErrInvalidInput
	}

	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return Project{}, ErrInvalidInput
	}

	_, err = s.queries.GetWorkspaceByIDAndOwnerUserID(ctx, pgstore.GetWorkspaceByIDAndOwnerUserIDParams{ID: workspaceUUID, OwnerUserID: ownerUUID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrUnauthorized
		}
		return Project{}, err
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		name, err = s.generateName(ctx)
		if err != nil {
			return Project{}, err
		}
	}

	project, err := buildProject(workspaceID, name, p.Region)
	if err != nil {
		return Project{}, ErrInvalidInput
	}

	baseSlug := project.Slug
	for i := 0; i < maxSlugRetries; i++ {
		row, err := s.queries.CreateProject(ctx, pgstore.CreateProjectParams{
			WorkspaceID: workspaceUUID,
			Name:        project.Name,
			Slug:        project.Slug,
			Region:      project.Region,
		})
		if err == nil {
			return projectFromRow(row), nil
		}
		if !isUniqueViolation(err) {
			return Project{}, err
		}
		project.Slug = baseSlug + "-" + randomSuffix(suffixLength)
	}

	return Project{}, ErrConflict
}

// buildProject validates raw project fields and applies service defaults.
// It requires non-empty workspace and name fields, assigns the default region when
// omitted, and derives a normalized slug from the display name.
// Names that cannot produce a usable slug are rejected as invalid input.
// CreatedAt and UpdatedAt are set in UTC to keep serialization consistent.
func buildProject(workspaceID, name, region string) (Project, error) {
	workspace := strings.TrimSpace(workspaceID)
	if workspace == "" {
		return Project{}, errors.New("workspace id is required")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, errors.New("project name is required")
	}

	region = strings.TrimSpace(region)
	if region == "" {
		region = defaultRegion
	}

	slug := slugifyProjectName(name)
	if slug == "" {
		return Project{}, errors.New("project name is invalid")
	}

	now := time.Now().UTC()
	return Project{
		ID:          "",
		WorkspaceID: workspace,
		Name:        name,
		Slug:        slug,
		Region:      region,
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

// slugifyProjectName converts a raw project name into a stable URL slug.
// The transformation is intentionally strict and predictable:
// 1) trim surrounding whitespace and lowercase the input,
// 2) keep only letters and numbers while collapsing separator runs to one hyphen,
// 3) trim edge hyphens so the returned value is path-safe.
func slugifyProjectName(name string) string {
	// Normalize first so casing and accidental outer spaces never affect slug
	// uniqueness or readability.
	normalized := strings.ToLower(strings.TrimSpace(name))

	var b strings.Builder // build slug incrementally to avoid repeated string allocations.

	var prevHyphen bool // tracks whether the most recently written byte was '-' for separator collapsing.

	for _, r := range normalized {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			// Keep alphanumeric runes exactly as they are after normalization.
			b.WriteRune(r)
			prevHyphen = false
		default:
			// Treat every non-alphanumeric rune as a separator boundary.
			// Add one hyphen only when:
			// - the previous output character was not already a hyphen, and
			// - output is not empty (prevents leading hyphens).
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}

	// Remove boundary hyphens that can appear when input begins or ends with
	// separators.
	return strings.Trim(b.String(), "-")
}

// randomSuffix returns a random lowercase alphanumeric suffix of fixed length.
// It is used only for slug collision retries so the user-visible base slug
// remains stable while each persistence attempt gets a fresh candidate.
func randomSuffix(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String()
}

// projectFromRow maps a database project row into the service Project model.
// It converts UUID values into plain string identifiers for API-facing models,
// and copies user-facing fields and timestamps as-is.
// Keeping this translation at the service boundary prevents HTTP handlers from
// depending on SQLC-generated types and keeps model conversion rules centralized.
func projectFromRow(r pgstore.Project) Project {
	return Project{
		ID:          r.ID.String(),
		WorkspaceID: r.WorkspaceID.String(),
		Name:        r.Name,
		Slug:        r.Slug,
		Region:      r.Region,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// isUniqueViolation reports whether an error is a PostgreSQL unique violation.
// The result is used to trigger slug retry logic and separate expected conflict
// handling from unexpected database failures.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
