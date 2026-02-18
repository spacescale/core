// This file defines the service-layer user model shared by business workflows.
// It represents the normalized user shape that handlers and services pass
// around, independent of database driver wrapper types.
// Keeping this model small and plain makes it easier for new contributors to
// understand which user fields are considered part of the service contract.
// Database-specific conversions should stay in mapping helpers so this struct
// remains stable even if persistence implementation details change.

// Package service defines core models for service workflows.
package service

import "time"

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
