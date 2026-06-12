package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spacescale/core/shared/config"
	"github.com/stretchr/testify/require"
)

func TestWorkOSAuthAuthUnavailablePaths(t *testing.T) {
	a := &workOSAuth{config: config.Control{WorkOS: config.WorkOSConfig{CookieName: "spacescale_session"}}}

	tests := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{name: "login", fn: a.handleLogin},
		{name: "callback", fn: a.handleCallback},
		{name: "logout", fn: a.handleLogout},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/"+tc.name, nil)

			tc.fn(rec, req)

			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.JSONEq(t, `{"error":"auth unavailable"}`, rec.Body.String())
		})
	}
}

func TestSessionMiddlewareWithoutClientRejectsSession(t *testing.T) {
	a := &workOSAuth{config: config.Control{WorkOS: config.WorkOSConfig{CookieName: "spacescale_session"}}}
	called := false
	handler := a.sessionMiddleware()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/workspaces", nil)
	handler.ServeHTTP(rec, req)

	require.False(t, called)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"unauthorized"}`, rec.Body.String())
	require.Contains(t, strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "spacescale_session=;")
}

func TestNonEmptyStringPtrTrimsAndOmitsBlank(t *testing.T) {
	require.Nil(t, nonEmptyStringPtr("   "))
	require.Equal(t, "hello", *nonEmptyStringPtr(" hello "))
}
