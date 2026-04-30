// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package dispatch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReturnLaunchError(t *testing.T) {
	t.Run("returns launch error when failure mark succeeds", func(t *testing.T) {
		launchErr := errors.New("launch failed")

		err := returnLaunchError(launchErr, nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, launchErr)
	})

	t.Run("joins launch and failure mark errors", func(t *testing.T) {
		launchErr := errors.New("launch failed")
		markErr := errors.New("mark failed")

		err := returnLaunchError(launchErr, markErr)

		require.Error(t, err)
		assert.ErrorIs(t, err, launchErr)
		assert.ErrorIs(t, err, markErr)
	})
}
