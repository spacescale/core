package service

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

func TestSyncAuthUserRejectsEmptyIdentity(t *testing.T) {
	svc := &UserService{}

	_, err := svc.SyncAuthUser(t.Context(), SyncAuthUserParams{IdentityKey: "   "})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestSyncAuthUserRejectsTooLongIdentity(t *testing.T) {
	svc := &UserService{}
	tooLongIdentity := strings.Repeat("a", maxIdentityKeyChars+1)

	_, err := svc.SyncAuthUser(t.Context(), SyncAuthUserParams{IdentityKey: tooLongIdentity})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestGetUserByIdentityKeyRejectsEmptyIdentity(t *testing.T) {
	svc := &UserService{}

	_, err := svc.GetUserByIdentityKey(t.Context(), "   ")

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestGetUserByIdentityKeyRejectsTooLongIdentity(t *testing.T) {
	svc := &UserService{}
	tooLongIdentity := strings.Repeat("a", maxIdentityKeyChars+1)

	_, err := svc.GetUserByIdentityKey(t.Context(), tooLongIdentity)

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestSanitizeEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "   ", want: ""},
		{name: "invalid email is dropped", in: "not-an-email", want: ""},
		{name: "valid email is normalized", in: "  Dev@Example.COM  ", want: "dev@example.com"},
		{name: "too long email is dropped", in: strings.Repeat("a", maxEmailChars+1), want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sanitizeEmail(tc.in))
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tooLong := strings.Repeat("a", maxNameChars+10)
	got := sanitizeName(tooLong)
	require.Len(t, []rune(got), maxNameChars)

	require.Equal(t, "Dev", sanitizeName("  Dev  "))
	require.Equal(t, "", sanitizeName("   "))
}

func TestSanitizeAvatarURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "   ", want: ""},
		{name: "invalid url is dropped", in: "not-a-url", want: ""},
		{name: "non-http scheme is dropped", in: "javascript:alert(1)", want: ""},
		{name: "valid https url is kept", in: "https://example.com/avatar.png", want: "https://example.com/avatar.png"},
		{name: "too long url is dropped", in: "https://example.com/" + strings.Repeat("a", maxAvatarURLChars+1), want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sanitizeAvatarURL(tc.in))
		})
	}
}

func TestTextToPG(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want pgtype.Text
	}{
		{name: "empty becomes null text", in: "", want: pgtype.Text{Valid: false}},
		{name: "whitespace becomes null text", in: "   ", want: pgtype.Text{Valid: false}},
		{name: "non-empty remains valid", in: "dev@example.com", want: pgtype.Text{String: "dev@example.com", Valid: true}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, textToPG(tc.in))
		})
	}
}

func TestKeepExistingIfPresent(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		incoming string
		want     string
	}{
		{name: "keeps existing when present", existing: "stored@example.com", incoming: "incoming@example.com", want: "stored@example.com"},
		{name: "trims existing when present", existing: "  stored@example.com  ", incoming: "incoming@example.com", want: "stored@example.com"},
		{name: "uses incoming when existing empty", existing: "   ", incoming: "  incoming@example.com  ", want: "incoming@example.com"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, keepExistingIfPresent(tc.existing, tc.incoming))
		})
	}
}

func TestUserFromRow(t *testing.T) {
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	created := time.Date(2026, 2, 9, 1, 2, 3, 0, time.UTC)
	updated := created.Add(5 * time.Minute)

	got := userFromRow(pgstore.User{
		ID:          id,
		IdentityKey: "12345",
		Email:       pgtype.Text{String: "dev@example.com", Valid: true},
		Name:        pgtype.Text{String: "Dev", Valid: true},
		AvatarUrl:   pgtype.Text{String: "https://example.com/avatar.png", Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: created, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: updated, Valid: true},
	})

	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", got.ID)
	require.Equal(t, "12345", got.IdentityKey)
	require.Equal(t, "dev@example.com", got.Email)
	require.Equal(t, "Dev", got.Name)
	require.Equal(t, "https://example.com/avatar.png", got.AvatarURL)
	require.False(t, got.OnboardingCompleted)
	require.True(t, got.CreatedAt.Equal(created))
	require.True(t, got.UpdatedAt.Equal(updated))
}
