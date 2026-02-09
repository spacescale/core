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
		"X-User-Github-ID": "12345",
		"Content-Type":     "application/json",
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, string(data))
	}

	var out projectResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.ID == "" || out.Name == "" || out.Slug == "" {
		t.Fatalf("missing required fields: %+v", out)
	}
	if out.Region != "global" {
		t.Fatalf("expected region global, got %q", out.Region)
	}
	if out.Name != out.Slug {
		t.Fatalf("expected name and slug to match, got %q and %q", out.Name, out.Slug)
	}
	if resp.Header.Get("Location") == "" {
		t.Fatalf("expected Location header")
	}

	if _, err := time.Parse(time.RFC3339, out.CreatedAt); err != nil {
		t.Fatalf("invalid createdAt: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, out.UpdatedAt); err != nil {
		t.Fatalf("invalid updatedAt: %v", err)
	}
}

// TestCreateProjectOverrides verifies explicit request values are preserved.
// It confirms user-supplied name and region survive the full request path
// without fallback generation overriding those fields.
func TestCreateProjectOverrides(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	body := []byte(`{"name":"misty-harbor","region":"global"}`)
	resp, data := doRequest(t, ts, http.MethodPost, "/v0/projects", body, map[string]string{
		"X-User-Github-ID": "12345",
		"Content-Type":     "application/json",
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, string(data))
	}

	var out projectResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Name != "misty-harbor" || out.Slug != "misty-harbor" {
		t.Fatalf("unexpected name/slug: %+v", out)
	}
	if out.Region != "global" {
		t.Fatalf("expected region global, got %q", out.Region)
	}
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

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", resp.StatusCode, string(data))
	}

	var out errorResponse
	_ = json.Unmarshal(data, &out)
	if out.Error == "" {
		t.Fatalf("expected error response")
	}
}

// TestCreateProjectInvalidJSON verifies malformed JSON handling.
// It expects a bad request response when decoding fails so handlers reject
// malformed payloads before any service logic executes.
func TestCreateProjectInvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/projects", []byte("{"), map[string]string{
		"X-User-Github-ID": "12345",
		"Content-Type":     "application/json",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", resp.StatusCode, string(data))
	}

	var out errorResponse
	_ = json.Unmarshal(data, &out)
	if out.Error == "" {
		t.Fatalf("expected error response")
	}
}
