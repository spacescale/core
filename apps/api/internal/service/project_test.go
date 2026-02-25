// This file provides white-box tests for pure project service logic.
// It intentionally avoids database-fake workflow tests; DB-backed behavior is
// expected to be covered by integration tests against a real database.

package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
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

func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(raw)
	require.NoError(t, err)
	return u
}
