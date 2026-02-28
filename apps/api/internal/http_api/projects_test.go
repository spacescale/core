// This file verifies end-to-end behavior of project CRUD HTTP endpoints.
// Tests assert externally visible contract details such as status codes,
// response payload shapes, ownership checks, and malformed input handling.

// Package http_api_test exercises the public HTTP API.
package http_api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type projectResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Region      string `json:"region"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type listProjectsResponse struct {
	Projects []projectResponse `json:"projects"`
}

func createProjectViaAPI(t *testing.T, ts *testServer, identityKey, workspaceID, name, region string) projectResponse {
	t.Helper()

	body := []byte(`{}`)
	if name != "" || region != "" {
		if region == "" {
			body = []byte(fmt.Sprintf(`{"name":"%s"}`, name))
		} else {
			body = []byte(fmt.Sprintf(`{"name":"%s","region":"%s"}`, name, region))
		}
	}

	resp, data := doRequest(t, ts, http.MethodPost, fmt.Sprintf("/v1/workspaces/%s/projects", workspaceID), body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// TestCreateProjectDefaults verifies project creation with default values.
// It confirms generated naming, default region assignment, location headers,
// and RFC3339 timestamp formatting for minimal valid input.
func TestCreateProjectDefaults(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	resp, data := doRequest(t, ts, http.MethodPost, fmt.Sprintf("/v1/workspaces/%s/projects", workspaceID), []byte(`{}`), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.Equal(t, workspaceID, out.WorkspaceID)
	require.NotEmpty(t, out.Name)
	require.NotEmpty(t, out.Slug)
	require.Equal(t, "global", out.Region)
	require.Equal(t, out.Name, out.Slug)
	require.NotEmpty(t, resp.Header.Get("Location"))

	_, err := time.Parse(time.RFC3339, out.CreatedAt)
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339, out.UpdatedAt)
	require.NoError(t, err)
}

// TestCreateProjectOverrides verifies explicit request values are preserved.
// It confirms user-supplied name and region survive the full request path
// without fallback generation overriding those fields.
func TestCreateProjectOverrides(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	projectName := fmt.Sprintf("misty-harbor-%d", time.Now().UnixNano())
	body := []byte(fmt.Sprintf(`{"name":"%s","region":"global"}`, projectName))
	resp, data := doRequest(t, ts, http.MethodPost, fmt.Sprintf("/v1/workspaces/%s/projects", workspaceID), body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, workspaceID, out.WorkspaceID)
	require.Equal(t, projectName, out.Name)
	require.Equal(t, projectName, out.Slug)
	require.Equal(t, "global", out.Region)
}

// TestCreateProjectMissingHeader verifies authentication header enforcement.
// It expects an unauthorized response when the request omits caller identity,
// which keeps project creation tied to an authenticated user context.
func TestCreateProjectMissingHeader(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces/00000000-0000-0000-0000-000000000000/projects", []byte(`{}`), map[string]string{
		"Content-Type": "application/json",
	})

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.Error)
}

// TestCreateProjectRequiresSyncedUser verifies project creation requires a
// user that has already been synced through the internal auth-sync endpoint.
func TestCreateProjectRequiresSyncedUser(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	unsyncedIdentityKey := uniqueIdentityKey(t)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces/00000000-0000-0000-0000-000000000000/projects", []byte(`{}`), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, unsyncedIdentityKey),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "unauthorized", out.Error)
}

// TestCreateProjectInvalidJSON verifies malformed JSON handling.
// It expects a bad request response when decoding fails so handlers reject
// malformed payloads before any service logic executes.
func TestCreateProjectInvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces/00000000-0000-0000-0000-000000000000/projects", []byte("{"), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.Error)
}

func TestCreateProjectInvalidRegion(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	body := []byte(`{"name":"misty-harbor","region":"global!"}`)
	resp, data := doRequest(t, ts, http.MethodPost, fmt.Sprintf("/v1/workspaces/%s/projects", workspaceID), body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestCreateProjectNameTooLong(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	longName := strings.Repeat("a", 121)
	body := []byte(fmt.Sprintf(`{"name":"%s","region":"global"}`, longName))
	resp, data := doRequest(t, ts, http.MethodPost, fmt.Sprintf("/v1/workspaces/%s/projects", workspaceID), body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestListProjectsByWorkspace(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceA := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-a-%d", time.Now().UnixNano()))
	workspaceB := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-b-%d", time.Now().UnixNano()))

	projectA := createProjectViaAPI(t, ts, identityKey, workspaceA, fmt.Sprintf("alpine-%d", time.Now().UnixNano()), "global")
	_ = createProjectViaAPI(t, ts, identityKey, workspaceB, fmt.Sprintf("delta-%d", time.Now().UnixNano()), "global")

	resp, data := doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects", workspaceA), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out listProjectsResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Len(t, out.Projects, 1)
	require.Equal(t, projectA.ID, out.Projects[0].ID)
	require.Equal(t, workspaceA, out.Projects[0].WorkspaceID)
}

func TestListProjectsWorkspaceOwnership(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	ownerIdentityKey := uniqueIdentityKey(t)
	otherIdentityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, ownerIdentityKey)
	syncAuthUserForTest(t, ts, otherIdentityKey)

	ownerWorkspaceID := createWorkspaceForIdentity(t, ts, ownerIdentityKey, fmt.Sprintf("workspace-owner-%d", time.Now().UnixNano()))

	resp, data := doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects", ownerWorkspaceID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, otherIdentityKey),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "unauthorized", out.Error)
}

func TestGetProject(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	created := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("echo-%d", time.Now().UnixNano()), "global")

	resp, data := doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects/%s", workspaceID, created.ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, created.ID, out.ID)
	require.Equal(t, workspaceID, out.WorkspaceID)
}

func TestGetProjectWorkspaceMismatch(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceA := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-a-%d", time.Now().UnixNano()))
	workspaceB := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-b-%d", time.Now().UnixNano()))

	created := createProjectViaAPI(t, ts, identityKey, workspaceA, fmt.Sprintf("foxtrot-%d", time.Now().UnixNano()), "global")

	resp, data := doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects/%s", workspaceB, created.ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}

func TestUpdateProject(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	created := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("golf-%d", time.Now().UnixNano()), "global")
	newName := fmt.Sprintf("harbor-%d", time.Now().UnixNano())

	body := []byte(fmt.Sprintf(`{"name":"%s","region":"global"}`, newName))
	resp, data := doRequest(t, ts, http.MethodPatch, fmt.Sprintf("/v1/workspaces/%s/projects/%s", workspaceID, created.ID), body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, created.ID, out.ID)
	require.Equal(t, newName, out.Name)
	require.Equal(t, created.Slug, out.Slug)
}

func TestUpdateProjectInvalidInput(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	created := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("india-%d", time.Now().UnixNano()), "global")

	resp, data := doRequest(t, ts, http.MethodPatch, fmt.Sprintf("/v1/workspaces/%s/projects/%s", workspaceID, created.ID), []byte(`{}`), map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestDeleteProject(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)
	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))

	created := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("juliet-%d", time.Now().UnixNano()), "global")

	resp, data := doRequest(t, ts, http.MethodDelete, fmt.Sprintf("/v1/workspaces/%s/projects/%s", workspaceID, created.ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(data))

	resp, data = doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects/%s", workspaceID, created.ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, identityKey),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}

func TestProjectCrudOwnership(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	ownerIdentityKey := uniqueIdentityKey(t)
	otherIdentityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, ownerIdentityKey)
	syncAuthUserForTest(t, ts, otherIdentityKey)

	ownerWorkspaceID := createWorkspaceForIdentity(t, ts, ownerIdentityKey, fmt.Sprintf("workspace-owner-%d", time.Now().UnixNano()))
	created := createProjectViaAPI(t, ts, ownerIdentityKey, ownerWorkspaceID, fmt.Sprintf("kilo-%d", time.Now().UnixNano()), "global")

	resp, data := doRequest(t, ts, http.MethodGet, fmt.Sprintf("/v1/workspaces/%s/projects/%s", ownerWorkspaceID, created.ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, otherIdentityKey),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	body := []byte(fmt.Sprintf(`{"name":"%s"}`, fmt.Sprintf("lima-%d", time.Now().UnixNano())))
	resp, data = doRequest(t, ts, http.MethodPatch, fmt.Sprintf("/v1/workspaces/%s/projects/%s", ownerWorkspaceID, created.ID), body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, otherIdentityKey),
		"Content-Type":  "application/json",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	resp, data = doRequest(t, ts, http.MethodDelete, fmt.Sprintf("/v1/workspaces/%s/projects/%s", ownerWorkspaceID, created.ID), nil, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, otherIdentityKey),
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
}
