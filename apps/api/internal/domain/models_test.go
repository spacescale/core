package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/paas/internal/domain"
)

func TestNewAppDefault(t *testing.T) {
	tests := []struct {
		label string
		in    domain.NewAppParams
		ok    bool
	}{
		{label: "valid app", in: domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080}, ok: true},
		{label: "invalid name", in: domain.NewAppParams{Name: "Bad_Name", Image: "nginx:latest", Port: 8080}, ok: false},
		{label: "empty image", in: domain.NewAppParams{Name: "hello", Image: "", Port: 8080}, ok: false},
		{label: "invalid port", in: domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 0}, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			app, err := domain.NewApp(tt.in)

			if tt.ok {
				assert.NoError(t, err)
				assert.NotEmpty(t, app.ID)
				assert.Equal(t, tt.in.Name, app.Name)
				assert.Equal(t, tt.in.Port, app.Port)
				assert.Equal(t, tt.in.Image, app.Image)
				assert.Equal(t, domain.AppStatusCreated, app.Status)
			}

			if !tt.ok {
				assert.Error(t, err)
				assert.Empty(t, app.ID)
			}

		})
	}
}

func TestNewDeployment(t *testing.T) {
	tests := []struct {
		label string
		appID string
	}{
		{"normal", "app-123"},
		{"uuid-like", "8e6c9a65-6f01-4db3-a3c2-9c9c9c9c9c9c"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			dep := domain.NewDeployment(tt.appID)

			assert.NotEmpty(t, dep.ID)
			assert.Equal(t, tt.appID, dep.AppID)
			assert.Equal(t, domain.DeploymentStatusQueued, dep.Status)

			assert.False(t, dep.CreatedAt.IsZero())
			assert.False(t, dep.UpdatedAt.IsZero())
			assert.WithinDuration(t, time.Now().UTC(), dep.CreatedAt, 2*time.Second)
		})
	}
}
