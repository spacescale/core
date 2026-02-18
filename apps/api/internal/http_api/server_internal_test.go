// This file adds focused unit tests for internal server wiring helpers.
// Coverage focus:
// - rate-limit key derivation fallback behavior when principal context is absent
// - trusted internal header middleware authorization checks and success path
// These tests are pure net/http helper tests and do not require database setup.

package http_api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKeyByGithubID verifies limiter key extraction from request context.
// It covers both the authenticated-user key path and fallback bucket behavior.
func TestKeyByGithubID(t *testing.T) {
	t.Run("returns fallback when principal missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v0/projects", nil)

		key, err := keyByGithubID(req)

		require.NoError(t, err)
		require.Equal(t, "github:unknown", key)
	})

	t.Run("returns fallback when github id empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v0/projects", nil)
		ctx := withPrincipal(req.Context(), AuthPrincipal{GithubID: ""})

		key, err := keyByGithubID(req.WithContext(ctx))

		require.NoError(t, err)
		require.Equal(t, "github:unknown", key)
	})

	t.Run("returns github key when principal present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v0/projects", nil)
		ctx := withPrincipal(req.Context(), AuthPrincipal{GithubID: "12345"})

		key, err := keyByGithubID(req.WithContext(ctx))

		require.NoError(t, err)
		require.Equal(t, "github:12345", key)
	})
}

// TestInternalAuthMiddleware verifies trusted header authorization behavior.
// Cases cover missing/incorrect secrets plus the success path when secrets match.
func TestInternalAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		expectedSecret string
		providedSecret string
		wantStatus     int
		wantNextCalled bool
	}{
		{
			name:           "rejects when expected secret is empty",
			expectedSecret: "   ",
			providedSecret: "test-secret",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "rejects when request header missing",
			expectedSecret: "test-secret",
			providedSecret: "",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "rejects when request header does not match",
			expectedSecret: "test-secret",
			providedSecret: "wrong-secret",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "allows when request header matches after trimming",
			expectedSecret: "  test-secret  ",
			providedSecret: " test-secret ",
			wantStatus:     http.StatusNoContent,
			wantNextCalled: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			})
			handler := internalAuthMiddleware(tc.expectedSecret)(next)

			req := httptest.NewRequest(http.MethodPost, "/v0/internal/auth-sync", nil)
			if tc.providedSecret != "" {
				req.Header.Set("X-Internal-Auth", tc.providedSecret)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req.WithContext(context.Background()))

			require.Equal(t, tc.wantStatus, rr.Code)
			require.Equal(t, tc.wantNextCalled, nextCalled)

			if tc.wantStatus == http.StatusUnauthorized {
				var out errResp
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
				require.Equal(t, "unauthorized", out.Error)
			}
		})
	}
}
