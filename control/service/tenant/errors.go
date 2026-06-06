// This file defines service-level sentinel errors returned by business workflows.
// Handlers and other callers use these stable values to map failures to user-
// facing behavior without coupling to storage-specific error details.
// Keeping shared error contracts in one file avoids inconsistent comparisons and
// makes cross-layer error handling easier to reason about during maintenance.

// Package tenant implements control-plane business workflows for tenant-owned resources.
package tenant

import "errors"

var (
	// ErrInvalidInput reports that request validation failed before persistence.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict reports a conflicting write or duplicate resource.
	ErrConflict = errors.New("conflict")
	// ErrUnauthorized reports that the caller is not allowed to access the requested resource.
	ErrUnauthorized = errors.New("unauthorized")
)
