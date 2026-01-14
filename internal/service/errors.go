package service

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
	ErrNotFound     = errors.New("not found")

	ErrNoWork    = errors.New("no queued deployments")
	ErrNoRuntime = errors.New("runtime not configured")
)
