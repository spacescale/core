// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// This file implements request-scoped access and panic logging middleware.
// It emits one structured access event per request and one structured panic
// event when recovery is needed. Shared request context allows downstream auth
// and handlers to enrich user/project/app identifiers without duplicating log
// emission logic.

package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// logContext stores optional request metadata that may be discovered mid-request
// and emitted by access/panic logging only when populated.
type logContext struct {
	UserID    string
	ProjectID string
	AppID     string
}

// logContextKey is an unexported key type for context value isolation.
// Using a private type prevents collisions with context keys from other packages.
type logContextKey struct{}

// withLogContext attaches mutable request log metadata to context.
func withLogContext(ctx context.Context, v *logContext) context.Context {
	return context.WithValue(ctx, logContextKey{}, v)
}

// logContextFromContext retrieves request log metadata when present.
func logContextFromContext(ctx context.Context) (*logContext, bool) {
	v, ok := ctx.Value(logContextKey{}).(*logContext)
	return v, ok
}

// accessLogMiddleware emits one structured access log after handler completion.
// It captures final response status, latency, and optional request enrichments.
func accessLogMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			lc := &logContext{}
			req := r.WithContext(withLogContext(r.Context(), lc))
			next.ServeHTTP(ww, req)

			// Match net/http behavior: no explicit WriteHeader means 200.
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			attrs := []any{
				"event", "http_access",
				"request_id", middleware.GetReqID(req.Context()),
				"method", req.Method,
				"route", routePatternFromContext(req.Context()),
				"path", req.URL.Path,
				"status_code", status,
				"rate_limited", status == http.StatusTooManyRequests,
				"duration_ms", time.Since(start).Milliseconds(),
				"bytes_out", ww.BytesWritten(),
				"client_ip", clientIP(req.RemoteAddr),
			}

			if key, value, ok := userAgentLogAttr(req.UserAgent()); ok {
				attrs = append(attrs, key, value)
			}

			if lc.UserID != "" {
				attrs = append(attrs, "user_id", lc.UserID)
			}
			if lc.ProjectID != "" {
				attrs = append(attrs, "project_id", lc.ProjectID)
			}
			if lc.AppID != "" {
				attrs = append(attrs, "app_id", lc.AppID)
			}

			slog.Log(req.Context(), accessLogLevel(status), "http_access", attrs...)
		})
	}
}

// accessLogLevel maps response classes to access-log severity.
func accessLogLevel(status int) slog.Level {
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}
	if status >= http.StatusBadRequest {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

// recovererMiddleware logs panic details and returns a generic 500 when possible.
// It avoids writing a second response if downstream already started one.
func recovererMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				attrs := []any{
					"event", "panic",
					"request_id", middleware.GetReqID(r.Context()),
					"method", r.Method,
					"route", routePatternFromContext(r.Context()),
					"path", r.URL.Path,
					"status_code", http.StatusInternalServerError,
					"client_ip", clientIP(r.RemoteAddr),
					"panic_type", fmt.Sprintf("%T", recovered),
					"panic_value", panicValueLogValue(recovered),
				}

				if key, value, ok := userAgentLogAttr(r.UserAgent()); ok {
					attrs = append(attrs, key, value)
				}

				if lc, ok := logContextFromContext(r.Context()); ok {
					if lc.UserID != "" {
						attrs = append(attrs, "user_id", lc.UserID)
					}
					if lc.ProjectID != "" {
						attrs = append(attrs, "project_id", lc.ProjectID)
					}
					if lc.AppID != "" {
						attrs = append(attrs, "app_id", lc.AppID)
					}
				}
				slog.Error("panic recovered", attrs...)

				if responseWriterStatus(w) != 0 {
					return
				}
				writeErr(w, http.StatusInternalServerError, "internal error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// responseWriterStatus returns current status code for status-aware writers.
// It returns 0 when status cannot be inspected.
func responseWriterStatus(w http.ResponseWriter) int {
	type statusProvider interface {
		Status() int
	}

	statusAwareWriter, ok := w.(statusProvider)
	if !ok {
		return 0
	}
	return statusAwareWriter.Status()
}

// routePatternFromContext returns matched chi route pattern for low-cardinality
// logging, or "-" when unavailable.
func routePatternFromContext(ctx context.Context) string {
	rctx := chi.RouteContext(ctx)
	if rctx == nil {
		return "-"
	}
	if pattern := rctx.RoutePattern(); pattern != "" {
		return pattern
	}
	return "-"
}

// clientIP returns host part of RemoteAddr (ip:port) when parseable.
// If parsing fails, original input is returned unchanged.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
