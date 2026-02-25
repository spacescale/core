// This file provides white-box tests for shared service helpers.

package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestParseUUID(t *testing.T) {
	t.Run("parses trimmed uuid", func(t *testing.T) {
		raw := "  f47ac10b-58cc-4372-a567-0e02b2c3d479  "
		got, ok := parseUUID(raw)
		require.True(t, ok)
		require.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", got.String())
	})

	t.Run("rejects invalid uuid", func(t *testing.T) {
		got, ok := parseUUID("not-a-uuid")
		require.False(t, ok)
		require.Equal(t, uuid.Nil, got)
	})
}

func TestUUIDOrEmpty(t *testing.T) {
	require.Equal(t, "", uuidOrEmpty(uuid.Nil))

	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", uuidOrEmpty(id))
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
		{name: "drops non ascii letters", in: "日本 語", want: ""},
		{name: "mixed unicode and ascii", in: "Project 日本 123", want: "project-123"},
		{name: "caps slug length", in: strings.Repeat("abc", 30), want: strings.Repeat("abc", 21)},
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

	empty, err := randomSuffix(0)
	require.NoError(t, err)
	require.Empty(t, empty)

	got, err := randomSuffix(24)
	require.NoError(t, err)
	require.Len(t, got, 24)
	for _, r := range got {
		require.Truef(t, strings.ContainsRune(alphabet, r), "unexpected suffix rune %q in %q", r, got)
	}
}

func TestNormalizeProjectName(t *testing.T) {
	t.Run("accepts trimmed name", func(t *testing.T) {
		got, ok := normalizeProjectName("  blue harbor  ")
		require.True(t, ok)
		require.Equal(t, "blue harbor", got)
	})

	t.Run("rejects empty", func(t *testing.T) {
		_, ok := normalizeProjectName("   ")
		require.False(t, ok)
	})

	t.Run("rejects over max length", func(t *testing.T) {
		_, ok := normalizeProjectName(strings.Repeat("a", projectNameMaxLength+1))
		require.False(t, ok)
	})
}

func TestNormalizeProjectRegion(t *testing.T) {
	t.Run("defaults to global", func(t *testing.T) {
		got, ok := normalizeProjectRegion(" ")
		require.True(t, ok)
		require.Equal(t, defaultRegion, got)
	})

	t.Run("normalizes to lowercase", func(t *testing.T) {
		got, ok := normalizeProjectRegion("US-EAST-1")
		require.True(t, ok)
		require.Equal(t, "us-east-1", got)
	})

	t.Run("rejects invalid chars", func(t *testing.T) {
		_, ok := normalizeProjectRegion("global!")
		require.False(t, ok)
	})

	t.Run("rejects edge hyphen", func(t *testing.T) {
		_, ok := normalizeProjectRegion("-global")
		require.False(t, ok)
	})

	t.Run("rejects over max length", func(t *testing.T) {
		_, ok := normalizeProjectRegion(strings.Repeat("a", projectRegionMaxLen+1))
		require.False(t, ok)
	})
}

func TestSlugWithSuffix(t *testing.T) {
	t.Run("appends suffix when base fits", func(t *testing.T) {
		got := slugWithSuffix("misty-harbor", "abc123")
		require.Equal(t, "misty-harbor-abc123", got)
		require.LessOrEqual(t, len(got), projectSlugMaxLength)
	})

	t.Run("truncates base to keep dns label length", func(t *testing.T) {
		base := strings.Repeat("a", projectSlugMaxLength)
		got := slugWithSuffix(base, "abc123")
		require.Equal(t, projectSlugMaxLength, len(got))
		require.Equal(t, strings.Repeat("a", projectSlugMaxLength-suffixLength-1)+"-abc123", got)
	})
}

func TestIsUniqueViolation(t *testing.T) {
	dupErr := &pgconn.PgError{Code: "23505"}
	require.True(t, isUniqueViolation(dupErr))

	wrapped := fmt.Errorf("wrapped: %w", dupErr)
	require.True(t, isUniqueViolation(wrapped))

	require.False(t, isUniqueViolation(&pgconn.PgError{Code: "22001"}))
	require.False(t, isUniqueViolation(errors.New("plain error")))
}
