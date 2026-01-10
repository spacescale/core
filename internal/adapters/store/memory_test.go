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
