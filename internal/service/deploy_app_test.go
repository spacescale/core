package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/paas/internal/adapters/store"
	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
	"github.com/t0gun/paas/internal/service"
)

func TestDeployApp(t *testing.T) {
	tests := []struct {
		label     string
		appExists bool
		appID     string
		ok        bool
		err       error
	}{
		{label: "invalid input: empty app id", appExists: false, appID: "", ok: false, err: service.ErrInvalidInput},
		{label: "not found: app missing", appExists: false, appID: "missing", ok: false, err: service.ErrNotFound},
		{label: "ok: queues deployment", appExists: true, appID: "", ok: true}, // we will create an app, use its ID
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			svc := service.NewAppService(st)
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
			dep, err := svc.DeployApp(ctx, service.DeployAppParams{AppID: appID})
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
		{label: "CreateDeployment not found maps to ErrNotFound", createDepErr: contracts.ErrNotFound, wantErr: service.ErrNotFound, ok: false},
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

			svc := service.NewAppService(st)
			dep, err := svc.DeployApp(ctx, service.DeployAppParams{AppID: app.ID})
			assert.Error(t, err)
			assert.Empty(t, dep.ID)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}

		})
	}
}

type fakeRuntime struct {
	url    string
	err    error
	called int
}

func (f *fakeRuntime) Deploy(ctx context.Context, app domain.App) (string, error) {
	f.called++
	return f.url, f.err
}

var _ contracts.Runtime = (*fakeRuntime)(nil)

func TestProcessNextDeployment(t *testing.T) {
	tests := []struct {
		label      string
		queueWork  bool
		runtimeErr error
		ok         bool
		wantErr    error
	}{
		{label: "no queued deployments", queueWork: false, ok: false, wantErr: service.ErrNoWork},
		{label: "runtime fails", queueWork: true, runtimeErr: errors.New("boom"), ok: false},
		{label: "runtime ok", queueWork: true, runtimeErr: nil, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()

			rt := &fakeRuntime{
				url: "https://hello.yourdomain.come",
				err: tt.runtimeErr,
			}
			svc := service.NewAppServiceWithRuntime(st, rt)

			var app domain.App
			if tt.queueWork {
				var err error
				app, err = domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
				assert.NoError(t, err)
				assert.NoError(t, st.CreateApp(ctx, app))
				dep := domain.NewDeployment(app.ID)
				assert.NoError(t, st.CreateDeployment(ctx, dep))
			}
			dep, err := svc.ProcessNextDeployment(ctx)
			if tt.ok {
				assert.NoError(t, err)
				assert.Equal(t, domain.DeploymentStatusRunning, dep.Status)
				assert.NotEmpty(t, dep.ID)
				assert.NotNil(t, dep.URL)
				assert.Equal(t, rt.url, *dep.URL)
				assert.Nil(t, dep.Error)
				assert.Equal(t, 1, rt.called)
				assert.WithinDuration(t, time.Now().UTC(), dep.UpdatedAt, 2*time.Second)
				return
			}
			assert.Error(t, err)

			// the "no work" case returns empty deployment and runtime not called
			if tt.wantErr != nil {
				assert.Error(t, err, tt.wantErr)
				assert.Empty(t, dep.ID)
				assert.Empty(t, dep.ID)
				assert.Equal(t, 0, rt.called)
				return
			}

		})
	}
}
