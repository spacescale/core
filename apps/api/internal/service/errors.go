// This file defines service-level sentinel errors returned by business workflows.
// Handlers and other callers use these stable values to map failures to user-
// facing behavior without coupling to storage-specific error details.
// Keeping shared error contracts in one file avoids inconsistent comparisons and
// makes cross-layer error handling easier to reason about during maintenance.

// Package service contains business error definitions.
package service

import "errors"

var (
	// ErrInvalidInput indicates request validation failure.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict indicates a conflicting write or duplicate resource.
	ErrConflict = errors.New("conflict")
)
