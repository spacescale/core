// This file builds the API router and middleware stack.
// It defines the Server wrapper that receives service dependencies and exposes
// one Router method used by main and integration tests.
// Route registration for health checks and versioned endpoints is centralized
// here so application wiring is discoverable in one place.
// When adding new endpoints, most HTTP route setup changes should begin here.

// Package http_api  routing and middleware wiring.
package http_api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/service"
)

// Server wires HTTP handlers to service dependencies and auth configuration.
type Server struct {
	svc     *service.ProjectService
	authCfg AuthConfig
	dbPool  *pgxpool.Pool
}

// NewServer creates a Server bound to the provided project service.
// Keeping wiring here makes dependencies explicit for startup and tests.
func NewServer(svc *service.ProjectService, authCfg AuthConfig, dbPool *pgxpool.Pool) *Server {
	return &Server{
		svc:     svc,
		authCfg: authCfg,
		dbPool:  dbPool,
	}
}

// Router builds the full HTTP router and middleware stack.
// It registers health and versioned API routes, then applies request-level
// middleware for traceability, logging, panic recovery, and authentication.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Base middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) // fine to keep for logs; limiter key is user id
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// userLimiter applies per-authenticated-user request limits on API v0 routes.
	// Keys come from keyByGithubID, so limits are enforced by JWT identity
	// (github:<id>) instead of source IP. This keeps limits fair when requests
	// are proxied through Next.js, CLI backends, or shared infrastructure.
	// Exceeded requests receive a consistent HTTP 429 JSON error response.
	userLimiter := httprate.Limit(100, time.Minute, httprate.WithKeyFuncs(keyByGithubID),
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

	r.Route("/v0", func(r chi.Router) {
		r.Use(authMiddleware(s.authCfg))
		r.Use(userLimiter)
		r.Post("/projects", s.handleCreateProject)
	})

	return r
}

// keyByGithubID returns the rate-limit key for a request based on the
// authenticated GitHub identity stored in request context.
//
// The key format is "GitHub:<id>", which makes rate limiting apply per user
// across all requests from that identity.
//
// Middleware order matters: auth middleware must run before the limiter so the
// principal is already present in context when this function executes.
//
// If principal data is missing or empty, the function returns the fallback key
// "github:unknown" instead of an error. This keeps limiter behavior predictable
// and avoids triggering httprate's error handler path.
func keyByGithubID(r *http.Request) (string, error) {
	p, ok := principalFromContext(r.Context())
	if !ok || p.GithubID == "" {
		// defensive fallback bucket; avoids httprate error handler path
		return "github:unknown", nil
	}
	return "github:" + p.GithubID, nil
}
