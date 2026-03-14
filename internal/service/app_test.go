// This file provides white-box tests for pure app service helper logic.
//
// Scope:
// - Input normalization and derivation helpers used by create-app workflows.
// - Small mapping helpers that do not require database access.

package service

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

func TestNormalizeImageRef(t *testing.T) {
	t.Run("accepts trimmed image ref", func(t *testing.T) {
		got, ok := normalizeImageRef("  ghcr.io/acme/api:latest  ")
		require.True(t, ok)
		require.Equal(t, "ghcr.io/acme/api:latest", got)
	})

	t.Run("rejects empty", func(t *testing.T) {
		_, ok := normalizeImageRef("   ")
		require.False(t, ok)
	})

	t.Run("rejects whitespace in ref", func(t *testing.T) {
		_, ok := normalizeImageRef("ghcr.io/acme/api :latest")
		require.False(t, ok)
	})

	t.Run("rejects over max length", func(t *testing.T) {
		tooLong := strings.Repeat("a", appImageRefMaxChars+1)
		_, ok := normalizeImageRef(tooLong)
		require.False(t, ok)
	})
}

func TestNormalizeOrDeriveAppName(t *testing.T) {
	t.Run("accepts explicit name", func(t *testing.T) {
		got, ok := normalizeOrDeriveAppName("  API  ", "ghcr.io/acme/ignored:latest")
		require.True(t, ok)
		require.Equal(t, "API", got)
	})

	t.Run("derives from image ref when empty", func(t *testing.T) {
		got, ok := normalizeOrDeriveAppName("", "ghcr.io/acme/worker:latest")
		require.True(t, ok)
		require.Equal(t, "worker", got)
	})

	t.Run("rejects over max length", func(t *testing.T) {
		_, ok := normalizeOrDeriveAppName(strings.Repeat("a", appNameMaxChars+1), "ghcr.io/acme/api:latest")
		require.False(t, ok)
	})

	t.Run("rejects when derive fails", func(t *testing.T) {
		_, ok := normalizeOrDeriveAppName("", "ghcr.io/acme/")
		require.False(t, ok)
	})
}

