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
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/service"
)

const (
	defaultUserRateLimitRequests = 100         // fallback per-user request budget when not explicitly configured.
	defaultUserRateLimitWindow   = time.Minute // fallback limiter window duration when not explicitly configured.
)

// RateLimitConfig describes per-authenticated-user API rate-limiting behavior.
//
// Requests defines how many requests one user can make during a single Window.
// Window defines the duration of one limiter bucket.
//
// Values less than or equal to zero are treated as invalid runtime input and
// normalized to safe defaults before middleware wiring.
type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

// DefaultRateLimitConfig returns the package default rate-limit configuration.
// Keeping defaults centralized here avoids repeating magic numbers across app
// startup, tests, and future binary entrypoints.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Requests: defaultUserRateLimitRequests,
		Window:   defaultUserRateLimitWindow,
	}
}

// normalized returns a safe runtime configuration for middleware wiring.
//
// This protects the router from accidental zero-value configuration and keeps
// behavior predictable even if callers forget to validate parsed env values.
func (c RateLimitConfig) normalized() RateLimitConfig {
	if c.Requests <= 0 {
		c.Requests = defaultUserRateLimitRequests
	}
	if c.Window <= 0 {
		c.Window = defaultUserRateLimitWindow
	}
	return c
}

// Server wires HTTP handlers to service dependencies and auth configuration.
//
// Runtime behavior fields:
//   - rateLimitCfg controls authenticated-user request budget enforcement.
//   - logPrivacyCfg controls user-agent representation and panic-log redaction
//     policy (panic value length and stack trace toggles).
type Server struct {
	svc           *service.ProjectService
	authCfg       AuthConfig
	dbPool        *pgxpool.Pool
	rateLimitCfg  RateLimitConfig
	logPrivacyCfg LogPrivacyConfig
}

// NewServer creates a Server bound to the provided service and middleware
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
func NewServer(svc *service.ProjectService, authCfg AuthConfig, dbPool *pgxpool.Pool, rateLimitCfg RateLimitConfig, logPrivacyCfg LogPrivacyConfig) *Server {
	return &Server{
		svc:           svc,
		authCfg:       authCfg,
		dbPool:        dbPool,
		rateLimitCfg:  rateLimitCfg.normalized(),
		logPrivacyCfg: logPrivacyCfg.normalized(),
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
	r.Use(accessLogMiddleware(s.logPrivacyCfg))
	r.Use(recovererMiddleware(s.logPrivacyCfg))

	// userLimiter applies per-authenticated-user request limits on API v0 routes.
	// Keys come from keyByGithubID, so limits are enforced by JWT identity
	// (github:<id>) instead of source IP. This keeps limits fair when requests
	// are proxied through Next.js, CLI backends, or shared infrastructure.
	// Exceeded requests receive a consistent HTTP 429 JSON error response.
	userLimiter := httprate.Limit(s.rateLimitCfg.Requests, s.rateLimitCfg.Window, httprate.WithKeyFuncs(keyByGithubID),
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
		r.Use(authMiddleware(s.authCfg, s.logPrivacyCfg))
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
