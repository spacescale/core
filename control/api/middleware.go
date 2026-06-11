// Package api owns request-scoped access and panic logging for the
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
	"strings"
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

const (
	maxUserAgentLogLen  = 255
	maxPanicValueLogLen = 200
)

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
			ctx := context.WithValue(r.Context(), contextKey{}, metadata)
			req := r.WithContext(ctx)
			next.ServeHTTP(ww, req)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			attrs := []any{
				"component", "api",
				"request_id", middleware.GetReqID(ctx),
				"method", req.Method,
				"status_code", status,
				"rate_limited", status == http.StatusTooManyRequests,
				"duration_ms", time.Since(start).Milliseconds(),
				"client_ip", clientIP(req.RemoteAddr),
			}

			route := routePatternFromContext(ctx)
			attrs = append(attrs, "route", route)
			if route == "-" {
				attrs = append(attrs, "path", req.URL.Path)
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

			slog.Log(ctx, accessLogLevel(status), "http_access", attrs...)
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

				if key, value, ok := userAgentLogAttr(r.UserAgent()); ok {
					attrs = append(attrs, key, value)
				}
				if metadata, ok := MetadataFromContext(ctx); ok {
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

func userAgentLogAttr(rawUserAgent string) (string, string, bool) {
	ua := strings.TrimSpace(rawUserAgent)
	if ua == "" {
		return "", "", false
	}
	return "user_agent", truncateLogString(ua, maxUserAgentLogLen), true
}

func truncateLogString(input string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen]
}

func panicValueLogValue(recovered any) string {
	return truncateLogString(fmt.Sprint(recovered), maxPanicValueLogLen)
}
