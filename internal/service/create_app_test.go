package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/paas/internal/adapters/store"
	"github.com/t0gun/paas/internal/service"
)

func TestCreateApp(t *testing.T) {
	tests := []struct {
		label string
		name  string
		image string
		port  int
		ok    bool
		err   error
	}{
		{label: "valid", name: "hello", image: "nginx:latest", port: 8080, ok: true},
		{label: "invalid name", name: "Bad_Name", image: "nginx:latest", port: 8080, ok: false, err: service.ErrInvalidInput},
		{label: "empty image", name: "hello", image: "", port: 8080, ok: false, err: service.ErrInvalidInput},
		{label: "invalid port", name: "hello", image: "nginx:latest", port: 0, ok: false, err: service.ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			svc := service.NewAppService(st)
			app, err := svc.CreateApp(ctx, service.CreateAppParams{
				Name:  tt.name,
				Image: tt.image,
				Port:  tt.port,
			})
			if tt.ok {
				assert.NoError(t, err)
				assert.NotEmpty(t, app.ID)
				assert.Equal(t, tt.name, app.Name)
				assert.Equal(t, tt.image, app.Image)
				assert.Equal(t, tt.port, app.Port)
			}

			if !tt.ok {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				assert.Empty(t, app.ID)
			}
		})
	}

}

func TestCreateApp_DuplicateName(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	svc := service.NewAppService(st)

	_, err := svc.CreateApp(ctx, service.CreateAppParams{
		Name:  "hello",
		Image: "nginx:latest",
		Port:  8080,
	})
	assert.NoError(t, err)

	_, err = svc.CreateApp(ctx, service.CreateAppParams{
		Name:  "hello",
		Image: "nginx:latest",
		Port:  8080,
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, service.ErrConflict)
}
