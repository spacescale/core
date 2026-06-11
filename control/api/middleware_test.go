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
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "2xx maps to info", status: http.StatusCreated, want: "INFO"},
		{name: "3xx maps to info", status: http.StatusFound, want: "INFO"},
		{name: "4xx maps to warn", status: http.StatusUnauthorized, want: "WARN"},
		{name: "429 maps to warn", status: http.StatusTooManyRequests, want: "WARN"},
		{name: "5xx maps to error", status: http.StatusInternalServerError, want: "ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, accessLogLevel(tc.status).String())
		})
	}
}

func TestAccessLogMiddlewareEmitsStructuredFields(t *testing.T) {
	tests := []struct {
		name             string
		status           int
		body             string
		enrichIDs        bool
		wantLevel        string
		wantRateLimited  bool
		wantHasUserID    bool
		wantHasProjectID bool
		wantHasAppID     bool
	}{
		{name: "created response with enrichment", status: http.StatusCreated, body: `{"ok":true}`, enrichIDs: true, wantLevel: "INFO", wantRateLimited: false, wantHasUserID: true, wantHasProjectID: true, wantHasAppID: true},
		{name: "rate limited response without enrichment", status: http.StatusTooManyRequests, body: `{"error":"rate limited"}`, wantLevel: "WARN", wantRateLimited: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf, restore := withCapturedAccessLogger(t)
			defer restore()

			r := chi.NewRouter()
			r.Use(middleware.RequestID)
			r.Use(Middleware())
			r.Get("/v1/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
				if tc.enrichIDs {
					if lc, ok := MetadataFromContext(r.Context()); ok {
						lc.UserID = "github:t0gun"
						lc.ProjectID = "proj_123"
						lc.AppID = "app_456"
					}
				}
				w.WriteHeader(tc.status)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			})

			rr := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/projects/123", nil)
			req.RemoteAddr = "192.168.97.1:54321"
			req.Header.Set("User-Agent", "yaak")
			r.ServeHTTP(rr, req)
			require.Equal(t, tc.status, rr.Code)

			entry := decodeLastJSONLogEntry(t, buf)
			require.Equal(t, "api", entry["component"])
			require.Equal(t, "http_access", entry["msg"])
			require.Equal(t, tc.wantLevel, entry["level"])
			require.Equal(t, float64(tc.status), entry["status_code"])
			require.Equal(t, tc.wantRateLimited, entry["rate_limited"])
			require.Equal(t, "GET", entry["method"])
			require.Equal(t, "/v1/projects/{id}", entry["route"])
			require.Equal(t, "192.168.97.1", entry["client_ip"])
			require.NotEmpty(t, entry["request_id"])

			if tc.status >= http.StatusBadRequest {
			} else {
			}
			require.NotContains(t, entry, "user_agent")
			require.NotContains(t, entry, "bytes_out")
			require.NotContains(t, entry, "path")

			if tc.wantHasUserID {
				require.Equal(t, "github:t0gun", entry["user_id"])
			} else {
				require.NotContains(t, entry, "user_id")
			}
			if tc.wantHasProjectID {
				require.Equal(t, "proj_123", entry["project_id"])
			} else {
				require.NotContains(t, entry, "project_id")
			}
			if tc.wantHasAppID {
				require.Equal(t, "app_456", entry["app_id"])
			} else {
				require.NotContains(t, entry, "app_id")
			}
		})
	}
}

