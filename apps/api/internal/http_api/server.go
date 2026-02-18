// This file builds the API router and middleware stack.
// It defines the Server wrapper that receives service dependencies and exposes
// one Router method used by main and integration tests.
// Route registration for health checks and versioned endpoints is centralized
// here so application wiring is discoverable in one place.
// In addition to routing, this file owns composition wiring for server-level
// runtime configuration (for example rate limits and log privacy). Config model
// definitions live in focused files, while this file stays responsible for
// assembling middleware and route behavior.
// When adding new endpoints or middleware-level wiring, changes should begin
// here so composition remains discoverable in one place.

// Package http_api provides routing and middleware wiring.
package http_api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/config"
	"github.com/t0gun/spacescale/internal/service"
)

// Server wires HTTP handlers to service dependencies and auth configuration.
//
// Runtime behavior fields:
//   - config.RateLimit controls authenticated-user request budget enforcement.
//   - config.LogPrivacy controls user-agent representation and panic-log
//     redaction policy (panic value length and stack trace toggles).
type Server struct {
	services *service.Services
	dbPool   *pgxpool.Pool
	config   config.APIConfig
}

// ServerDeps groups dependencies required to construct the API server.
// It keeps startup and test wiring concise while making required inputs
// explicit at one call site.
type ServerDeps struct {
	Services *service.Services
	DBPool   *pgxpool.Pool
	Config   config.APIConfig
}

// NewServer creates a Server bound to provided dependencies and middleware
// runtime configuration.
//
// Construction behavior:
//   - Normalizes rate-limit config so zero-value callers still get safe runtime
//     limiter behavior.
//   - Normalizes log-privacy config so middleware sees valid mode/length values
//     even when startup config is incomplete.
//
// Keeping this wiring constructor explicit makes startup and tests easier to
// understand because dependency and config flow is visible at the call site.
func NewServer(deps ServerDeps) *Server {
	if deps.Services == nil {
		panic("http_api.NewServer requires non-nil services")
	}
	if deps.Services.Projects == nil {
		panic("http_api.NewServer requires non-nil project service")
	}
	if deps.Services.Users == nil {
		panic("http_api.NewServer requires non-nil user service")
	}
	if deps.DBPool == nil {
		panic("http_api.NewServer requires non-nil db pool")
	}

	normalizedConfig := deps.Config.Normalized()
	if normalizedConfig.InternalAuthSecret == "" {
		panic("http_api.NewServer requires non-empty internal auth secret")
	}

	return &Server{
		services: deps.Services,
		dbPool:   deps.DBPool,
		config:   normalizedConfig,
	}
}

// Router builds the full HTTP router and middleware stack.
// It registers health and versioned API routes, then applies request-level
// middleware for traceability, logging, panic recovery, and authentication.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Base middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) // keep client IP extraction before logging middleware
	r.Use(accessLogMiddleware(s.config.LogPrivacy))
	r.Use(recovererMiddleware(s.config.LogPrivacy))

	// userLimiter applies per-authenticated-user request limits on API v0 routes.
	// Keys come from keyByIdentityKey, so limits are enforced by authenticated
	// identity key instead of source IP. This keeps limits fair when requests
	// are proxied through Next.js, CLI backends, or shared infrastructure.
	// Exceeded requests receive a consistent HTTP 429 JSON error response.
	userLimiter := httprate.Limit(s.config.RateLimit.Requests, s.config.RateLimit.Window, httprate.WithKeyFuncs(keyByIdentityKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		}),
	)

	// Health check route.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.dbPool.Ping(r.Context()); err != nil { // check database connectivity
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/v0/internal", func(r chi.Router) {
		r.Use(internalAuthMiddleware(s.config.InternalAuthSecret))
		r.Post("/auth-sync", s.handleSyncAuthUser)
	})

	r.Route("/v0", func(r chi.Router) {
		r.Use(authMiddleware(s.config.Auth, s.config.LogPrivacy))
		r.Use(userLimiter)
		r.Post("/projects", s.handleCreateProject)
	})

	return r
}

// keyByIdentityKey returns the rate-limit key for a request based on the
// authenticated identity key stored in request context.
//
// The key format is "identity:<key>", which makes rate limiting apply per user
// across all requests from that identity key.
//
// Middleware order matters: auth middleware must run before the limiter so the
// principal is already present in context when this function executes.
//
// If principal data is missing or empty, the function returns the fallback key
// "identity:unknown" instead of an error. This keeps limiter behavior predictable
// and avoids triggering httprate's error handler path.
func keyByIdentityKey(r *http.Request) (string, error) {
	p, ok := principalFromContext(r.Context())
	if !ok || p.IdentityKey == "" {
		// defensive fallback bucket; avoids httprate error handler path
		return "identity:unknown", nil
	}
	return "identity:" + p.IdentityKey, nil
}

// internalAuthMiddleware protects trusted internal endpoints with a shared
// secret header.
//
// The middleware expects header "X-Internal-Auth" and compares it using
// constant-time equality. Requests with missing or incorrect secrets receive
// an unauthorized response.
func internalAuthMiddleware(expectedSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expected := strings.TrimSpace(expectedSecret)
			provided := strings.TrimSpace(r.Header.Get("X-Internal-Auth"))

			if expected == "" || provided == "" {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
