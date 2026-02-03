// Package service defines core models for service workflows.
package service

import "time"

// User represents a persisted user identity.
type User struct {
	ID        string
	GithubID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}
