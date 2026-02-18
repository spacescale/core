// This file defines user-domain service models and workflows.
// It centralizes user lookup and auth-sync persistence behavior so user lifecycle
// responsibilities remain separate from project creation logic.
// Keeping user workflows in one place clarifies ownership boundaries:
// - UserService owns user identity resolution and profile upsert behavior.
// - ProjectService consumes resolved user IDs and focuses only on projects.

// Package service defines core models for service workflows.
package service

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

const (
	maxIdentityKeyChars = 512
	maxEmailChars       = 320
	maxNameChars        = 255
	maxAvatarURLChars   = 2048
)

// User represents a persisted user identity.
// The service layer keeps this type database-agnostic so handlers and callers
// do not depend directly on SQLC-generated wrapper types.
type User struct {
	ID                  string
	IdentityKey         string
	Email               string
	Name                string
	AvatarURL           string
	OnboardingCompleted bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// UserService provides user identity and profile persistence operations.
type UserService struct {
	queries *pgstore.Queries
}

// NewUserService creates a UserService bound to the provided query set.
func NewUserService(queries *pgstore.Queries) *UserService {
	return &UserService{queries: queries}
}

// SyncAuthUserParams defines the profile payload accepted from trusted auth
// sync callers.
//
// IdentityKey must be a stable, non-empty identifier that the caller controls.
// Optional profile fields are sanitized and stored as nullable text values.
type SyncAuthUserParams struct {
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
}

// GetUserByIdentityKey returns a synced user for the provided identity key.
// Missing rows are mapped to ErrUnauthorized so callers can treat unknown
// identities as unauthorized request contexts.
func (s *UserService) GetUserByIdentityKey(ctx context.Context, identityKey string) (User, error) {
	identityKey, ok := normalizeIdentityKey(identityKey)
	if !ok {
		return User{}, ErrInvalidInput
	}

	row, err := s.queries.GetUserByIdentityKey(ctx, identityKey)
	if err == nil {
		return userFromRow(row), nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	return User{}, err
}

// SyncAuthUser persists a user profile by stable identity key and returns the
// normalized service user model.
//
// Validation rules:
// - IdentityKey is required after trimming.
// - IdentityKey must not exceed maxIdentityKeyChars.
//
// Persistence behavior:
//   - Upsert by identity key so repeated sign-ins are idempotent.
//   - Existing non-empty profile fields are preserved; incoming data only
//     populates fields that are currently empty. This prevents OAuth provider
//     switches from overwriting user-preferred profile data.
//   - Optional profile fields are sanitized before persistence.
func (s *UserService) SyncAuthUser(ctx context.Context, p SyncAuthUserParams) (User, error) {
	identityKey, ok := normalizeIdentityKey(p.IdentityKey)
	if !ok {
		return User{}, ErrInvalidInput
	}

	incomingEmail := sanitizeEmail(p.Email)
	incomingName := sanitizeName(p.Name)
	incomingAvatarURL := sanitizeAvatarURL(p.AvatarURL)

	// Preserve existing profile fields once set so switching OAuth providers does
	// not continuously overwrite avatar/name on each login.
	existingUser, err := s.queries.GetUserByIdentityKey(ctx, identityKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// Log database errors (connection issues, permission errors, etc.) for debugging.
		// ErrNoRows is expected for new users, so we only log unexpected errors.
		slog.Error(
			"user sync: failed to fetch existing user",
			"identity_key", identityKey,
			"error", err,
		)
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

	row, err := s.queries.UpsertUserByIdentityKey(ctx, pgstore.UpsertUserByIdentityKeyParams{
		IdentityKey: identityKey,
		Email:       textToPG(incomingEmail),
		Name:        textToPG(incomingName),
		AvatarUrl:   textToPG(incomingAvatarURL),
	})
	if err != nil {
		return User{}, err
	}

	return userFromRow(row), nil
}

// userFromRow maps a storage user row into the service user shape.
// This keeps database-specific wrapper types out of higher layers and normalizes
// UUID and timestamp fields into plain service values.
func userFromRow(r pgstore.User) User {
	return User{
		ID:                  uuidToString(r.ID),
		IdentityKey:         r.IdentityKey,
		Email:               textFromPG(r.Email),
		Name:                textFromPG(r.Name),
		AvatarURL:           textFromPG(r.AvatarUrl),
		OnboardingCompleted: r.OnboardingCompleted,
		CreatedAt:           timeFromTimestamptz(r.CreatedAt),
		UpdatedAt:           timeFromTimestamptz(r.UpdatedAt),
	}
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

// normalizeIdentityKey validates and trims identity keys from trusted callers.
func normalizeIdentityKey(raw string) (string, bool) {
	identityKey := strings.TrimSpace(raw)
	if identityKey == "" {
		return "", false
	}
	if utf8.RuneCountInString(identityKey) > maxIdentityKeyChars {
		return "", false
	}
	return identityKey, true
}

// sanitizeEmail normalizes optional email input.
// Invalid or oversized values are dropped instead of failing auth-sync.
func sanitizeEmail(raw string) string {
	email := strings.TrimSpace(raw)
	if email == "" {
		return ""
	}
	if utf8.RuneCountInString(email) > maxEmailChars {
		return ""
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return ""
	}

	email = strings.TrimSpace(parsed.Address)
	if email == "" {
		return ""
	}
	if utf8.RuneCountInString(email) > maxEmailChars {
		return ""
	}

	return strings.ToLower(email)
}

// sanitizeName trims and bounds optional display names.
func sanitizeName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	return truncateRunes(name, maxNameChars)
}

// sanitizeAvatarURL validates optional avatar URLs.
// Only absolute http/https URLs within max length are persisted.
func sanitizeAvatarURL(raw string) string {
	avatarURL := strings.TrimSpace(raw)
	if avatarURL == "" {
		return ""
	}
	if utf8.RuneCountInString(avatarURL) > maxAvatarURLChars {
		return ""
	}

	u, err := url.Parse(avatarURL)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}

	return avatarURL
}

// truncateRunes truncates to a maximum rune count.
func truncateRunes(in string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(in)
	if len(runes) <= max {
		return in
	}
	return string(runes[:max])
}
