// Package api owns scalecp HTTP server lifecycle and route composition.
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
	"github.com/spacescale/core/scalecp/fabric"
	"github.com/spacescale/core/scalecp/service"
	"github.com/spacescale/core/scalecp/service/tenant"
	"github.com/spacescale/core/shared/config"
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
	dispatcher *fabric.Dispatcher
}

const (
	internalGlobalLimiterKey        = "internal:global"
	maxRequestBodyBytes             = 1 << 20
	serverReadHeaderTimeout         = 5 * time.Second
	serverWriteTimeout              = 30 * time.Second
	serverIdleTimeout               = 120 * time.Second
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
	Dispatcher *fabric.Dispatcher
}

// NewServer constructs a scalecp API server from service dependencies.
func NewServer(deps ServerDeps) *Server {
	tenantServices := deps.Services.Tenant
	apiServer := &Server{
		users:                   tenantServices.Users,
		projects:                tenantServices.Projects,
		workspaces:              tenantServices.Workspaces,
		bootstrap:               tenantServices.Bootstrap,
		apps:                    tenantServices.Apps,
		dbPool:                  deps.DBPool,
		config:                  deps.Config,
		internalIdentityLimiter: NewSyncIdentityLimiter(),

		// fabric dependencies
		dispatcher: deps.Dispatcher,
		server:     nil,
	}
	server := new(http.Server)
	server.Addr = deps.Config.ListenAddr()
	server.Handler = http.MaxBytesHandler(apiServer.Router(), maxRequestBodyBytes)
	server.ReadHeaderTimeout = serverReadHeaderTimeout
	server.WriteTimeout = serverWriteTimeout
	server.IdleTimeout = serverIdleTimeout
	apiServer.server = server

	return apiServer
}

// Start listens for HTTP requests until the server is shut down.
func (s *Server) Start() error {
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http serve failed: %w", err)
	}

	return nil
}

// Shutdown gracefully stops the HTTP server.
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
	router := chi.NewRouter()

	// Base middleware stack.
	router.Use(middleware.RequestID)
	router.Use(middleware.ClientIPFromRemoteAddr)
	router.Use(Middleware())
	router.Use(Recoverer())

	// userLimiter applies per-authenticated-user request limits on API v1 routes.
	// Keys come from KeyByIdentityKey, so limits are enforced by authenticated
	// identity key instead of source IP. This keeps limits fair when requests
	// are proxied through Next.js, CLI backends, or shared infrastructure.
	// Exceeded requests receive a consistent HTTP 429 JSON error response.
	userLimiter := httprate.Limit(userRateLimitRequests, userRateLimitWindow, httprate.WithKeyFuncs(KeyByIdentityKey),
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
	router.Get("/healthz", func(responseWriter http.ResponseWriter, r *http.Request) {
		if err := s.dbPool.Ping(r.Context()); err != nil { // check database connectivity
			responseWriter.WriteHeader(http.StatusServiceUnavailable)

			return
		}
		responseWriter.WriteHeader(http.StatusOK)
	})

	// Internal routes are intended for private-network service-to-service traffic.
	// They apply both a global circuit breaker and per-identity guardrails.
	router.Route("/v1/internal", func(r chi.Router) {
		r.Use(internalGlobalLimiter.Handler)
		r.Use(InternalAuthMiddleware(s.config.InternalAuthSecret))
		r.Post("/auth-sync", SyncUserHandler(s.users, s.internalIdentityLimiter))
	})

	router.Route("/v1", func(apiRouter chi.Router) {
		apiRouter.Use(AuthMiddleware(s.config.Auth))
		apiRouter.Use(userLimiter)

		apiRouter.Post("/bootstrap-defaults", s.handleBootstrapDefaults)

		s.registerV1Routes(apiRouter)
	})

	return router
}

// registerV1Routes registers the authenticated v1 routes on router.
func (s *Server) registerV1Routes(router chi.Router) {
	router.Post("/workspaces", s.handleCreateWorkspace)
	router.Get("/workspaces", s.handleListWorkspaces)
	router.Get("/workspaces/{workspaceId}", s.handleGetWorkspace)
	router.Patch("/workspaces/{workspaceId}", s.handleUpdateWorkspace)
	router.Delete("/workspaces/{workspaceId}", s.handleDeleteWorkspace)

	router.Post("/workspaces/{workspaceId}/projects", s.handleCreateProject)
	router.Get("/workspaces/{workspaceId}/projects", s.handleListProjects)
	router.Get("/workspaces/{workspaceId}/projects/{projectId}", s.handleGetProject)
	router.Patch("/workspaces/{workspaceId}/projects/{projectId}", s.handleUpdateProject)
	router.Delete("/workspaces/{workspaceId}/projects/{projectId}", s.handleDeleteProject)

	router.Get("/workspaces/{workspaceId}/projects/{projectId}/apps", s.handleListApps)
	router.Post("/workspaces/{workspaceId}/projects/{projectId}/apps", s.handleCreateApp)
}

// rateLimitExceeded keeps 429 responses consistent across route groups.
func rateLimitExceeded(w http.ResponseWriter, _ *http.Request) {
	Error(w, http.StatusTooManyRequests, "rate limit exceeded")
}
