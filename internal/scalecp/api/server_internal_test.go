package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyByIdentityKey(t *testing.T) {
	t.Run("falls back when principal missing", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		key, err := keyByIdentityKey(req)

		require.NoError(t, err)
		assert.Equal(t, "identity:unknown", key)
	})

	t.Run("falls back when identity key empty", func(t *testing.T) {
		req, err := http.NewRequestWithContext(withPrincipal(context.Background(), AuthPrincipal{}), http.MethodGet, "/", nil)
		require.NoError(t, err)

		key, err := keyByIdentityKey(req)

		require.NoError(t, err)
		assert.Equal(t, "identity:unknown", key)
	})

	t.Run("uses principal identity key", func(t *testing.T) {
		req, err := http.NewRequestWithContext(withPrincipal(context.Background(), AuthPrincipal{IdentityKey: "user-123"}), http.MethodGet, "/", nil)
		require.NoError(t, err)

		key, err := keyByIdentityKey(req)

		require.NoError(t, err)
		assert.Equal(t, "identity:user-123", key)
	})
}
