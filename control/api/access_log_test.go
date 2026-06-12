package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
)

func withCapturedAccessLogger(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	old := slog.Default()
	buf := &bytes.Buffer{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return buf, func() { slog.SetDefault(old) }
}

func TestAccessLogLevel(t *testing.T) {
	require.Equal(t, slog.LevelInfo, accessLogLevel(http.StatusCreated))
	require.Equal(t, slog.LevelWarn, accessLogLevel(http.StatusBadRequest))
	require.Equal(t, slog.LevelError, accessLogLevel(http.StatusInternalServerError))
}

func TestAccessLoggerEmitsAccessLog(t *testing.T) {
	buf, restore := withCapturedAccessLogger(t)
	defer restore()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(AccessLogger())
	r.Get("/v1/projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/projects/123", nil)
	req.RemoteAddr = "192.168.97.1:54321"
	r.ServeHTTP(rr, req)

	entries := decodeJSONLogEntries(t, buf)
	entry := entries[len(entries)-1]
	require.Equal(t, "api", entry["component"])
	require.Equal(t, "http_access", entry["msg"])
	require.Equal(t, "INFO", entry["level"])
	require.Equal(t, "GET", entry["method"])
	require.InDelta(t, float64(http.StatusCreated), entry["status_code"], 0)
	require.Equal(t, "192.168.97.1", entry["client_ip"])
	require.Equal(t, "/v1/projects/{id}", entry["route"])
	require.NotEmpty(t, entry["request_id"])
}

func TestClientIP(t *testing.T) {
	require.Equal(t, "192.168.97.1", clientIP("192.168.97.1:54321"))
	require.Equal(t, "bad-addr", clientIP("bad-addr"))
}

func decodeJSONLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}
	require.NotEmpty(t, entries)
	return entries
}
