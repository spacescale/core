package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/paas/internal/adapters/store"
	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
)

func TestMemoryStore_CreateApp_OK(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)

	err = st.CreateApp(ctx, app)
	assert.NoError(t, err)
}

func TestMemoryStore_CreateApp_DuplicateName_Conflict(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app1, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)
	app2, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)

	assert.NoError(t, st.CreateApp(ctx, app1))

	err = st.CreateApp(ctx, app2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrConflict)
}

func TestMemoryStore_GetAppByID_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	_, err := st.GetAppByID(ctx, "missing")
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

func TestMemoryStore_GetAppByName_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	_, err := st.GetAppByName(ctx, "missing")
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

func TestMemoryStore_ListApps_Count(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	a, err := domain.NewApp(domain.NewAppParams{Name: "a", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)
	b, err := domain.NewApp(domain.NewAppParams{Name: "b", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)

	assert.NoError(t, st.CreateApp(ctx, a))
	assert.NoError(t, st.CreateApp(ctx, b))

	apps, err := st.ListApps(ctx)
	assert.NoError(t, err)
	assert.Len(t, apps, 2)
}

func TestMemoryStore_CreateDeployment_AppMissing_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	dep := domain.NewDeployment("missing-app")

	err := st.CreateDeployment(ctx, dep)
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

func TestMemoryStore_CreateDeployment_And_GetByID_OK(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)
	assert.NoError(t, st.CreateApp(ctx, app))

	dep := domain.NewDeployment(app.ID)
	assert.NoError(t, st.CreateDeployment(ctx, dep))

	got, err := st.GetDeploymentByID(ctx, dep.ID)
	assert.NoError(t, err)
	assert.Equal(t, dep.ID, got.ID)
	assert.Equal(t, app.ID, got.AppID)
	assert.Equal(t, domain.DeploymentStatusQueued, got.Status)
}

func TestMemoryStore_ListDeploymentsByAppID_Count(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: 8080})
	assert.NoError(t, err)
	assert.NoError(t, st.CreateApp(ctx, app))

	d1 := domain.NewDeployment(app.ID)
	d2 := domain.NewDeployment(app.ID)

	assert.NoError(t, st.CreateDeployment(ctx, d1))
	assert.NoError(t, st.CreateDeployment(ctx, d2))

	deps, err := st.ListDeploymentsByAppID(ctx, app.ID)
	assert.NoError(t, err)
	assert.Len(t, deps, 2)
}
