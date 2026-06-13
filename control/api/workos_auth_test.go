package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/spacescale/core/control/db/sqlc"
	"github.com/spacescale/core/shared/config"
	"github.com/stretchr/testify/require"
	workos "github.com/workos/workos-go/v9"
)

func TestWorkOSLoginRedirectsAndSetsStateCookie(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, _ := doRequestNoRedirect(t, ts, "/auth/login", nil)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Location"), "/user_management/authorize")
	require.Contains(t, strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "spacescale_workos_state=")
}

func TestWorkOSCallbackRejectsMissingCode(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequestNoRedirect(t, ts, "/auth/callback?state=abc", map[string]string{
		"Cookie": "spacescale_workos_state=abc",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.JSONEq(t, `{"error":"missing code"}`, string(data))
}

func TestWorkOSCallbackRejectsMismatchedState(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequestNoRedirect(t, ts, "/auth/callback?code=valid-code&state=wrong", map[string]string{
		"Cookie": "spacescale_workos_state=right",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.JSONEq(t, `{"error":"unauthorized"}`, string(data))
}

func TestWorkOSCallbackSetsSessionCookieAndSyncsUser(t *testing.T) {
	workosServer := newFakeWorkOSServer(t)
	defer workosServer.Close()

	client := workos.NewClient("sk_test", workos.WithClientID(testWorkOSClientID), workos.WithBaseURL(workosServer.URL))
	ts := newTestServerWithWorkOSClient(t, client)
	defer ts.close()

	resp, _ := doRequestNoRedirect(t, ts, "/auth/callback?code=valid-code&state=state-123", map[string]string{
		"Cookie": "spacescale_workos_state=state-123",
	})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "http://localhost:8080/healthz", resp.Header.Get("Location"))

	cookies := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	require.Contains(t, cookies, "spacescale_session=")
	require.Contains(t, cookies, "spacescale_workos_state=;")

	queries := sqlc.New(ts.pool)
	user, err := queries.GetUserByIdentityKey(context.Background(), workOSIdentityKey("user_123"))
	require.NoError(t, err)
	require.Equal(t, "dev@example.com", derefString(user.Email))
	require.Equal(t, "Dev User", derefString(user.Name))
	require.Equal(t, "https://example.com/avatar.png", derefString(user.AvatarUrl))
}

func TestWorkOSSessionRefreshResealsCookie(t *testing.T) {
	workosServer := newFakeWorkOSServer(t)
	defer workosServer.Close()

	client := workos.NewClient("sk_test", workos.WithClientID(testWorkOSClientID), workos.WithBaseURL(workosServer.URL))
	ts := newTestServerWithWorkOSClient(t, client)
	defer ts.close()

	identityKey := "user_refresh"
	syncAuthUserForTest(t, ts, identityKey)

	resp, data := doRequestNoRedirect(t, ts, "/v1/workspaces", map[string]string{
		"Cookie": authCookieForIdentityKeyWithExpiry(t, identityKey, time.Now().Add(-time.Minute)),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	require.JSONEq(t, `{"workspaces":[]}`, string(data))
	require.Contains(t, strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "spacescale_session=")
}

func TestInvalidSessionCookieReturnsUnauthorized(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequestNoRedirect(t, ts, "/v1/workspaces", map[string]string{
		"Cookie": "spacescale_session=not-a-valid-cookie",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.JSONEq(t, `{"error":"unauthorized"}`, string(data))
	require.Contains(t, strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "spacescale_session=;")
}

func TestWorkOSLogoutClearsCookieAndRedirectsToWorkOS(t *testing.T) {
	workosServer := newFakeWorkOSServer(t)
	defer workosServer.Close()

	client := workos.NewClient("sk_test", workos.WithClientID(testWorkOSClientID), workos.WithBaseURL(workosServer.URL))
	ts := newTestServerWithWorkOSClient(t, client)
	defer ts.close()

	resp, _ := doRequestNoRedirect(t, ts, "/auth/logout", map[string]string{
		"Cookie": authCookieForSessionID(t, "user_logout", "sess_logout", time.Now().Add(time.Hour)),
	})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	location := resp.Header.Get("Location")
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	require.Equal(t, "/user_management/sessions/logout", parsed.Path)
	require.Equal(t, "sess_logout", parsed.Query().Get("session_id"))
	require.Equal(t, "http://localhost:8080/healthz", parsed.Query().Get("return_to"))
	require.Contains(t, strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "spacescale_session=;")
}

func TestWorkOSAuthUnavailablePaths(t *testing.T) {
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

func authCookieForIdentityKeyWithExpiry(t *testing.T, identityKey string, expiresAt time.Time) string {
	t.Helper()
	return authCookieForSessionID(t, identityKey, "sess_"+identityKey, expiresAt)
}

func authCookieForSessionID(t *testing.T, identityKey, sessionID string, expiresAt time.Time) string {
	t.Helper()
	sealedSession, err := workos.SealSession(&workos.SessionData{
		AccessToken:  fakeJWT(sessionID, expiresAt),
		RefreshToken: "refresh-token",
		User: &workos.User{
			ID:    identityKey,
			Email: "dev@example.com",
		},
	}, testWorkOSCookieSecret)
	require.NoError(t, err)
	return testWorkOSCookieName + "=" + sealedSession
}

func newFakeWorkOSServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/user_management/authenticate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}

		grantType, _ := req["grant_type"].(string)
		var sessionID string
		var userID string
		switch grantType {
		case "authorization_code":
			if req["code"] != "valid-code" {
				t.Fatalf("unexpected code: %#v", req["code"])
			}
			sessionID = "sess_callback"
			userID = "user_123"
		case "refresh_token":
			if req["refresh_token"] != "refresh-token" {
				t.Fatalf("unexpected refresh token: %#v", req["refresh_token"])
			}
			sessionID = "sess_refresh"
			userID = "user_refresh"
		default:
			t.Fatalf("unexpected grant_type %q", grantType)
		}

		firstName := "Dev"
		lastName := "User"
		avatarURL := "https://example.com/avatar.png"
		accessToken := fakeJWT(sessionID, time.Now().Add(time.Hour))
		response := map[string]any{
			"access_token":  accessToken,
			"refresh_token": "refresh-token",
			"user": map[string]any{
				"object":              "user",
				"id":                  userID,
				"first_name":          firstName,
				"last_name":           lastName,
				"profile_picture_url": avatarURL,
				"email":               "dev@example.com",
				"email_verified":      true,
				"created_at":          "2026-01-01T00:00:00.000Z",
				"updated_at":          "2026-01-01T00:00:00.000Z",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
