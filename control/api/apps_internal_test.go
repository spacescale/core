package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spacescale/core/control/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCreateWorkloadAfterDispatch(t *testing.T) {
	t.Run("uses refreshed workload when available", func(t *testing.T) {
		current := tenant.Workload{ID: "workload-1", Status: "queued"}
		refreshed := tenant.Workload{ID: "workload-1", Status: "failed"}

		resolved := resolveCreateWorkloadAfterDispatch(current, errors.New("dispatch failed"), refreshed, nil)

		assert.Equal(t, refreshed, resolved)
	})

	t.Run("marks deploying when dispatch succeeded and refresh failed", func(t *testing.T) {
		current := tenant.Workload{ID: "workload-1", Status: "queued"}

		resolved := resolveCreateWorkloadAfterDispatch(current, nil, tenant.Workload{}, errors.New("refresh failed"))

		assert.Equal(t, "deploying", resolved.Status)
	})

	t.Run("keeps current status when dispatch and refresh both fail", func(t *testing.T) {
		current := tenant.Workload{ID: "workload-1", Status: "queued"}

		resolved := resolveCreateWorkloadAfterDispatch(current, errors.New("dispatch failed"), tenant.Workload{}, errors.New("refresh failed"))

		assert.Equal(t, current, resolved)
	})
}

func TestNewCreateWorkloadDispatchContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := newCreateWorkloadDispatchContext(parent)
	defer cancel()

	require.NoError(t, ctx.Err())

	select {
	case <-ctx.Done():
		require.Fail(t, "dispatch context should ignore parent cancellation")
	case <-time.After(10 * time.Millisecond):
	}
}
