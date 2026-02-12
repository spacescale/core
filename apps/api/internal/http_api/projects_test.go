// This file verifies end-to-end behavior of the project creation HTTP endpoint.
// Tests assert externally visible contract details such as status codes,
// response payload shape, defaulting behavior, and error handling paths.
// These cases are intentionally API-focused and treat the service as a backend
// dependency, which helps prevent regressions in client-facing behavior.
// Add new project endpoint behavior checks here to keep contract coverage local.

// Package http_api_test exercises the public HTTP API.
package http_api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type projectResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Region    string `json:"region"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// TestCreateProjectDefaults verifies project creation with default values.
// It confirms generated naming, default region assignment, location headers,
// and RFC3339 timestamp formatting for minimal valid input.
func TestCreateProjectDefaults(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/projects", []byte(`{}`), map[string]string{
		"Authorization": authHeaderForGithubID(t, "12345"),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
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

	body := []byte(`{"name":"misty-harbor","region":"global"}`)
	resp, data := doRequest(t, ts, http.MethodPost, "/v0/projects", body, map[string]string{
		"Authorization": authHeaderForGithubID(t, "12345"),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out projectResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "misty-harbor", out.Name)
	require.Equal(t, "misty-harbor", out.Slug)
	require.Equal(t, "global", out.Region)
}

// TestCreateProjectMissingHeader verifies authentication header enforcement.
// It expects an unauthorized response when the request omits GitHub identity,
// which keeps project creation tied to an authenticated user context.
func TestCreateProjectMissingHeader(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/projects", []byte(`{}`), map[string]string{
		"Content-Type": "application/json",
	})

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.Error)
}

// TestCreateProjectInvalidJSON verifies malformed JSON handling.
// It expects a bad request response when decoding fails so handlers reject
// malformed payloads before any service logic executes.
func TestCreateProjectInvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/projects", []byte("{"), map[string]string{
		"Authorization": authHeaderForGithubID(t, "12345"),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.Error)
}
