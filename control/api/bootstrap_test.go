// This file verifies end-to-end behavior of the bootstrap defaults endpoint.
// Tests assert first-time creation, idempotent no-op behavior, auth rules, and
// malformed input handling for the authenticated bootstrap workflow.

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type bootstrapDefaultsAPIResponse struct {
	Created     bool   `json:"created"`
	WorkspaceID string `json:"workspaceId"`
	ProjectID   string `json:"projectId"`
}

type bootstrapWorkspaceListResponse struct {
	Workspaces []workspaceAPIResponse `json:"workspaces"`
}

type bootstrapProjectListResponse struct {
	Projects []projectResponse `json:"projects"`
}

type bootstrapErrorResponse struct {
	Error string `json:"error"`
}

func TestBootstrapDefaultsCreatesDefaults(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/bootstrap-defaults", []byte(`{}`), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out bootstrapDefaultsAPIResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.True(t, out.Created)
	require.NotEmpty(t, out.WorkspaceID)
	require.NotEmpty(t, out.ProjectID)

	resp, data = doRequest(t, ts, http.MethodGet, "/v1/workspaces", nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var wsOut bootstrapWorkspaceListResponse
	require.NoError(t, json.Unmarshal(data, &wsOut))
	require.Len(t, wsOut.Workspaces, 1)
	require.Equal(t, out.WorkspaceID, wsOut.Workspaces[0].ID)
	require.Equal(t, "workspace-01", wsOut.Workspaces[0].Name)

	resp, data = doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects", out.WorkspaceID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var prOut bootstrapProjectListResponse
	require.NoError(t, json.Unmarshal(data, &prOut))
	require.Len(t, prOut.Projects, 1)
	require.Equal(t, out.ProjectID, prOut.Projects[0].ID)
	require.Equal(t, out.WorkspaceID, prOut.Projects[0].WorkspaceID)
	require.NotEmpty(t, prOut.Projects[0].Slug)
}

func TestBootstrapDefaultsIsIdempotent(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/bootstrap-defaults", []byte(`{}`), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var first bootstrapDefaultsAPIResponse
	require.NoError(t, json.Unmarshal(data, &first))
	require.True(t, first.Created)
	require.NotEmpty(t, first.WorkspaceID)
	require.NotEmpty(t, first.ProjectID)

	resp, data = doRequest(t, ts, http.MethodPost, "/v1/bootstrap-defaults", nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var second bootstrapDefaultsAPIResponse
	require.NoError(t, json.Unmarshal(data, &second))
	require.False(t, second.Created)
	require.Empty(t, second.WorkspaceID)
	require.Empty(t, second.ProjectID)

	resp, data = doRequest(t, ts, http.MethodGet, "/v1/workspaces", nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var wsOut bootstrapWorkspaceListResponse
	require.NoError(t, json.Unmarshal(data, &wsOut))
	require.Len(t, wsOut.Workspaces, 1)

	resp, data = doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects", wsOut.Workspaces[0].ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var prOut bootstrapProjectListResponse
	require.NoError(t, json.Unmarshal(data, &prOut))
	require.Len(t, prOut.Projects, 1)
}

func TestBootstrapDefaultsMissingAuth(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/bootstrap-defaults", []byte(`{}`), map[string]string{
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out bootstrapErrorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "unauthorized", out.Error)
}

func TestBootstrapDefaultsRequiresSyncedUser(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/bootstrap-defaults", []byte(`{}`), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out bootstrapErrorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "unauthorized", out.Error)
}

func TestBootstrapDefaultsInvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/bootstrap-defaults", []byte("{"), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out bootstrapErrorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid json", out.Error)
}
