// This file implements structured access logging middleware for HTTP traffic.
//
// What this middleware does:
// - Wraps the response writer so final status code and bytes written can be read.
// - Starts a timer before calling the downstream middleware/handler chain.
// - Attaches a shared request-scoped log context for enrichment (user/project/app).
// - Logs exactly one JSON access event after request completion.
//
// Why we log after handler execution:
// - Final response status is only known after downstream handlers return.
// - Total duration can only be measured end-to-end.
// - Optional identifiers (user_id/project_id/app_id) may be discovered mid-request.
//
// Why this is request-scoped and context-backed:
// - Auth middleware and handlers run deeper in the chain and can enrich metadata.
// - Access middleware remains the single place that emits final access logs.
// - The shared context object avoids duplicated logging logic in each endpoint.
//
// Operational goal:
// - Produce stable, query-friendly JSON logs suitable for container/Kubernetes
//   collection pipelines, local debugging, and production dashboards.

package http_api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// logContext stores mutable per-request metadata used by access logging.
//
// Data is intentionally small and request-scoped only.
// Downstream code can fill fields as information becomes available.
// The access logger emits these fields only when non-empty.
//
// Field intent:
// - UserID: authenticated caller identity (for example "github:t0gun").
// - ProjectID: resolved/created project id for routes that operate on projects.
// - AppID: resolved/created app id for routes that operate on apps.
type logContext struct {
	UserID    string
	ProjectID string
	AppID     string
}

// logContextKey is an unexported key type for context value isolation.
// Using a private type prevents collisions with context keys from other packages.
type logContextKey struct{}

// withLogContext attaches a shared logContext pointer to request context.
//
// Important behavior:
//   - Context itself is immutable, but the stored pointer value can be mutated.
//   - This allows nested middleware/handlers to enrich fields without re-plumbing
//     a custom struct through every function signature.
func withLogContext(ctx context.Context, v *logContext) context.Context {
	return context.WithValue(ctx, logContextKey{}, v)
}

// logContextFromContext retrieves the shared logContext pointer from context.
//
// Return contract:
// - (*logContext, true): access-log context exists and may be enriched.
// - (nil, false): no context attached; caller should continue safely.
func logContextFromContext(ctx context.Context) (*logContext, bool) {
	v, ok := ctx.Value(logContextKey{}).(*logContext)
	return v, ok
}

// accessLogMiddleware emits one structured JSON access log after each request.
//
// Processing flow summary:
// 1) Wrap the response writer so status code and bytes written can be captured.
// 2) Attach a shared request log context for downstream enrichment.
// 3) Execute the next handler chain.
// 4) Resolve route metadata and emit a single "http_access" event.
//
// Emitted fields include request id, method/path/route, status, latency, bytes,
// client metadata, rate-limit hint, and optional identity/resource ids.
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture request start timestamp before invoking downstream chain.
		start := time.Now()

		// Wrap the writer so we can read final status and bytes after response.
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Create and attach shared metadata context before entering downstream chain.
		// Downstream middleware/handlers can enrich this object (for example user_id).
		lc := &logContext{}
		req := r.WithContext(withLogContext(r.Context(), lc))

		// Call the rest of the middleware+handler chain.
		// This call is synchronous: execution continues below only when request
		// handling is finished and response has been written.
		next.ServeHTTP(ww, req)

		// If WriteHeader was never called, net/http treats it as 200.
		// Mirror that behavior so logs match actual response semantics.
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}

		// Build base access payload using final request/response metadata.
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
			"user_agent", req.UserAgent(),
		}

		// Emit optional resource/identity fields only when present.
		// This keeps logs compact and avoids noisy empty-string fields.
		if lc.UserID != "" {
			attrs = append(attrs, "user_id", lc.UserID)
		}
		if lc.ProjectID != "" {
			attrs = append(attrs, "project_id", lc.ProjectID)
		}
		if lc.AppID != "" {
			attrs = append(attrs, "app_id", lc.AppID)
		}

		// Choose log level from final response class and emit one completion event.
		slog.Log(req.Context(), accessLogLevel(status), "http_access", attrs...)
	})
}

// accessLogLevel maps HTTP response classes to slog levels for access events.
//
// Level mapping policy:
// - 2xx/3xx: Info (expected success and redirects)
// - 4xx: Warn (client-visible failure worth attention, not server fault)
//   - includes 429 Too Many Requests (rate limiting)
//
// - 5xx: Error (server-side failure path)
//
// This keeps request completion logging consistent while making failures stand
// out in production log filters and dashboards.
func accessLogLevel(status int) slog.Level {
	// 5xx means server-side error path.
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}
	// 4xx means client-visible failure path (auth/validation/rate-limit, etc).
	if status >= http.StatusBadRequest {
		return slog.LevelWarn
	}
	// 1xx/2xx/3xx use info level.
	return slog.LevelInfo
}

// recovererMiddleware wraps request handling with panic recovery and structured
// panic logging.
//
// Why this lives in the logging middleware file:
//   - Panic recovery is part of request-level observability.
//   - Keeping access and panic logging together makes HTTP logging behavior easy
//     to discover and maintain from one location.
//
// Execution flow:
//  1. Register deferred recovery closure.
//  2. Execute downstream middleware/handler chain.
//  3. If panic occurs, recover value, emit structured panic log, and return
//     generic 500 only when response was not already started.
func recovererMiddleware(next http.Handler) http.Handler {
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
				"user_agent", r.UserAgent(),
				"panic_type", fmt.Sprintf("%T", recovered),
				"panic_value", fmt.Sprint(recovered),
				"stack_trace", string(debug.Stack()),
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

			// If downstream already wrote a status/body, do not attempt to write
			// another response during panic recovery.
			if responseWriterStatus(w) != 0 {
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal error")
		}()

		next.ServeHTTP(w, r)
	})
}

// responseWriterStatus returns the currently written status code when the writer
// supports status inspection (for example chi WrapResponseWriter), otherwise 0.
//
// Why this helper exists:
//   - Panic recovery should avoid writing a second status/body when downstream
//     code already started the response.
func responseWriterStatus(w http.ResponseWriter) int {
	// statusProvider describes writers that expose current response status.
	type statusProvider interface {
		Status() int
	}

	// Safe runtime type assertion (comma-ok form).
	statusAwareWriter, ok := w.(statusProvider)
	if !ok {
		return 0
	}
	return statusAwareWriter.Status()
}

// routePatternFromContext resolves matched chi route pattern from context.
//
// Why route pattern is logged:
// - It has lower cardinality than raw path and is easier to aggregate.
// - It is more stable for dashboards/alerts than id-heavy path values.
//
// Fallback behavior:
//   - Returns "-" when route context/pattern is unavailable (for example early
//     middleware exits before a concrete route pattern is finalized).
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

// clientIP extracts host-only client IP from RemoteAddr.
//
// Expected input:
// - RemoteAddr is usually in "ip:port" form.
//
// Return behavior:
// - Returns only host/IP part when parsing succeeds.
// - Returns original input unchanged when parsing fails.
//
// Note:
//   - RealIP middleware should run before this middleware so RemoteAddr reflects
//     best-available client address from trusted forwarding headers.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
