package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/paas/internal/adapters/store"
	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
	"github.com/t0gun/paas/internal/usecase"
)

func TestDeployApp(t *testing.T) {
	tests := []struct {
		label     string
		appExists bool
		appID     string
		ok        bool
		err       error
	}{
		{label: "invalid input: empty app id", appExists: false, appID: "", ok: false, err: usecase.ErrInvalidInput},
		{label: "not found: app missing", appExists: false, appID: "missing", ok: false, err: usecase.ErrNotFound},
		{label: "ok: queues deployment", appExists: true, appID: "", ok: true}, // we will create an app, use its ID
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			svc := usecase.NewAppService(st)
			appID := tt.appID

			if tt.appExists {
				app, err := domain.NewApp(domain.NewAppParams{
					Name:  "hello",
					Image: "nginx:latest",
					Port:  8080,
				})
				assert.NoError(t, err)
				assert.NoError(t, st.CreateApp(ctx, app))
				appID = app.ID
			}
			dep, err := svc.DeployApp(ctx, usecase.DeployAppParams{AppID: appID})
			if tt.ok {
				assert.NoError(t, err)
				assert.NotEmpty(t, dep.ID)
				assert.Equal(t, appID, dep.AppID)
				assert.Equal(t, domain.DeploymentStatusQueued, dep.Status)
				assert.False(t, dep.CreatedAt.IsZero())
				assert.False(t, dep.UpdatedAt.IsZero())
				assert.WithinDuration(t, time.Now().UTC(), dep.CreatedAt, 2*time.Second)
			}

			if !tt.ok {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				assert.Empty(t, dep.ID)
			}

		})
	}
}

type storeWithHooks struct {
	contracts.Store
	getAppErr    error
	createDepErr error
}

func (s storeWithHooks) GetAppByID(ctx context.Context, id string) (domain.App, error) {
	if s.getAppErr != nil {
		return domain.App{}, s.getAppErr
	}
	return s.Store.GetAppByID(ctx, id)
}

func (s storeWithHooks) CreateDeployment(ctx context.Context, dep domain.Deployment) error {
	if s.createDepErr != nil {
		return s.createDepErr
	}
	return s.Store.CreateDeployment(ctx, dep)
}

func TestDeployApp_StoreErrors(t *testing.T) {
	tests := []struct {
		label        string
		getAppErr    error
		createDepErr error
		ok           bool
		wantErr      error
	}{
		{label: "GetAppByID unexpected error bubbles up", getAppErr: errors.New("boom"), ok: false},
		{label: "CreateDeployment unexpected error bubbles up", createDepErr: errors.New("boom"), ok: false},
		{label: "CreateDeployment not found maps to ErrNotFound", createDepErr: contracts.ErrNotFound, wantErr: usecase.ErrNotFound, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := context.Background()
			mem := store.NewMemoryStore()
			app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
			assert.NoError(t, err)
			assert.NoError(t, mem.CreateApp(ctx, app))

			st := storeWithHooks{
				Store:        mem,
				getAppErr:    tt.getAppErr,
				createDepErr: tt.createDepErr,
			}

			svc := usecase.NewAppService(st)
			dep, err := svc.DeployApp(ctx, usecase.DeployAppParams{AppID: app.ID})
			assert.Error(t, err)
			assert.Empty(t, dep.ID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}

		})
	}
}
