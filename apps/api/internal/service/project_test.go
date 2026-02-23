// This file provides white-box tests for pure project service logic.
// It intentionally avoids database-fake workflow tests; DB-backed behavior is
// expected to be covered by integration tests against a real database.

package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

func TestBuildProject(t *testing.T) {
	t.Run("applies defaults and normalization", func(t *testing.T) {
		got, err := buildProject(" workspace-1 ", "  My Project  ", " ")
		require.NoError(t, err)
		require.Equal(t, "workspace-1", got.WorkspaceID)
		require.Equal(t, "My Project", got.Name)
		require.Equal(t, defaultRegion, got.Region)
		require.Equal(t, "my-project", got.Slug)
		require.Empty(t, got.ID)
		require.False(t, got.CreatedAt.IsZero())
		require.False(t, got.UpdatedAt.IsZero())
		require.True(t, got.CreatedAt.Equal(got.UpdatedAt))
		require.Equal(t, time.UTC, got.CreatedAt.Location())
	})

	t.Run("rejects missing workspace", func(t *testing.T) {
		_, err := buildProject(" ", "project", "")
		require.Error(t, err)
	})

	t.Run("rejects missing name", func(t *testing.T) {
		_, err := buildProject("owner-1", " ", "")
		require.Error(t, err)
	})

	t.Run("rejects name that cannot produce slug", func(t *testing.T) {
		_, err := buildProject("owner-1", " !!! ", "")
		require.Error(t, err)
	})
}

func TestSlugifyProjectName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple words", in: "Hello World", want: "hello-world"},
		{name: "collapses separators", in: "__Hello---WORLD__", want: "hello-world"},
		{name: "trims boundaries", in: "  ###go### ", want: "go"},
		{name: "keeps numbers", in: "Project 123", want: "project-123"},
		{name: "unicode letters", in: "日本 語", want: "日本-語"},
		{name: "all symbols", in: "!!!", want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := slugifyProjectName(tc.in)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRandomSuffix(t *testing.T) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

	require.Empty(t, randomSuffix(0))

	got := randomSuffix(24)
	require.Len(t, got, 24)
	for _, r := range got {
		require.Truef(t, strings.ContainsRune(alphabet, r), "unexpected suffix rune %q in %q", r, got)
	}
}

func TestProjectFromRow(t *testing.T) {
	wid := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	pid := mustUUID(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	created := time.Date(2026, 2, 9, 1, 2, 3, 0, time.UTC)
	updated := created.Add(5 * time.Minute)

	gotProject := projectFromRow(pgstore.Project{
		ID:          pid,
		WorkspaceID: wid,
		Name:        "misty-harbor",
		Slug:        "misty-harbor",
		Region:      "global",
		CreatedAt:   created,
		UpdatedAt:   updated,
	})
	require.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", gotProject.ID)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", gotProject.WorkspaceID)
	require.Equal(t, "misty-harbor", gotProject.Name)
	require.Equal(t, "global", gotProject.Region)
	require.True(t, gotProject.CreatedAt.Equal(created))
	require.True(t, gotProject.UpdatedAt.Equal(updated))
}

func TestIsUniqueViolation(t *testing.T) {
	dupErr := &pgconn.PgError{Code: "23505"}
	require.True(t, isUniqueViolation(dupErr))

	wrapped := fmt.Errorf("wrapped: %w", dupErr)
	require.True(t, isUniqueViolation(wrapped))

	require.False(t, isUniqueViolation(&pgconn.PgError{Code: "22001"}))
	require.False(t, isUniqueViolation(errors.New("plain error")))
}

func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(raw)
	require.NoError(t, err)
	return u
}
