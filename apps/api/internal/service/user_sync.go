// This file contains user-sync workflow logic used by the web auth callback.
// It upserts user profile fields into persistent storage using a stable auth
// identity key and returns the normalized service user model for callers.
// Keeping this workflow in the service layer keeps HTTP handlers thin and
// centralizes validation and persistence behavior for auth user writes.

package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

// SyncAuthUserParams defines the profile payload accepted from trusted auth
// sync callers.
//
// IdentityKey must be a stable, non-empty identifier that the caller controls.
// Optional profile fields are trimmed and stored as nullable text values.
type SyncAuthUserParams struct {
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
}

// SyncAuthUser persists a user profile by stable identity key and returns the
// normalized service user model.
//
// Validation rules:
// - IdentityKey is required after trimming.
//
// Persistence behavior:
// - Upsert by identity key so repeated sign-ins are idempotent.
// - Profile fields are updated on each successful write.
func (s *ProjectService) SyncAuthUser(ctx context.Context, p SyncAuthUserParams) (User, error) {
	identityKey := strings.TrimSpace(p.IdentityKey)
	if identityKey == "" {
		return User{}, ErrInvalidInput
	}

	incomingEmail := strings.TrimSpace(p.Email)
	incomingName := strings.TrimSpace(p.Name)
	incomingAvatarURL := strings.TrimSpace(p.AvatarURL)

	// Preserve existing profile fields once set so switching OAuth providers does
	// not continuously overwrite avatar/name on each login.
	existingUser, err := s.queries.GetUserByGithubID(ctx, identityKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}
	if err == nil {
		incomingEmail = keepExistingIfPresent(textFromPG(existingUser.Email), incomingEmail)
		incomingName = keepExistingIfPresent(textFromPG(existingUser.Name), incomingName)
		incomingAvatarURL = keepExistingIfPresent(
			textFromPG(existingUser.AvatarUrl),
			incomingAvatarURL,
		)
	}

	row, err := s.queries.UpsertUserByGithubID(ctx, pgstore.UpsertUserByGithubIDParams{
		GithubID:  identityKey,
		Email:     textToPG(incomingEmail),
		Name:      textToPG(incomingName),
		AvatarUrl: textToPG(incomingAvatarURL),
	})
	if err != nil {
		return User{}, err
	}

	return userFromRow(row), nil
}

// textToPG converts plain string values into nullable pgtype.Text.
// Empty strings are normalized to invalid values so storage keeps nullability
// semantics for optional fields.
func textToPG(v string) pgtype.Text {
	if strings.TrimSpace(v) == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: v, Valid: true}
}

// keepExistingIfPresent keeps the stored value once available, and only uses
// new provider data when the stored value is still empty.
func keepExistingIfPresent(existing, incoming string) string {
	trimmedExisting := strings.TrimSpace(existing)
	if trimmedExisting != "" {
		return trimmedExisting
	}
	return strings.TrimSpace(incoming)
}
