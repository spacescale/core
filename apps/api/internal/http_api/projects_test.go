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

// TestCreateProjectDefaults verifies auto-generated project creation.
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

// TestCreateProjectOverrides verifies custom name and region overrides.
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

// TestCreateProjectMissingHeader verifies missing auth header handling.
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

// TestCreateProjectInvalidJSON verifies JSON parsing errors.
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
