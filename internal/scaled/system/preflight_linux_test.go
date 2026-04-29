package system

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreflight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	originalEnsureKVM := preflightEnsureKVM
	originalEnsureFirecrackerJailer := preflightEnsureFirecrackerJailer
	originalDisableSwap := preflightDisableSwap
	originalDisableKSM := preflightDisableKSM
	originalDisableSMT := preflightDisableSMT
	defer func() {
		preflightEnsureKVM = originalEnsureKVM
		preflightEnsureFirecrackerJailer = originalEnsureFirecrackerJailer
		preflightDisableSwap = originalDisableSwap
		preflightDisableKSM = originalDisableKSM
		preflightDisableSMT = originalDisableSMT
	}()

	t.Run("runs checks in order", func(t *testing.T) {
		calls := make([]string, 0, 5)

		preflightEnsureKVM = func() error {
			calls = append(calls, "kvm")
			return nil
		}
		preflightEnsureFirecrackerJailer = func() (FirecrackerJailerIdentity, error) {
			calls = append(calls, "jailer")
			return FirecrackerJailerIdentity{UID: 100, GID: 100}, nil
		}
		preflightDisableSwap = func() error {
			calls = append(calls, "swap")
			return nil
		}
		preflightDisableKSM = func() error {
			calls = append(calls, "ksm")
			return nil
		}
		preflightDisableSMT = func() error {
			calls = append(calls, "smt")
			return nil
		}

		err := Preflight(logger)

		require.NoError(t, err)
		assert.Equal(t, []string{"kvm", "jailer", "swap", "ksm", "smt"}, calls)
	})

	t.Run("stops on first failure", func(t *testing.T) {
		calls := make([]string, 0, 5)
		wantErr := errors.New("swap failed")

		preflightEnsureKVM = func() error {
			calls = append(calls, "kvm")
			return nil
		}
		preflightEnsureFirecrackerJailer = func() (FirecrackerJailerIdentity, error) {
			calls = append(calls, "jailer")
			return FirecrackerJailerIdentity{UID: 100, GID: 100}, nil
		}
		preflightDisableSwap = func() error {
			calls = append(calls, "swap")
			return wantErr
		}
		preflightDisableKSM = func() error {
			calls = append(calls, "ksm")
			return nil
		}
		preflightDisableSMT = func() error {
			calls = append(calls, "smt")
			return nil
		}

		err := Preflight(logger)

		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
		assert.Equal(t, []string{"kvm", "jailer", "swap"}, calls)
	})
}
