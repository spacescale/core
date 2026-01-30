// Contract error values shared by adapters and services.
package contracts

import "errors"

// Contract errors returned by adapters for consistent mapping.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)
