// Package service contains business error definitions.
package service

import "errors"

var (
	// ErrInvalidInput indicates request validation failure.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict indicates a conflicting write or duplicate resource.
	ErrConflict = errors.New("conflict")
)
