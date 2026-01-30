// Tests for app name image and port validation
// Tests include valid and invalid examples
// Port tests include nil and range checks
// Validation errors are expected for bad inputs
// These tests guard validation rules

package domain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/spacescale/internal/domain"
)

// TestValidateAppName verifies app name validation.
func TestValidateAppName(t *testing.T) {
	tests := []struct {
		name  string
		ok    bool
		label string
	}{
		{name: "hello", ok: true, label: "simple"},
		{name: "hello-1", ok: true, label: "hyphen-number"},
		{name: "my-app-2", ok: true, label: "hyphenated"},

		{name: "Hello", ok: false, label: "uppercase not allowed"},
		{name: "hello_world", ok: false, label: "underscore not allowed"},
		{name: "-bad", ok: false, label: "starts with hyphen"},
		{name: "bad--name", ok: false, label: "double hyphen"},
		{name: "", ok: false, label: "empty"},
		{name: " ", ok: false, label: "space"},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			err := domain.ValidateAppName(tc.name)

			if tc.ok {
				assert.NoError(t, err, "expected valid name: %q", tc.name)
			}
			if !tc.ok {
				assert.Error(t, err, "expected invalid name: %q", tc.name)
			}
		})
	}
}

// TestValidateImageRef verifies image reference validation.
func TestValidateImageRef(t *testing.T) {
	tests := []struct {
		label string
		image string
		ok    bool
	}{
		{"dockerhub simple tag", "nginx:latest", true},
		{"ghcr full ref", "ghcr.io/user/app:1.0.0", true},
		{"public ecr", "public.ecr.aws/nginx/nginx:latest", true},

		{"empty", "", false},
		{"spaces", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			err := domain.ValidateImageRef(tt.image)

			if tt.ok {
				assert.NoError(t, err, "expected ok for %q", tt.image)
			}
			if !tt.ok {
				assert.Error(t, err, "expected error for %q", tt.image)
			}
		})
	}
}

// TestValidatePort verifies port validation.
func TestValidatePort(t *testing.T) {
	tests := []struct {
		label string
		port  *int
		ok    bool
	}{
		{"typical app port", ptrInt(8080), true},
		{"http", ptrInt(80), true},
		{"min", ptrInt(1), true},
		{"max", ptrInt(65535), true},
		{"nil", nil, true},

		{"zero", ptrInt(0), false},
		{"negative", ptrInt(-1), false},
		{"too high", ptrInt(65536), false},
	}

	for _, tt := range tests {
		name := tt.label
		if tt.port != nil {
			name = fmt.Sprintf("%s (%d)", tt.label, *tt.port)
		}
		t.Run(name, func(t *testing.T) {
			err := domain.ValidatePort(tt.port)

			if tt.ok {
				assert.NoError(t, err)
			}
			if !tt.ok {
				assert.Error(t, err)
			}

		})
	}
}
