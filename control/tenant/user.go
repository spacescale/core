// This file defines user-domain service models and workflows.
// It centralizes user lookup and auth-sync persistence behavior so user lifecycle
// responsibilities remain separate from project creation logic.
// Keeping user workflows in one place clarifies ownership boundaries:
// - UserService owns user identity resolution and profile upsert behavior.
// - ProjectService consumes resolved user IDs and focuses only on projects.

// Package tenant implements control-plane business workflows for tenant-owned resources.
package tenant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spacescale/core/control/db/sqlc"
)

// User represents a persisted user identity.
// The service layer keeps this type database-agnostic so handlers and callers
// do not depend directly on SQLC-generated wrapper types.
type User struct {
	ID          string
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserService provides user identity and profile persistence operations.
type UserService struct {
	queries *sqlc.Queries
}

// NewUserService creates a UserService bound to the provided query set.
func NewUserService(queries *sqlc.Queries) *UserService {
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
	identityKey = strings.TrimSpace(identityKey)
	if identityKey == "" {
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
// Persistence behavior:
//   - Upsert by identity key so repeated sign-ins are idempotent.
//   - Existing non-empty profile fields are preserved; incoming data only
//     populates fields that are currently empty. This prevents OAuth provider
//     switches from overwriting user-preferred profile data.
//   - Optional profile fields are sanitized before persistence.
func (s *UserService) SyncAuthUser(ctx context.Context, p SyncAuthUserParams) (User, error) {
	identityKey := strings.TrimSpace(p.IdentityKey)
	if identityKey == "" {
		return User{}, ErrInvalidInput
	}

	incomingEmail := strings.TrimSpace(p.Email)
	incomingName := strings.TrimSpace(p.Name)
	incomingAvatarURL := strings.TrimSpace(p.AvatarURL)

	// Preserve existing profile fields once set so switching OAuth providers does
	// not continuously overwrite avatar/name on each login.
	existingUser, err := s.queries.GetUserByIdentityKey(ctx, identityKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// Log database errors (connection issues, permission errors, etc.) for debugging.
		// ErrNoRows is expected for new users, so we only log unexpected errors.
		slog.Error(
			"user sync: failed to fetch existing user",
			"identity_ref", identityKeyLogRef(identityKey),
			"error", err,
		)
		return User{}, err
	}
	if err == nil {
		incomingEmail = keepExistingIfPresent(stringValue(existingUser.Email), incomingEmail)
		incomingName = keepExistingIfPresent(stringValue(existingUser.Name), incomingName)
		incomingAvatarURL = keepExistingIfPresent(
			stringValue(existingUser.AvatarUrl),
			incomingAvatarURL,
		)
	}

	row, err := s.queries.UpsertUserByIdentityKey(ctx, sqlc.UpsertUserByIdentityKeyParams{
		IdentityKey: identityKey,
		Email:       stringPtrOrNil(incomingEmail),
		Name:        stringPtrOrNil(incomingName),
		AvatarUrl:   stringPtrOrNil(incomingAvatarURL),
	})
	if err != nil {
		return User{}, err
	}

	return userFromRow(row), nil
}

// userFromRow maps a storage user row into the service user shape.
// This keeps storage types out of higher layers and normalizes optional profile
// fields into plain service strings.
func userFromRow(r sqlc.User) User {
	return User{
		ID:          r.ID.String(),
		IdentityKey: r.IdentityKey,
		Email:       stringValue(r.Email),
		Name:        stringValue(r.Name),
		AvatarURL:   stringValue(r.AvatarUrl),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// stringPtrOrNil returns nil for blank values and a trimmed pointer otherwise.
func stringPtrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// stringValue converts optional DB text fields into service-safe strings.
func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// keepExistingIfPresent keeps the stored value once available, and only uses
// new provider data when the stored value is still empty.
func keepExistingIfPresent(existing, incoming string) string {
	if existing != "" {
		return existing
	}
	return incoming
}

// identityKeyLogRef returns a stable opaque identifier for logs.
// Raw identity keys may include emails, so logs must never emit them directly.
func identityKeyLogRef(identityKey string) string {
	if identityKey == "" {
		return "identity:unknown"
	}

	sum := sha256.Sum256([]byte(identityKey))
	return "identity-hash:" + hex.EncodeToString(sum[:])
}
