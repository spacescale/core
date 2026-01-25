// Tests for deployment service behaviors
// Tests include queueing and runtime processing
// Tests verify error handling and status updates
// Tests cover no work and missing app cases
// Tests verify url behavior when expose is false

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/spacescale/internal/adapters/store"
	"github.com/t0gun/spacescale/internal/contracts"
	"github.com/t0gun/spacescale/internal/domain"
	"github.com/t0gun/spacescale/internal/service"
)

// TestDeployApp verifies deployment creation behavior.
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
		{label: "ok: queues deployment", appExists: true, appID: "", ok: true}, // we will create an app use its ID
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
					Port:  ptrInt(8080),
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
	listDepsErr  error
}

// GetAppByID returns an app or a configured error.
func (s storeWithHooks) GetAppByID(ctx context.Context, id string) (domain.App, error) {
	if s.getAppErr != nil {
		return domain.App{}, s.getAppErr
	}
	return s.Store.GetAppByID(ctx, id)
}

// CreateDeployment creates a deployment or returns a configured error.
func (s storeWithHooks) CreateDeployment(ctx context.Context, dep domain.Deployment) error {
	if s.createDepErr != nil {
		return s.createDepErr
	}
	return s.Store.CreateDeployment(ctx, dep)
}

// ListDeploymentsByAppID returns deployments or a configured error.
func (s storeWithHooks) ListDeploymentsByAppID(ctx context.Context, appID string) ([]domain.Deployment, error) {
	if s.listDepsErr != nil {
		return nil, s.listDepsErr
	}
	return s.Store.ListDeploymentsByAppID(ctx, appID)
}

// TestDeployApp_StoreErrors verifies store error handling.
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
			app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
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
	url    *string
	err    error
	called int
}

// Deploy tracks calls and returns configured results.
func (f *fakeRuntime) Deploy(ctx context.Context, app domain.App) (*string, error) {
	f.called++
	if f.err != nil {
		return nil, f.err
	}
	if !app.Expose {
		return nil, nil
	}
	return f.url, nil
}

var _ contracts.Runtime = (*fakeRuntime)(nil)

// TestProcessNextDeployment verifies runtime processing behavior.
func TestProcessNextDeployment(t *testing.T) {
	t.Run("no queued deployments", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		rt := &fakeRuntime{url: ptrString("https://hello.yourdomain.come")}
		svc := service.NewAppServiceWithRuntime(st, rt)

		dep, err := svc.ProcessNextDeployment(ctx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrNoWork)
		assert.Empty(t, dep.ID)
		assert.Equal(t, 0, rt.called)
	})

	t.Run("runtime fails", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		rt := &fakeRuntime{url: ptrString("https://hello.yourdomain.come"), err: errors.New("boom")}
		svc := service.NewAppServiceWithRuntime(st, rt)

		app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
		assert.NoError(t, err)
		assert.NoError(t, st.CreateApp(ctx, app))
		dep := domain.NewDeployment(app.ID)
		assert.NoError(t, st.CreateDeployment(ctx, dep))

		_, err = svc.ProcessNextDeployment(ctx)
		assert.Error(t, err)
		assert.Equal(t, 1, rt.called)
	})

	t.Run("runtime ok", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		rt := &fakeRuntime{url: ptrString("https://hello.yourdomain.come")}
		svc := service.NewAppServiceWithRuntime(st, rt)

		app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
		assert.NoError(t, err)
		assert.NoError(t, st.CreateApp(ctx, app))
		dep := domain.NewDeployment(app.ID)
		assert.NoError(t, st.CreateDeployment(ctx, dep))

		got, err := svc.ProcessNextDeployment(ctx)
		assert.NoError(t, err)
		assert.Equal(t, domain.DeploymentStatusRunning, got.Status)
		assert.NotEmpty(t, got.ID)
		assert.NotNil(t, got.URL)
		assert.Equal(t, *rt.url, *got.URL)
		assert.Nil(t, got.Error)
		assert.Equal(t, 1, rt.called)
		assert.WithinDuration(t, time.Now().UTC(), got.UpdatedAt, 2*time.Second)
	})

	t.Run("runtime ok no expose", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		rt := &fakeRuntime{url: ptrString("https://hello.yourdomain.come")}
		svc := service.NewAppServiceWithRuntime(st, rt)

		expose := false
		app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Expose: &expose})
		assert.NoError(t, err)
		assert.NoError(t, st.CreateApp(ctx, app))
		dep := domain.NewDeployment(app.ID)
		assert.NoError(t, st.CreateDeployment(ctx, dep))

		got, err := svc.ProcessNextDeployment(ctx)
		assert.NoError(t, err)
		assert.Equal(t, domain.DeploymentStatusRunning, got.Status)
		assert.Nil(t, got.URL)
		assert.Equal(t, 1, rt.called)
	})
}

// TestListDeployments verifies list deployments behavior.
func TestListDeployments(t *testing.T) {
	t.Run("invalid input: empty app id", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		svc := service.NewAppService(st)

		deps, err := svc.ListDeployments(ctx, service.ListDeploymentsParams{AppID: ""})
		assert.ErrorIs(t, err, service.ErrInvalidInput)
		assert.Nil(t, deps)
	})

	t.Run("not found: app missing", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		svc := service.NewAppService(st)

		deps, err := svc.ListDeployments(ctx, service.ListDeploymentsParams{AppID: "missing"})
		assert.ErrorIs(t, err, service.ErrNotFound)
		assert.Nil(t, deps)
	})

	t.Run("ok: returns deployments in create order", func(t *testing.T) {
		ctx := context.Background()
		st := store.NewMemoryStore()
		svc := service.NewAppService(st)

		app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
		assert.NoError(t, err)
		assert.NoError(t, st.CreateApp(ctx, app))

		dep1 := domain.NewDeployment(app.ID)
		dep2 := domain.NewDeployment(app.ID)
		assert.NoError(t, st.CreateDeployment(ctx, dep1))
		assert.NoError(t, st.CreateDeployment(ctx, dep2))

		deps, err := svc.ListDeployments(ctx, service.ListDeploymentsParams{AppID: app.ID})
		assert.NoError(t, err)
		assert.Len(t, deps, 2)
		assert.Equal(t, dep1.ID, deps[0].ID)
		assert.Equal(t, dep2.ID, deps[1].ID)
	})

	t.Run("store error bubbles up", func(t *testing.T) {
		ctx := context.Background()
		mem := store.NewMemoryStore()
		app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
		assert.NoError(t, err)
		assert.NoError(t, mem.CreateApp(ctx, app))

		st := storeWithHooks{
			Store:       mem,
			listDepsErr: errors.New("boom"),
		}
		svc := service.NewAppService(st)

		deps, err := svc.ListDeployments(ctx, service.ListDeploymentsParams{AppID: app.ID})
		assert.Error(t, err)
		assert.Nil(t, deps)
	})
}
