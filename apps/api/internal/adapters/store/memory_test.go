// Tests for in memory store behavior
// Tests include create list and fetch for apps
// Tests cover deployment queue and update flows
// Tests verify not found and conflict errors
// These tests ensure store data integrity

package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t0gun/spacescale/internal/adapters/store"
	"github.com/t0gun/spacescale/internal/contracts"
	"github.com/t0gun/spacescale/internal/domain"
)

// TestMemoryStore_CreateApp_OK verifies app creation succeeds.
func TestMemoryStore_CreateApp_OK(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
	assert.NoError(t, err)

	err = st.CreateApp(ctx, app)
	assert.NoError(t, err)
}

// TestMemoryStore_CreateApp_DuplicateName_Conflict verifies duplicate names return conflict.
func TestMemoryStore_CreateApp_DuplicateName_Conflict(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app1, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
	assert.NoError(t, err)
	app2, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
	assert.NoError(t, err)

	assert.NoError(t, st.CreateApp(ctx, app1))

	err = st.CreateApp(ctx, app2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrConflict)
}

// TestMemoryStore_GetAppByID_NotFound verifies missing ids return not found.
func TestMemoryStore_GetAppByID_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	_, err := st.GetAppByID(ctx, "missing")
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

// TestMemoryStore_GetAppByName_NotFound verifies missing names return not found.
func TestMemoryStore_GetAppByName_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	_, err := st.GetAppByName(ctx, "missing")
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

// TestMemoryStore_ListApps_Count verifies list count.
func TestMemoryStore_ListApps_Count(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	a, err := domain.NewApp(domain.NewAppParams{Name: "a", Image: "nginx:latest", Port: ptrInt(8080)})
	assert.NoError(t, err)
	b, err := domain.NewApp(domain.NewAppParams{Name: "b", Image: "nginx:latest", Port: ptrInt(8080)})
	assert.NoError(t, err)

	assert.NoError(t, st.CreateApp(ctx, a))
	assert.NoError(t, st.CreateApp(ctx, b))

	apps, err := st.ListApps(ctx)
	assert.NoError(t, err)
	assert.Len(t, apps, 2)
}

// TestMemoryStore_CreateDeployment_AppMissing_NotFound verifies missing apps return not found.
func TestMemoryStore_CreateDeployment_AppMissing_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	dep := domain.NewDeployment("missing-app")

	err := st.CreateDeployment(ctx, dep)
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

// TestMemoryStore_CreateDeployment_And_GetByID_OK verifies create and fetch by id.
func TestMemoryStore_CreateDeployment_And_GetByID_OK(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
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

// TestMemoryStore_ListDeploymentsByAppID_OrderAndReflectsUpdates verifies ordering and updates.
func TestMemoryStore_ListDeploymentsByAppID_OrderAndReflectsUpdates(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
	require.NoError(t, err)
	require.NoError(t, st.CreateApp(ctx, app))

	d1 := domain.NewDeployment(app.ID)
	d2 := domain.NewDeployment(app.ID)

	require.NoError(t, st.CreateDeployment(ctx, d1))
	require.NoError(t, st.CreateDeployment(ctx, d2))

	updatedURL := "https://example.com"
	d1.Status = domain.DeploymentStatusRunning
	d1.URL = &updatedURL
	require.NoError(t, st.UpdateDeployment(ctx, d1))

	deps, err := st.ListDeploymentsByAppID(ctx, app.ID)
	require.NoError(t, err)
	require.Len(t, deps, 2)
	assert.Equal(t, d1.ID, deps[0].ID)
	assert.Equal(t, domain.DeploymentStatusRunning, deps[0].Status)
	require.NotNil(t, deps[0].URL)
	assert.Equal(t, updatedURL, *deps[0].URL)
	assert.Equal(t, d2.ID, deps[1].ID)
}

// TestMemoryStore_ListDeploymentsByAppID_EmptySlice verifies empty results.
func TestMemoryStore_ListDeploymentsByAppID_EmptySlice(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	deps, err := st.ListDeploymentsByAppID(ctx, "missing-app")
	assert.NoError(t, err)
	assert.Empty(t, deps)
}

// TestMemoryStore_TakeNextQueuedDeployment_FIFO verifies FIFO queue behavior.
func TestMemoryStore_TakeNextQueuedDeployment_FIFO(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
	require.NoError(t, err)
	require.NoError(t, st.CreateApp(ctx, app))

	first := domain.NewDeployment(app.ID)
	second := domain.NewDeployment(app.ID)
	require.NoError(t, st.CreateDeployment(ctx, first))
	require.NoError(t, st.CreateDeployment(ctx, second))

	dep, err := st.TakeNextQueuedDeployment(ctx)
	require.NoError(t, err)
	assert.Equal(t, first.ID, dep.ID)

	dep, err = st.TakeNextQueuedDeployment(ctx)
	require.NoError(t, err)
	assert.Equal(t, second.ID, dep.ID)

	_, err = st.TakeNextQueuedDeployment(ctx)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

// TestMemoryStore_TakeNextQueuedDeployment_SkipsNonQueued skips non-queued deployments.
func TestMemoryStore_TakeNextQueuedDeployment_SkipsNonQueued(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	app, err := domain.NewApp(domain.NewAppParams{Name: "hello", Image: "nginx:latest", Port: ptrInt(8080)})
	require.NoError(t, err)
	require.NoError(t, st.CreateApp(ctx, app))

	skip := domain.NewDeployment(app.ID)
	next := domain.NewDeployment(app.ID)
	require.NoError(t, st.CreateDeployment(ctx, skip))
	require.NoError(t, st.CreateDeployment(ctx, next))

	skip.Status = domain.DeploymentStatusRunning
	require.NoError(t, st.UpdateDeployment(ctx, skip))

	dep, err := st.TakeNextQueuedDeployment(ctx)
	require.NoError(t, err)
	assert.Equal(t, next.ID, dep.ID)

	_, err = st.TakeNextQueuedDeployment(ctx)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

// TestMemoryStore_UpdateDeployment_NotFound verifies missing updates fail.
func TestMemoryStore_UpdateDeployment_NotFound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	dep := domain.NewDeployment("missing-app")

	err := st.UpdateDeployment(ctx, dep)
	assert.Error(t, err)
	assert.ErrorIs(t, err, contracts.ErrNotFound)
}

// ptrInt returns a pointer to v.
func ptrInt(v int) *int {
	return &v
}
