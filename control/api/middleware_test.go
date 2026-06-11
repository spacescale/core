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

func TestMiddlewareEmitsAccessLog(t *testing.T) {
	buf, restore := withCapturedAccessLogger(t)
	defer restore()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(Middleware())
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
	require.Equal(t, float64(http.StatusCreated), entry["status_code"])
	require.Equal(t, "192.168.97.1", entry["client_ip"])
	require.Equal(t, "/v1/projects/{id}", entry["route"])
	require.NotEmpty(t, entry["request_id"])
}

func TestRecovererMiddlewareRecoversPanic(t *testing.T) {
	buf, restore := withCapturedAccessLogger(t)
	defer restore()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(Middleware())
	r.Use(Recoverer())
	r.Get("/v1/projects/{id}", func(_ http.ResponseWriter, _ *http.Request) {
		panic(strings.Repeat("a", 300))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/projects/123", nil)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.JSONEq(t, `{"error":"internal error"}`, rr.Body.String())

	entries := decodeJSONLogEntries(t, buf)
	panicEntry := findEntryByEvent(t, entries, "panic")
	require.Equal(t, "api", panicEntry["component"])
	require.Equal(t, "ERROR", panicEntry["level"])
	require.Equal(t, "panic recovered", panicEntry["msg"])
	require.Equal(t, "/v1/projects/{id}", panicEntry["route"])
	require.Equal(t, "string", panicEntry["panic_type"])
	require.Equal(t, strings.Repeat("a", maxPanicValueLogLen), panicEntry["panic_value"])
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

func findEntryByEvent(t *testing.T, entries []map[string]any, event string) map[string]any {
	t.Helper()
	for _, entry := range entries {
		if got, ok := entry["event"].(string); ok && got == event {
			return entry
		}
		if got, ok := entry["msg"].(string); ok && got == event {
			return entry
		}
	}
	require.Failf(t, "missing event log", "event %q not found in captured logs", event)
	return nil
}
