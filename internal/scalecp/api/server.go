package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/api/auth"
	"github.com/spacescale/core/internal/scalecp/api/requestlog"
	"github.com/spacescale/core/internal/scalecp/api/respond"
	"github.com/spacescale/core/internal/scalecp/fabric/dispatch"
	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
	"github.com/spacescale/core/internal/shared/config"
)

// Server wires HTTP handlers to service dependencies and auth configuration.
type Server struct {

	// multi tenant service layer
	users      *tenant.UserService
	projects   *tenant.ProjectService
	workspaces *tenant.WorkspaceService
	bootstrap  *tenant.BootstrapService
	apps       *tenant.AppService

	// dependencies
	dbPool                  *pgxpool.Pool
	config                  config.Config
	internalIdentityLimiter *httprate.RateLimiter
	server                  *http.Server

	// fabric dependencies
	dispatcher *dispatch.Dispatcher
}

const (
	internalGlobalLimiterKey        = "internal:global"
	userRateLimitRequests           = 200
	userRateLimitWindow             = time.Minute
	internalGlobalRateLimitRequests = 24000
	internalGlobalRateLimitWindow   = time.Minute
)

// ServerDeps groups dependencies required to construct the API server.
// It keeps startup and test wiring concise while making required inputs
// explicit at one call site.
type ServerDeps struct {
	Services *service.Services
	DBPool   *pgxpool.Pool
	Config   config.Config

	// fabric dependencies
	Dispatcher *dispatch.Dispatcher
}

func NewServer(deps ServerDeps) *Server {
	tenantServices := deps.Services.Tenant
	s := &Server{
		users:                   tenantServices.Users,
		projects:                tenantServices.Projects,
		workspaces:              tenantServices.Workspaces,
		bootstrap:               tenantServices.Bootstrap,
		apps:                    tenantServices.Apps,
		dbPool:                  deps.DBPool,
		config:                  deps.Config,
		internalIdentityLimiter: auth.NewSyncIdentityLimiter(),

		// fabric dependencies
		dispatcher: deps.Dispatcher,
	}
	s.server = &http.Server{
		Addr:              deps.Config.ListenAddr(),
		Handler:           http.MaxBytesHandler(s.Router(), 1<<20),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http serve failed: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http shutdown failed: %w", err)
	}
	return nil
}

// Router builds the full HTTP router and middleware stack.
// It registers health and versioned API routes, then applies request-level
// middleware for traceability, logging, panic recovery, and authentication.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Base middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) // keep client IP extraction before logging middleware
	r.Use(requestlog.Middleware())
	r.Use(requestlog.Recoverer())

	// userLimiter applies per-authenticated-user request limits on API v1 routes.
	// Keys come from auth.KeyByIdentityKey, so limits are enforced by authenticated
	// identity key instead of source IP. This keeps limits fair when requests
	// are proxied through Next.js, CLI backends, or shared infrastructure.
	// Exceeded requests receive a consistent HTTP 429 JSON error response.
	userLimiter := httprate.Limit(userRateLimitRequests, userRateLimitWindow, httprate.WithKeyFuncs(auth.KeyByIdentityKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			rateLimitExceeded(w, r)
		}),
	)

	internalGlobalLimiter := httprate.NewRateLimiter(
		internalGlobalRateLimitRequests,
		internalGlobalRateLimitWindow,
		httprate.WithKeyFuncs(httprate.Key(internalGlobalLimiterKey)),
		httprate.WithLimitHandler(rateLimitExceeded),
	)

	// Health check route.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.dbPool.Ping(r.Context()); err != nil { // check database connectivity
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Internal routes are intended for private-network service-to-service traffic.
	// They apply both a global circuit breaker and per-identity guardrails.
	r.Route("/v1/internal", func(r chi.Router) {
		r.Use(internalGlobalLimiter.Handler)
		r.Use(auth.InternalMiddleware(s.config.InternalAuthSecret))
		r.Post("/auth-sync", auth.SyncUserHandler(s.users, s.internalIdentityLimiter))
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.Middleware(s.config.Auth))
		r.Use(userLimiter)
		r.Post("/bootstrap-defaults", s.handleBootstrapDefaults)

		// Workspaces endpoints.
		r.Post("/workspaces", s.handleCreateWorkspace)
		r.Get("/workspaces", s.handleListWorkspaces)
		r.Get("/workspaces/{workspaceId}", s.handleGetWorkspace)
		r.Patch("/workspaces/{workspaceId}", s.handleUpdateWorkspace)
		r.Delete("/workspaces/{workspaceId}", s.handleDeleteWorkspace)

		// Projects endpoints.
		r.Post("/workspaces/{workspaceId}/projects", s.handleCreateProject)
		r.Get("/workspaces/{workspaceId}/projects", s.handleListProjects)
		r.Get("/workspaces/{workspaceId}/projects/{projectId}", s.handleGetProject)
		r.Patch("/workspaces/{workspaceId}/projects/{projectId}", s.handleUpdateProject)
		r.Delete("/workspaces/{workspaceId}/projects/{projectId}", s.handleDeleteProject)

		// Apps endpoints.
		r.Get("/workspaces/{workspaceId}/projects/{projectId}/apps", s.handleListApps)
		r.Post("/workspaces/{workspaceId}/projects/{projectId}/apps", s.handleCreateApp)
	})

	return r
}

// rateLimitExceeded keeps 429 responses consistent across route groups.
func rateLimitExceeded(w http.ResponseWriter, _ *http.Request) {
	respond.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
}
