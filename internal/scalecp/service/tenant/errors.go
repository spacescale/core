// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// This file defines service-level sentinel errors returned by business workflows.
// Handlers and other callers use these stable values to map failures to user-
// facing behavior without coupling to storage-specific error details.
// Keeping shared error contracts in one file avoids inconsistent comparisons and
// makes cross-layer error handling easier to reason about during maintenance.

// Package service contains business error definitions.
package tenant

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input") // request validation failed.
	ErrConflict     = errors.New("conflict")      // conflicting write or duplicate resource.
	ErrUnauthorized = errors.New("unauthorized")  // caller identity is not allowed for requested operation.
)