func TestAuthFailureEnrichesSingleAccessLog(t *testing.T) {
	buf, restore := withCapturedAccessLogger(t)
	defer restore()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(Middleware())
	r.Get("/v1/projects/{id}", func(w http.ResponseWriter, req *http.Request) {
		SetAuthFailure(req, "invalid_token")
		Error(w, http.StatusUnauthorized, "unauthorized")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/projects/123", nil)
	r.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	entries := decodeJSONLogEntries(t, buf)
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, "api", entry["component"])
	require.Equal(t, "WARN", entry["level"])
	require.Equal(t, float64(http.StatusUnauthorized), entry["status_code"])
	require.Equal(t, "invalid_token", entry["auth_reason"])
}

func TestRecovererMiddlewareRecoversPanicAndWritesInternalError(t *testing.T) {
	buf, restore := withCapturedAccessLogger(t)
	defer restore()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(Middleware())
	r.Use(Recoverer())
	r.Get("/v1/projects/{id}", func(_ http.ResponseWriter, r *http.Request) {
		if lc, ok := MetadataFromContext(r.Context()); ok {
			lc.UserID = "github:t0gun"
			lc.ProjectID = "proj_123"
		}
		panic("boom")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/projects/123", nil)
	req.RemoteAddr = "192.168.97.1:54321"
	req.Header.Set("User-Agent", "yaak")
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.JSONEq(t, `{"error":"internal error"}`, rr.Body.String())

	entries := decodeJSONLogEntries(t, buf)
	panicEntry := findEntryByEvent(t, entries, "panic")
	require.Equal(t, "api", panicEntry["component"])
	require.Equal(t, "ERROR", panicEntry["level"])
	require.Equal(t, "panic recovered", panicEntry["msg"])
	require.Equal(t, float64(http.StatusInternalServerError), panicEntry["status_code"])
	require.Equal(t, "string", panicEntry["panic_type"])
	require.Equal(t, "boom", panicEntry["panic_value"])
	require.Equal(t, "/v1/projects/{id}", panicEntry["route"])
	require.Equal(t, "/v1/projects/123", panicEntry["path"])
	require.Equal(t, "192.168.97.1", panicEntry["client_ip"])
	require.Equal(t, "yaak", panicEntry["user_agent"])
	require.Equal(t, "github:t0gun", panicEntry["user_id"])
	require.Equal(t, "proj_123", panicEntry["project_id"])
	require.NotContains(t, panicEntry, "app_id")
	require.NotContains(t, panicEntry, "stack_trace")
	require.NotEmpty(t, panicEntry["request_id"])

	accessEntry := findEntryByEvent(t, entries, "http_access")
	require.Equal(t, "api", accessEntry["component"])
	require.Equal(t, "ERROR", accessEntry["level"])
	require.Equal(t, float64(http.StatusInternalServerError), accessEntry["status_code"])
	require.Equal(t, false, accessEntry["rate_limited"])
	require.Equal(t, "/v1/projects/{id}", accessEntry["route"])
	require.Equal(t, "192.168.97.1", accessEntry["client_ip"])
	require.NotContains(t, accessEntry, "path")
	require.NotContains(t, accessEntry, "user_agent")
	require.NotContains(t, accessEntry, "bytes_out")
}

func TestRecovererMiddlewareDoesNotRewriteStartedResponse(t *testing.T) {
	buf, restore := withCapturedAccessLogger(t)
	defer restore()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(Middleware())
	r.Use(Recoverer())
	r.Get("/v1/projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("after write")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/projects/123", nil)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	require.Empty(t, strings.TrimSpace(rr.Body.String()))

	entries := decodeJSONLogEntries(t, buf)
	panicEntry := findEntryByEvent(t, entries, "panic")
	require.Equal(t, "ERROR", panicEntry["level"])
	require.Equal(t, "after write", panicEntry["panic_value"])

	accessEntry := findEntryByEvent(t, entries, "http_access")
	require.Equal(t, "INFO", accessEntry["level"])
	require.Equal(t, float64(http.StatusAccepted), accessEntry["status_code"])
}

func TestRecovererMiddlewareTruncatesLongPanicValue(t *testing.T) {
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

	entries := decodeJSONLogEntries(t, buf)
	panicEntry := findEntryByEvent(t, entries, "panic")
	require.Equal(t, strings.Repeat("a", maxPanicValueLogLen), panicEntry["panic_value"])
	require.NotContains(t, panicEntry, "stack_trace")
	require.NotContains(t, panicEntry, "user_agent")
}

func TestUserAgentLogAttr(t *testing.T) {
	tests := []struct {
		name      string
		rawUA     string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{name: "plain text user agent is logged", rawUA: "yaak", wantKey: "user_agent", wantValue: "yaak", wantOK: true},
		{name: "long user agent is truncated", rawUA: strings.Repeat("a", 300), wantKey: "user_agent", wantValue: strings.Repeat("a", maxUserAgentLogLen), wantOK: true},
		{name: "empty user agent is omitted", rawUA: "   ", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotValue, gotOK := userAgentLogAttr(tc.rawUA)
			require.Equal(t, tc.wantOK, gotOK)
			if !tc.wantOK {
				require.Empty(t, gotKey)
				require.Empty(t, gotValue)
				return
			}
			require.Equal(t, tc.wantKey, gotKey)
			require.Equal(t, tc.wantValue, gotValue)
		})
	}
}

func TestTruncateLogString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "no truncation needed", input: "abc", maxLen: 5, want: "abc"},
		{name: "ascii truncation", input: "abcdef", maxLen: 3, want: "abc"},
		{name: "zero max returns empty", input: "abcdef", maxLen: 0, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, truncateLogString(tc.input, tc.maxLen))
		})
	}
}

func TestPanicValueLogValue(t *testing.T) {
	require.Equal(t, strings.Repeat("a", maxPanicValueLogLen), panicValueLogValue(strings.Repeat("a", 300)))
	require.Equal(t, "12345", panicValueLogValue(12345))
}

func decodeJSONLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines)
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

func decodeLastJSONLogEntry(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	entries := decodeJSONLogEntries(t, buf)
	return entries[len(entries)-1]
}
