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

func TestIsUniqueViolation(t *testing.T) {
	dupErr := &pgconn.PgError{Code: "23505"}
	require.True(t, isUniqueViolation(dupErr))

	wrapped := fmt.Errorf("wrapped: %w", dupErr)
	require.True(t, isUniqueViolation(wrapped))

	require.False(t, isUniqueViolation(&pgconn.PgError{Code: "22001"}))
	require.False(t, isUniqueViolation(errors.New("plain error")))
}
