// This file verifies end-to-end behavior of authenticated workspace endpoints.
// Tests assert externally visible contracts for CRUD actions and ownership rules.

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type workspaceAPIResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type listWorkspacesAPIResponse struct {
	Workspaces []workspaceAPIResponse `json:"workspaces"`
}

func createWorkspaceViaAPI(t *testing.T, ts *testServer, identityKey, name string) workspaceAPIResponse {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"name":"%s"}`, name))
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces", body, map[string]string{
		"Cookie":       authCookieForIdentityKey(t, identityKey),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out workspaceAPIResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.Equal(t, name, out.Name)

	return out
}

func TestCreateWorkspace(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceName := fmt.Sprintf("workspace-%d", time.Now().UnixNano())
	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces", []byte(fmt.Sprintf(`{"name":"%s"}`, workspaceName)), map[string]string{
		"Cookie":       authCookieForIdentityKey(t, identityKey),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out workspaceAPIResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.Equal(t, workspaceName, out.Name)
	require.Equal(t, "/v1/workspaces/"+out.ID, resp.Header.Get("Location"))

	_, err := time.Parse(time.RFC3339, out.CreatedAt)
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339, out.UpdatedAt)
	require.NoError(t, err)
}

func TestCreateWorkspaceConflict(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceName := fmt.Sprintf("workspace-%d", time.Now().UnixNano())
	_ = createWorkspaceViaAPI(t, ts, identityKey, workspaceName)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces", []byte(fmt.Sprintf(`{"name":"%s"}`, workspaceName)), map[string]string{
		"Cookie":       authCookieForIdentityKey(t, identityKey),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusConflict, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "conflict", out.Error)
}

func TestCreateWorkspaceAllowsSameNameAcrossUsers(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	userA := uniqueIdentityKey(t)
	userB := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, userA)
	syncAuthUserForTest(t, ts, userB)

	workspaceName := fmt.Sprintf("workspace-%d", time.Now().UnixNano())
	a := createWorkspaceViaAPI(t, ts, userA, workspaceName)
	b := createWorkspaceViaAPI(t, ts, userB, workspaceName)

	require.NotEqual(t, a.ID, b.ID)
}

func TestCreateWorkspaceInvalidInput(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces", []byte(`{"name":"   "}`), map[string]string{
		"Cookie":       authCookieForIdentityKey(t, identityKey),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestCreateWorkspaceMissingAuth(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces", []byte(`{"name":"workspace-01"}`), map[string]string{
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}

func TestListWorkspacesScopesToCaller(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	userA := uniqueIdentityKey(t)
	userB := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, userA)
	syncAuthUserForTest(t, ts, userB)

	a1 := fmt.Sprintf("workspace-a1-%d", time.Now().UnixNano())
	a2 := fmt.Sprintf("workspace-a2-%d", time.Now().UnixNano())
	b1 := fmt.Sprintf("workspace-b1-%d", time.Now().UnixNano())

	_ = createWorkspaceViaAPI(t, ts, userA, a1)
	_ = createWorkspaceViaAPI(t, ts, userA, a2)
	_ = createWorkspaceViaAPI(t, ts, userB, b1)

	resp, data := doRequest(t, ts, http.MethodGet, "/v1/workspaces", nil, map[string]string{
		"Cookie": authCookieForIdentityKey(t, userA),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out listWorkspacesAPIResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Len(t, out.Workspaces, 2)

	names := map[string]bool{}
	for _, ws := range out.Workspaces {
		names[ws.Name] = true
	}
	require.True(t, names[a1])
	require.True(t, names[a2])
	require.False(t, names[b1])
}

func TestGetWorkspaceOwnership(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	owner := uniqueIdentityKey(t)
	other := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, owner)
	syncAuthUserForTest(t, ts, other)

	workspace := createWorkspaceViaAPI(t, ts, owner, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	resp, data := doRequest(t, ts, http.MethodGet, "/v1/workspaces/"+workspace.ID, nil, map[string]string{
		"Cookie": authCookieForIdentityKey(t, owner),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	resp, data = doRequest(t, ts, http.MethodGet, "/v1/workspaces/"+workspace.ID, nil, map[string]string{
		"Cookie": authCookieForIdentityKey(t, other),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}

func TestUpdateWorkspace(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	owner := uniqueIdentityKey(t)
	other := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, owner)
	syncAuthUserForTest(t, ts, other)

	workspace := createWorkspaceViaAPI(t, ts, owner, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	newName := fmt.Sprintf("workspace-renamed-%d", time.Now().UnixNano())

	resp, data := doRequest(t, ts, http.MethodPatch, "/v1/workspaces/"+workspace.ID, []byte(fmt.Sprintf(`{"name":"%s"}`, newName)), map[string]string{
		"Cookie":       authCookieForIdentityKey(t, owner),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out workspaceAPIResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, workspace.ID, out.ID)
	require.Equal(t, newName, out.Name)

	resp, data = doRequest(t, ts, http.MethodPatch, "/v1/workspaces/"+workspace.ID, []byte(`{"name":"nope"}`), map[string]string{
		"Cookie":       authCookieForIdentityKey(t, other),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}

func TestUpdateWorkspaceConflict(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	w1 := createWorkspaceViaAPI(t, ts, identityKey, fmt.Sprintf("workspace-a-%d", time.Now().UnixNano()))
	w2 := createWorkspaceViaAPI(t, ts, identityKey, fmt.Sprintf("workspace-b-%d", time.Now().UnixNano()))

	resp, data := doRequest(t, ts, http.MethodPatch, "/v1/workspaces/"+w1.ID, []byte(fmt.Sprintf(`{"name":"%s"}`, w2.Name)), map[string]string{
		"Cookie":       authCookieForIdentityKey(t, identityKey),
		"Content-Type": "application/json",
	})
	require.Equal(t, http.StatusConflict, resp.StatusCode, string(data))
}

func TestDeleteWorkspace(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	owner := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, owner)

	workspace := createWorkspaceViaAPI(t, ts, owner, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	resp, data := doRequest(t, ts, http.MethodDelete, "/v1/workspaces/"+workspace.ID, nil, map[string]string{
		"Cookie": authCookieForIdentityKey(t, owner),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(data))

	resp, data = doRequest(t, ts, http.MethodGet, "/v1/workspaces/"+workspace.ID, nil, map[string]string{
		"Cookie": authCookieForIdentityKey(t, owner),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}

func TestDeleteWorkspaceOwnership(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	owner := uniqueIdentityKey(t)
	other := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, owner)
	syncAuthUserForTest(t, ts, other)

	workspace := createWorkspaceViaAPI(t, ts, owner, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	resp, data := doRequest(t, ts, http.MethodDelete, "/v1/workspaces/"+workspace.ID, nil, map[string]string{
		"Cookie": authCookieForIdentityKey(t, other),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "unauthorized", out.Error)
}
