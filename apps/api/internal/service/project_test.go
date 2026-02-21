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

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

func TestBuildProject(t *testing.T) {
	t.Run("applies defaults and normalization", func(t *testing.T) {
		got, err := buildProject(" owner-1 ", "  My Project  ", " ")
		require.NoError(t, err)
		require.Equal(t, "owner-1", got.OwnerUserID)
		require.Equal(t, "My Project", got.Name)
		require.Equal(t, defaultRegion, got.Region)
		require.Equal(t, "my-project", got.Slug)
		require.Empty(t, got.ID)
		require.False(t, got.CreatedAt.IsZero())
		require.False(t, got.UpdatedAt.IsZero())
		require.True(t, got.CreatedAt.Equal(got.UpdatedAt))
		require.Equal(t, time.UTC, got.CreatedAt.Location())
	})

	t.Run("rejects missing owner", func(t *testing.T) {
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

func TestTextAndTimeHelpers(t *testing.T) {
	require.Equal(t, "dev@example.com", textFromPG(pgtype.Text{String: "dev@example.com", Valid: true}))
	require.Empty(t, textFromPG(pgtype.Text{Valid: false}))

	ts := time.Date(2026, 2, 9, 10, 20, 30, 0, time.UTC)
	require.True(t, timeFromTimestamptz(pgtype.Timestamptz{Time: ts, Valid: true}).Equal(ts))
	require.True(t, timeFromTimestamptz(pgtype.Timestamptz{Valid: false}).IsZero())
}

func TestUUIDHelpers(t *testing.T) {
	validID := "550e8400-e29b-41d4-a716-446655440000"
	u := uuidFromString(validID)
	require.True(t, u.Valid)
	require.Equal(t, validID, uuidToString(u))

	invalid := uuidFromString("not-a-uuid")
	require.False(t, invalid.Valid)
	require.Empty(t, uuidToString(invalid))
}

func TestProjectFromRow(t *testing.T) {
	uid := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	pid := mustUUID(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	created := time.Date(2026, 2, 9, 1, 2, 3, 0, time.UTC)
	updated := created.Add(5 * time.Minute)

	gotProject := projectFromRow(pgstore.Project{
		ID:          pid,
		OwnerUserID: uid,
		Name:        "misty-harbor",
		Slug:        "misty-harbor",
		Region:      "global",
		CreatedAt:   pgtype.Timestamptz{Time: created, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: updated, Valid: true},
	})
	require.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", gotProject.ID)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", gotProject.OwnerUserID)
	require.Equal(t, "misty-harbor", gotProject.Name)
	require.Equal(t, "global", gotProject.Region)
}

func TestIsUniqueViolation(t *testing.T) {
	dupErr := &pgconn.PgError{Code: "23505"}
	require.True(t, isUniqueViolation(dupErr))

	wrapped := fmt.Errorf("wrapped: %w", dupErr)
	require.True(t, isUniqueViolation(wrapped))

	require.False(t, isUniqueViolation(&pgconn.PgError{Code: "22001"}))
	require.False(t, isUniqueViolation(errors.New("plain error")))
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	require.NoError(t, u.Scan(raw))
	return u
}
