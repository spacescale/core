// This file provides white-box tests for pure workspace service helpers.
// Persistence behavior is intentionally validated through HTTP integration tests.

package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

func TestNormalizeWorkspaceName(t *testing.T) {
	t.Run("trims and keeps valid name", func(t *testing.T) {
		got, ok := normalizeWorkspaceName("  workspace-01  ")
		require.True(t, ok)
		require.Equal(t, "workspace-01", got)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, ok := normalizeWorkspaceName("   ")
		require.False(t, ok)
	})

	t.Run("rejects name above rune limit", func(t *testing.T) {
		_, ok := normalizeWorkspaceName(strings.Repeat("a", maxWorkspaceNameChars+1))
		require.False(t, ok)
	})
}

func TestWorkspaceFromRow(t *testing.T) {
	id := mustUUID(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	created := time.Date(2026, 2, 9, 1, 2, 3, 0, time.UTC)
	updated := created.Add(10 * time.Minute)

	got := workspaceFromRow(pgstore.Workspace{
		ID:        id,
		Name:      "workspace-01",
		CreatedAt: created,
		UpdatedAt: updated,
	})

	require.Equal(t, id.String(), got.ID)
	require.Equal(t, "workspace-01", got.Name)
	require.True(t, got.CreatedAt.Equal(created))
	require.True(t, got.UpdatedAt.Equal(updated))
}
