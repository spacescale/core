// Package api owns access logging and panic recovery for the control HTTP API.
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

const maxPanicValueLogLen = 200

// Middleware emits one structured access log event after request completion.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			attrs := []any{
				"component", "api",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"status_code", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"client_ip", clientIP(r.RemoteAddr),
			}
			route := routePatternFromContext(r.Context())
			attrs = append(attrs, "route", route)
			if route == "-" {
				attrs = append(attrs, "path", r.URL.Path)
			}

			slog.Log(r.Context(), accessLogLevel(status), "http_access", attrs...)
		})
	}
}

// Recoverer logs panics and writes a generic 500 response when possible.
func Recoverer() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			defer func(ctx context.Context) {
				recovered := recover()
				if recovered == nil {
					return
				}

				attrs := []any{
					"component", "api",
					"event", "panic",
					"request_id", middleware.GetReqID(ctx),
					"method", r.Method,
					"route", routePatternFromContext(ctx),
					"path", r.URL.Path,
					"status_code", http.StatusInternalServerError,
					"client_ip", clientIP(r.RemoteAddr),
					"panic_type", fmt.Sprintf("%T", recovered),
					"panic_value", panicValueLogValue(recovered),
				}

				slog.Error("panic recovered", attrs...)
				type statusProvider interface{ Status() int }
				if statusAwareWriter, ok := w.(statusProvider); ok && statusAwareWriter.Status() != 0 {
					return
				}
				Error(w, http.StatusInternalServerError, "internal error")
			}(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func accessLogLevel(status int) slog.Level {
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}
	if status >= http.StatusBadRequest {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

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

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func panicValueLogValue(recovered any) string {
	value := fmt.Sprint(recovered)
	if len(value) <= maxPanicValueLogLen {
		return value
	}
	return value[:maxPanicValueLogLen]
}
