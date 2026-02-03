// Package service contains business workflows for the API.
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

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
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
	OwnerUserID string
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

// NewProjectService builds a ProjectService.
func NewProjectService(queries *pgstore.Queries) *ProjectService {
	return &ProjectService{queries: queries}
}

// CreateProject creates a new project for the given GitHub ID.
func (s *ProjectService) CreateProject(ctx context.Context, githubID string, p CreateProjectParams) (Project, error) {
	githubID = strings.TrimSpace(githubID)
	userRow, err := s.queries.UpsertUserByGithubID(ctx, githubID)
	if err != nil {
		return Project{}, err
	}
	user := userFromRow(userRow)

	name := strings.TrimSpace(p.Name)
	if name == "" {
		name, err = s.generateName(ctx)
		if err != nil {
			return Project{}, err
		}
	}

	project, err := buildProject(user.ID, name, p.Region)
	if err != nil {
		return Project{}, ErrInvalidInput
	}

	baseSlug := project.Slug
	for i := 0; i < maxSlugRetries; i++ {
		row, err := s.queries.CreateProject(ctx, pgstore.CreateProjectParams{
			OwnerUserID: uuidFromString(project.OwnerUserID),
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

// buildProject builds a project with defaults and validation applied.
func buildProject(ownerUserID, name, region string) (Project, error) {
	owner := strings.TrimSpace(ownerUserID)
	if owner == "" {
		return Project{}, errors.New("owner user id is required")
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
		OwnerUserID: owner,
		Name:        name,
		Slug:        slug,
		Region:      region,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// generateName builds a name from a random adjective and noun.
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

// slugifyProjectName normalizes a project name into a URL-safe slug.
func slugifyProjectName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	var prevHyphen bool

	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

// randomSuffix generates a short random suffix.
func randomSuffix(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String()
}

// userFromRow converts a SQLC User row into a service User.
func userFromRow(r pgstore.User) User {
	return User{
		ID:        uuidToString(r.ID),
		GithubID:  r.GithubID,
		CreatedAt: timeFromTimestamptz(r.CreatedAt),
		UpdatedAt: timeFromTimestamptz(r.UpdatedAt),
	}
}

// projectFromRow converts a SQLC Project row into a service Project.
func projectFromRow(r pgstore.Project) Project {
	return Project{
		ID:          uuidToString(r.ID),
		OwnerUserID: uuidToString(r.OwnerUserID),
		Name:        r.Name,
		Slug:        r.Slug,
		Region:      r.Region,
		CreatedAt:   timeFromTimestamptz(r.CreatedAt),
		UpdatedAt:   timeFromTimestamptz(r.UpdatedAt),
	}
}

// uuidFromString converts a string to pgtype.UUID.
func uuidFromString(id string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(id)
	return u
}

// uuidToString converts a pgtype.UUID to a string.
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

// timeFromTimestamptz converts pgtype.Timestamptz to time.Time.
func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

// isUniqueViolation reports if the error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
