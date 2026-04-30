// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spacescale/core/internal/scalecp/service/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCreateAppAfterDispatch(t *testing.T) {
	t.Run("uses refreshed app when available", func(t *testing.T) {
		current := tenant.App{ID: "app-1", Status: "queued"}
		refreshed := tenant.App{ID: "app-1", Status: "failed"}

		resolved := resolveCreateAppAfterDispatch(current, errors.New("dispatch failed"), refreshed, nil)

		assert.Equal(t, refreshed, resolved)
	})

	t.Run("marks deploying when dispatch succeeded and refresh failed", func(t *testing.T) {
		current := tenant.App{ID: "app-1", Status: "queued"}

		resolved := resolveCreateAppAfterDispatch(current, nil, tenant.App{}, errors.New("refresh failed"))

		assert.Equal(t, "deploying", resolved.Status)
	})

	t.Run("keeps current status when dispatch and refresh both fail", func(t *testing.T) {
		current := tenant.App{ID: "app-1", Status: "queued"}

		resolved := resolveCreateAppAfterDispatch(current, errors.New("dispatch failed"), tenant.App{}, errors.New("refresh failed"))

		assert.Equal(t, current, resolved)
	})
}

func TestNewCreateAppDispatchContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := newCreateAppDispatchContext(parent)
	defer cancel()

	require.NoError(t, ctx.Err())

	select {
	case <-ctx.Done():
		require.Fail(t, "dispatch context should ignore parent cancellation")
	case <-time.After(10 * time.Millisecond):
	}
}
