// Package api owns access logging for the control HTTP API.
package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// AccessLogger emits one structured access log event after request completion.
func AccessLogger() func(http.Handler) http.Handler {
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

// requestOrigin resolves the caller IP and optional Cloudflare country code.
// CF headers are used when present so Geo-IP placement works behind Cloudflare
// without requiring a local MaxMind database in the first revision.
func requestOrigin(r *http.Request) (ip, country string) {
	country = strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
	if connectingIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); connectingIP != "" {
		return connectingIP, country
	}
	return clientIP(r.RemoteAddr), country
}
