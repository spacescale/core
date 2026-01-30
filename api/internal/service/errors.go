// Service error definitions for HTTP mapping.
package service

import "errors"

// Service level errors returned to handlers for consistent mapping.
// These allow the transport layer to translate outcomes into HTTP status codes.
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
	ErrNotFound     = errors.New("not found")

	ErrNoWork    = errors.New("no queued deployments")
	ErrNoRuntime = errors.New("runtime not configured")
)
