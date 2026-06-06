package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type readJSONPayload struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

func newJSONTestRequest(body string) *http.Request {
	return httptest.NewRequest(
		http.MethodPost,
		"/v1/projects",
		strings.NewReader(body),
	)
}

func TestReadJSON(t *testing.T) {
	t.Run("valid single object", func(t *testing.T) {
		req := newJSONTestRequest(`{"name":"misty-harbor","region":"global"}`)
		var dst readJSONPayload

		err := ReadJSON(req, &dst)
		require.NoError(t, err)
		require.Equal(t, "misty-harbor", dst.Name)
		require.Equal(t, "global", dst.Region)
	})

	t.Run("empty body", func(t *testing.T) {
		req := newJSONTestRequest("")
		var dst readJSONPayload

		err := ReadJSON(req, &dst)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("malformed json", func(t *testing.T) {
		req := newJSONTestRequest(`{"name":"misty-harbor"`)
		var dst readJSONPayload

		err := ReadJSON(req, &dst)
		require.Error(t, err)
	})

	t.Run("unknown field", func(t *testing.T) {
		req := newJSONTestRequest(
			`{"name":"misty-harbor","region":"global","extra":true}`,
		)
		var dst readJSONPayload

		err := ReadJSON(req, &dst)
		require.ErrorContains(t, err, `unknown field "extra"`)
	})

	t.Run("multiple json values", func(t *testing.T) {
		req := newJSONTestRequest(
			`{"name":"a","region":"global"}{"name":"b","region":"global"}`,
		)
		var dst readJSONPayload

		err := ReadJSON(req, &dst)
		require.EqualError(t, err, "multiple json values")
	})

	t.Run("body too large", func(t *testing.T) {
		req := newJSONTestRequest(`{"name":"` + strings.Repeat("a", 128) + `"}`)
		var dst readJSONPayload

		rr := httptest.NewRecorder()
		req.Body = http.MaxBytesReader(rr, req.Body, 32)

		err := ReadJSON(req, &dst)
		require.ErrorIs(t, err, ErrRequestBodyTooLarge)
	})
}

func TestJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	payload := map[string]string{"name": "misty-harbor", "region": "global"}

	JSON(rr, http.StatusCreated, payload)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var out map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, payload, out)
}

func TestError(t *testing.T) {
	rr := httptest.NewRecorder()

	Error(rr, http.StatusBadRequest, "invalid input")

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var out errorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, "invalid input", out.Error)
}
