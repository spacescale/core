// Package requestlog owns request-scoped access and panic logging for the
// control HTTP API. It emits one structured access event per request and lets
// middleware or handlers attach safe, low-cardinality metadata discovered while
// handling the request.
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

// Metadata stores request log fields discovered after middleware setup.
type Metadata struct {
	UserID            string
	ProjectID         string
	AppID             string
	AuthFailureReason string
}

type contextKey struct{}

// MetadataFromContext returns mutable request log metadata when middleware installed it.
func MetadataFromContext(ctx context.Context) (*Metadata, bool) {
	v, ok := ctx.Value(contextKey{}).(*Metadata)
	return v, ok
}

// SetAuthFailure attaches a stable auth failure reason to the request access log.
func SetAuthFailure(r *http.Request, reason string) {
	if metadata, ok := MetadataFromContext(r.Context()); ok {
		metadata.AuthFailureReason = reason
	}
}

// Middleware emits one structured access log event after request completion.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			metadata := &Metadata{}
			req := r.WithContext(context.WithValue(r.Context(), contextKey{}, metadata))
			next.ServeHTTP(ww, req)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			attrs := []any{
				"component", "api",
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
			if metadata.UserID != "" {
				attrs = append(attrs, "user_id", metadata.UserID)
			}
			if metadata.ProjectID != "" {
				attrs = append(attrs, "project_id", metadata.ProjectID)
			}
			if metadata.AppID != "" {
				attrs = append(attrs, "app_id", metadata.AppID)
			}
			if metadata.AuthFailureReason != "" {
				attrs = append(attrs, "auth_reason", metadata.AuthFailureReason)
			}

			slog.Log(req.Context(), accessLogLevel(status), "http_access", attrs...)
		})
	}
}

// Recoverer logs panics and writes a generic 500 response when possible.
func Recoverer() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				attrs := []any{
					"component", "api",
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
				if metadata, ok := MetadataFromContext(r.Context()); ok {
					if metadata.UserID != "" {
						attrs = append(attrs, "user_id", metadata.UserID)
					}
					if metadata.ProjectID != "" {
						attrs = append(attrs, "project_id", metadata.ProjectID)
					}
					if metadata.AppID != "" {
						attrs = append(attrs, "app_id", metadata.AppID)
					}
				}

				slog.Error("panic recovered", attrs...)
				if responseWriterStatus(w) != 0 {
					return
				}
				Error(w, http.StatusInternalServerError, "internal error")
			}()
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
