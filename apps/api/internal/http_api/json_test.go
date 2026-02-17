// This file provides white-box tests for shared JSON helper behavior.
// The helpers in json.go are reused by multiple HTTP handlers, so regressions
// here can break endpoint contracts even when route-level tests look fine.
//
// Coverage focus in this file:
// - readJSON strict decoding behavior (unknown fields, trailing values, etc.)
// - writeJSON response status/content-type/body serialization contract
// - writeErr canonical error payload envelope

package http_api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// readJSONPayload is a local decode target used by readJSON tests.
// Keeping this type in the test file makes expected field behavior explicit.
type readJSONPayload struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

// newJSONTestRequest builds a request with the provided JSON body string.
// Keeping this helper local avoids repeated endpoint/body setup in each subtest.
func newJSONTestRequest(body string) *http.Request {
	return httptest.NewRequest(
		http.MethodPost,
		"/v0/projects",
		strings.NewReader(body),
	)
}

// TestReadJSON verifies strict request-body decoding behavior.
// Each case documents one transport contract expectation for helper callers.
func TestReadJSON(t *testing.T) {
	t.Run("valid single object", func(t *testing.T) {
		req := newJSONTestRequest(`{"name":"misty-harbor","region":"global"}`)
		var dst readJSONPayload

		err := readJSON(req, &dst)
		require.NoError(t, err)
		require.Equal(t, "misty-harbor", dst.Name)
		require.Equal(t, "global", dst.Region)
	})

	t.Run("empty body", func(t *testing.T) {
		req := newJSONTestRequest("")
		var dst readJSONPayload

		err := readJSON(req, &dst)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("malformed json", func(t *testing.T) {
		req := newJSONTestRequest(`{"name":"misty-harbor"`)
		var dst readJSONPayload

		err := readJSON(req, &dst)
		require.Error(t, err)
	})

	t.Run("unknown field", func(t *testing.T) {
		req := newJSONTestRequest(
			`{"name":"misty-harbor","region":"global","extra":true}`,
		)
		var dst readJSONPayload

		err := readJSON(req, &dst)
		require.ErrorContains(t, err, `unknown field "extra"`)
	})

	t.Run("multiple json values", func(t *testing.T) {
		req := newJSONTestRequest(
			`{"name":"a","region":"global"}{"name":"b","region":"global"}`,
		)
		var dst readJSONPayload

		err := readJSON(req, &dst)
		require.EqualError(t, err, "multiple json values")
	})

	t.Run("body too large", func(t *testing.T) {
		req := newJSONTestRequest(`{"name":"` + strings.Repeat("a", 128) + `"}`)
		var dst readJSONPayload

		// MaxBytesReader mirrors behavior used by the top-level HTTP wrapper in
		// main, where oversized request bodies are rejected during decode reads.
		rr := httptest.NewRecorder()
		req.Body = http.MaxBytesReader(rr, req.Body, 32)

		err := readJSON(req, &dst)
		require.ErrorIs(t, err, errRequestBodyTooLarge)
	})
}

// TestWriteJSON verifies response metadata and JSON body serialization.
// This keeps helper-level write behavior stable for all handlers.
func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	payload := map[string]string{"name": "misty-harbor", "region": "global"}

	writeJSON(rr, http.StatusCreated, payload)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var out map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, payload, out)
}

// TestWriteErr verifies the canonical API error envelope shape.
// Handlers rely on this helper to keep client-facing errors consistent.
func TestWriteErr(t *testing.T) {
	rr := httptest.NewRecorder()

	writeErr(rr, http.StatusBadRequest, "invalid input")

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var out errResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, "invalid input", out.Error)
}
