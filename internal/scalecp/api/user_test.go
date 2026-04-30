// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// This file verifies auth-sync endpoint contract behavior.
// It focuses on auth-sync payload validation and idempotent persistence results.

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", []byte("{"), map[string]string{
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
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", body, map[string]string{
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
	identityKey := uniqueIdentityKey(t)

	body := []byte(fmt.Sprintf(`{"identityKey":"%s","email":"dev@example.com","name":"Dev","avatarUrl":"https://example.com/avatar.png"}`,
		identityKey,
	))
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", body, map[string]string{
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
	identityKey := uniqueIdentityKey(t)

	firstBody := []byte(fmt.Sprintf(`{"identityKey":"%s","email":"first@example.com","name":"First","avatarUrl":"https://example.com/first.png"}`,
		identityKey,
	))
	firstResp, firstData := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", firstBody, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})
	require.Equal(t, http.StatusOK, firstResp.StatusCode, string(firstData))

	var first syncAuthUserResponse
	require.NoError(t, json.Unmarshal(firstData, &first))
	require.NotEmpty(t, first.ID)

	secondBody := []byte(fmt.Sprintf(`{"identityKey":"%s","email":"second@example.com","name":"Second","avatarUrl":"https://example.com/second.png"}`,
		identityKey,
	))
	secondResp, secondData := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", secondBody, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})
	require.Equal(t, http.StatusOK, secondResp.StatusCode, string(secondData))

	var second syncAuthUserResponse
	require.NoError(t, json.Unmarshal(secondData, &second))
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.OnboardingCompleted, second.OnboardingCompleted)
}

func TestSyncAuthUserRejectsTooLongIdentity(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	body := []byte(fmt.Sprintf(`{"identityKey":"%s"}`, strings.Repeat("a", 513)))
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", body, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestSyncAuthUserSanitizesOptionalFieldsAndContinues(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)

	body := []byte(fmt.Sprintf(
		`{"identityKey":"%s","email":"not-an-email","name":"%s","avatarUrl":"javascript:alert(1)"}`,
		identityKey,
		strings.Repeat("n", 400),
	))
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", body, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out syncAuthUserResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
}

func TestSyncAuthUserRateLimitIsPerIdentity(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityA := uniqueIdentityKey(t)
	bodyA := []byte(fmt.Sprintf(`{"identityKey":"%s"}`, identityA))

	for i := 0; i < 60; i++ {
		resp, data := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", bodyA, map[string]string{
			"X-Internal-Auth": testInternalAuthSecret,
			"Content-Type":    "application/json",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	}

	secondResp, secondData := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", bodyA, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})
	require.Equal(t, http.StatusTooManyRequests, secondResp.StatusCode, string(secondData))

	var secondOut errorResponse
	require.NoError(t, json.Unmarshal(secondData, &secondOut))
	require.Equal(t, "rate limit exceeded", secondOut.Error)

	identityB := uniqueIdentityKey(t)
	bodyB := []byte(fmt.Sprintf(`{"identityKey":"%s"}`, identityB))
	thirdResp, thirdData := doRequest(t, ts, http.MethodPost, "/v1/internal/auth-sync", bodyB, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})
	require.Equal(t, http.StatusOK, thirdResp.StatusCode, string(thirdData))
}