func TestDeriveAppNameFromImageRef(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		valid bool
	}{
		{name: "repo with tag", in: "ghcr.io/acme/api:latest", want: "api", valid: true},
		{name: "repo with digest", in: "ghcr.io/acme/api@sha256:deadbeef", want: "api", valid: true},
		{name: "single segment with tag", in: "api:v1", want: "api", valid: true},
		{name: "trims whitespace", in: "  ghcr.io/acme/worker:1.2.3  ", want: "worker", valid: true},
		{name: "rejects empty", in: "", want: "", valid: false},
		{name: "rejects trailing slash", in: "ghcr.io/acme/", want: "", valid: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok := deriveAppNameFromImageRef(tc.in)
			require.Equal(t, tc.valid, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeRuntimePort(t *testing.T) {
	t.Run("defaults when nil", func(t *testing.T) {
		got, ok := normalizeRuntimePort(nil)
		require.True(t, ok)
		require.EqualValues(t, defaultAppRuntimePort, got)
	})

	t.Run("accepts valid bounds", func(t *testing.T) {
		min := 1
		max := 65535

		gotMin, okMin := normalizeRuntimePort(&min)
		require.True(t, okMin)
		require.EqualValues(t, 1, gotMin)

		gotMax, okMax := normalizeRuntimePort(&max)
		require.True(t, okMax)
		require.EqualValues(t, 65535, gotMax)
	})

	t.Run("rejects invalid bounds", func(t *testing.T) {
		zero := 0
		tooHigh := 65536

		_, okZero := normalizeRuntimePort(&zero)
		require.False(t, okZero)

		_, okTooHigh := normalizeRuntimePort(&tooHigh)
		require.False(t, okTooHigh)
	})
}

func TestNormalizeIsPublic(t *testing.T) {
	require.False(t, normalizeIsPublic(nil))

	falseVal := false
	require.False(t, normalizeIsPublic(&falseVal))

	trueVal := true
	require.True(t, normalizeIsPublic(&trueVal))
}

func TestNormalizeEnvVarKey(t *testing.T) {
	t.Run("normalizes to uppercase and trims", func(t *testing.T) {
		got, ok := normalizeEnvVarKey("  app_mode  ")
		require.True(t, ok)
		require.Equal(t, "APP_MODE", got)
	})

	t.Run("accepts underscore start", func(t *testing.T) {
		got, ok := normalizeEnvVarKey("_private")
		require.True(t, ok)
		require.Equal(t, "_PRIVATE", got)
	})

	t.Run("rejects invalid start", func(t *testing.T) {
		_, ok := normalizeEnvVarKey("1INVALID")
		require.False(t, ok)
	})

	t.Run("rejects invalid characters", func(t *testing.T) {
		_, ok := normalizeEnvVarKey("API-KEY")
		require.False(t, ok)
	})

	t.Run("rejects over max length", func(t *testing.T) {
		_, ok := normalizeEnvVarKey(strings.Repeat("A", appEnvVarKeyMaxChars+1))
		require.False(t, ok)
	})
}

func TestNormalizeEnvVars(t *testing.T) {
	t.Run("accepts empty slice", func(t *testing.T) {
		got, ok := normalizeEnvVars(nil)
		require.True(t, ok)
		require.Nil(t, got)
	})

	t.Run("normalizes keys and preserves values", func(t *testing.T) {
		got, ok := normalizeEnvVars([]AppEnvVarInput{{
			Key:      " app_mode ",
			Value:    "production",
			IsSecret: false,
		}})
		require.True(t, ok)
		require.Len(t, got, 1)
		require.Equal(t, "APP_MODE", got[0].Key)
		require.Equal(t, "production", got[0].Value)
		require.False(t, got[0].IsSecret)
	})

	t.Run("rejects duplicate keys after normalization", func(t *testing.T) {
		_, ok := normalizeEnvVars([]AppEnvVarInput{
			{Key: "api_key", Value: "a", IsSecret: true},
			{Key: " API_KEY ", Value: "b", IsSecret: true},
		})
		require.False(t, ok)
	})

	t.Run("rejects invalid key", func(t *testing.T) {
		_, ok := normalizeEnvVars([]AppEnvVarInput{{Key: "invalid-key", Value: "x", IsSecret: false}})
		require.False(t, ok)
	})

	t.Run("rejects over max value length", func(t *testing.T) {
		tooLong := strings.Repeat("a", appEnvVarValueMaxRunes+1)
		_, ok := normalizeEnvVars([]AppEnvVarInput{{Key: "APP_MODE", Value: tooLong, IsSecret: false}})
		require.False(t, ok)
	})

	t.Run("rejects over max env var count", func(t *testing.T) {
		raw := make([]AppEnvVarInput, appEnvVarsMaxCount+1)
		for i := 0; i < len(raw); i++ {
			raw[i] = AppEnvVarInput{
				Key:      "KEY_" + strconv.Itoa(i),
				Value:    "x",
				IsSecret: false,
			}
		}
		_, ok := normalizeEnvVars(raw)
		require.False(t, ok)
	})
}

func TestAppFromRow(t *testing.T) {
	appID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	projectID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	created := time.Date(2026, 2, 10, 1, 2, 3, 0, time.UTC)
	updated := created.Add(10 * time.Minute)

	got := appFromRow(pgstore.App{
		ID:          appID,
		ProjectID:   projectID,
		Name:        "api",
		Slug:        "api",
		Subdomain:   "api",
		ImageRef:    "ghcr.io/acme/api:latest",
		RuntimePort: 8080,
		Status:      "queued",
		IsPublic:    true,
		CreatedAt:   created,
		UpdatedAt:   updated,
	})

	require.Equal(t, appID.String(), got.ID)
	require.Equal(t, projectID.String(), got.ProjectID)
	require.Equal(t, "api", got.Name)
	require.Equal(t, "api", got.Slug)
	require.Equal(t, "api", got.Subdomain)
	require.Equal(t, "ghcr.io/acme/api:latest", got.ImageRef)
	require.EqualValues(t, 8080, got.RuntimePort)
	require.Equal(t, "queued", got.Status)
	require.True(t, got.IsPublic)
	require.True(t, got.CreatedAt.Equal(created))
	require.True(t, got.UpdatedAt.Equal(updated))
}
