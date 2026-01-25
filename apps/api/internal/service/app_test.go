// Tests for app service create and get behavior
// Tests include optional port expose and env handling
// Tests verify service error mapping
// Tests cover duplicate name conflicts
// These tests keep service behavior stable

package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/spacescale/internal/adapters/store"
	"github.com/t0gun/spacescale/internal/service"
)

// TestCreateApp validates app creation behavior.
func TestCreateApp(t *testing.T) {
	tests := []struct {
		label  string
		name   string
		image  string
		port   *int
		expose *bool
		env    map[string]string
		ok     bool
		err    error
	}{
		{label: "valid", name: "hello", image: "nginx:latest", port: ptrInt(8080), ok: true},
		{label: "valid no port", name: "hello", image: "nginx:latest", ok: true},
		{label: "valid expose false", name: "hello", image: "nginx:latest", expose: ptrBool(false), ok: true},
		{label: "valid env", name: "hello", image: "nginx:latest", port: ptrInt(8080), env: map[string]string{"KEY": "VALUE"}, ok: true},
		{label: "invalid name", name: "Bad_Name", image: "nginx:latest", port: ptrInt(8080), ok: false, err: service.ErrInvalidInput},
		{label: "empty image", name: "hello", image: "", port: ptrInt(8080), ok: false, err: service.ErrInvalidInput},
		{label: "invalid port", name: "hello", image: "nginx:latest", port: ptrInt(0), ok: false, err: service.ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			svc := service.NewAppService(st)
			app, err := svc.CreateApp(ctx, service.CreateAppParams{
				Name:   tt.name,
				Image:  tt.image,
				Port:   tt.port,
				Expose: tt.expose,
				Env:    tt.env,
			})
			if tt.ok {
				assert.NoError(t, err)
				assert.NotEmpty(t, app.ID)
				assert.Equal(t, tt.name, app.Name)
				assert.Equal(t, tt.image, app.Image)
				if tt.port == nil {
					assert.Nil(t, app.Port)
				} else {
					assert.NotNil(t, app.Port)
					assert.Equal(t, *tt.port, *app.Port)
				}
				if tt.expose == nil {
					assert.True(t, app.Expose)
				} else {
					assert.Equal(t, *tt.expose, app.Expose)
				}
				if tt.env == nil {
					assert.Nil(t, app.Env)
				} else {
					assert.Equal(t, tt.env, app.Env)
				}
			}

			if !tt.ok {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				assert.Empty(t, app.ID)
			}
		})
	}

}

// TestCreateApp_DuplicateName verifies conflicts on duplicate names.
func TestCreateApp_DuplicateName(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	svc := service.NewAppService(st)

	_, err := svc.CreateApp(ctx, service.CreateAppParams{
		Name:  "hello",
		Image: "nginx:latest",
		Port:  ptrInt(8080),
	})
	assert.NoError(t, err)

	_, err = svc.CreateApp(ctx, service.CreateAppParams{
		Name:  "hello",
		Image: "nginx:latest",
		Port:  ptrInt(8080),
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, service.ErrConflict)
}

// TestGetAppByID verifies get-by-id behavior.
func TestGetAppByID(t *testing.T) {
	t.Run("invalid input: empty id", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		svc := service.NewAppService(st)

		app, err := svc.GetAppByID(ctx, "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidInput)
		assert.Empty(t, app.ID)
	})

	t.Run("not found", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		svc := service.NewAppService(st)

		app, err := svc.GetAppByID(ctx, "missing")
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrNotFound)
		assert.Empty(t, app.ID)
	})

	t.Run("ok", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		svc := service.NewAppService(st)

		created, err := svc.CreateApp(ctx, service.CreateAppParams{
			Name:  "hello",
			Image: "nginx:latest",
			Port:  ptrInt(8080),
		})
		assert.NoError(t, err)
		assert.NotEmpty(t, created.ID)

		app, err := svc.GetAppByID(ctx, created.ID)
		assert.NoError(t, err)
		assert.Equal(t, created.ID, app.ID)
		assert.Equal(t, created.Name, app.Name)
		assert.Equal(t, created.Image, app.Image)
		assert.Equal(t, created.Port, app.Port)
	})
}
