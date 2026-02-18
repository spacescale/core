// This file verifies auth-sync endpoint contract behavior.
// It focuses on auth-sync payload validation and idempotent persistence results.

package http_api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type syncAuthUserResponse struct {
	ID                  string `json:"id"`
	OnboardingCompleted bool   `json:"onboardingCompleted"`
}

func TestSyncAuthUserInvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", []byte("{"), map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid json", out.Error)
}

func TestSyncAuthUserRejectsEmptyIdentity(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	body := []byte(`{"identityKey":"   "}`)
	resp, data := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", body, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestSyncAuthUserSuccess(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	body := []byte(`{"identityKey":"12345","email":"dev@example.com","name":"Dev","avatarUrl":"https://example.com/avatar.png"}`)
	resp, data := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", body, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out syncAuthUserResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.False(t, out.OnboardingCompleted)
}

func TestSyncAuthUserReturnsSameUserOnResync(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	firstBody := []byte(`{"identityKey":"12345","email":"first@example.com","name":"First","avatarUrl":"https://example.com/first.png"}`)
	firstResp, firstData := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", firstBody, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})
	require.Equal(t, http.StatusOK, firstResp.StatusCode, string(firstData))

	var first syncAuthUserResponse
	require.NoError(t, json.Unmarshal(firstData, &first))
	require.NotEmpty(t, first.ID)

	secondBody := []byte(`{"identityKey":"12345","email":"second@example.com","name":"Second","avatarUrl":"https://example.com/second.png"}`)
	secondResp, secondData := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", secondBody, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})
	require.Equal(t, http.StatusOK, secondResp.StatusCode, string(secondData))

	var second syncAuthUserResponse
	require.NoError(t, json.Unmarshal(secondData, &second))
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.OnboardingCompleted, second.OnboardingCompleted)
}
