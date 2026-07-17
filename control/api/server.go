// Package api owns the control-plane HTTP surface.
// It wires routing, middleware, rate limiting, tenant services, and auth state
// into the server that handles API requests.
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
	"github.com/spacescale/core/control/fabric"
	"github.com/spacescale/core/control/placement"
	"github.com/spacescale/core/control/tenant"
	"github.com/spacescale/core/shared/config"
	"github.com/workos/workos-go/v9"
)

const (
	maxRequestBodyBytes     = 1 << 20
	serverReadHeaderTimeout = 5 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 120 * time.Second
	userRateLimitRequests   = 1000
	userRateLimitWindow     = time.Minute
)

// Server wires HTTP handlers to tenant services, database access, and auth state.
type Server struct {
	users      *tenant.UserService
	projects   *tenant.ProjectService
	workspaces *tenant.WorkspaceService
	bootstrap  *tenant.BootstrapService
	workloads  *tenant.WorkloadService

	// dependencies
	dbPool *pgxpool.Pool
	auth   *workOSAuth
	server *http.Server

	// fabric dependencies
	dispatcher *fabric.Dispatcher
	placement  *placement.Catalog
}

// Principal is the authenticated identity stored in request context.
// Handlers use it instead of reading auth state directly.
type Principal struct {
	Subject     string
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
}

type principalContextKey struct{}

// ServerDeps groups the dependencies required to construct the API server.
// It keeps startup and test wiring explicit at one call site.
type ServerDeps struct {
	Users        *tenant.UserService
	Projects     *tenant.ProjectService
	Workspaces   *tenant.WorkspaceService
	Bootstrap    *tenant.BootstrapService
	Workloads    *tenant.WorkloadService
	DBPool       *pgxpool.Pool
	Config       config.Control
	Dispatcher   *fabric.Dispatcher
	Placement    *placement.Catalog
	WorkOSClient *workos.Client
}

// NewServer constructs a control API server from services and config.
// It also preconfigures the underlying http.Server with body limits and
// timeout settings.
func NewServer(deps ServerDeps) *Server {
	apiServer := &Server{
		users:      deps.Users,
		projects:   deps.Projects,
		workspaces: deps.Workspaces,
		bootstrap:  deps.Bootstrap,
		workloads:  deps.Workloads,
		dbPool:     deps.DBPool,
		auth:       newWorkOSAuth(deps.Config, deps.Users, deps.WorkOSClient),
		dispatcher: deps.Dispatcher,
		placement:  deps.Placement,
	}
	server := new(http.Server)
	server.Addr = deps.Config.ListenAddr
	server.Handler = http.MaxBytesHandler(apiServer.Router(), maxRequestBodyBytes)
	server.ReadHeaderTimeout = serverReadHeaderTimeout
	server.WriteTimeout = serverWriteTimeout
	server.IdleTimeout = serverIdleTimeout
	apiServer.server = server

	return apiServer
}

// Start listens for HTTP requests until the server stops or fails.
func (s *Server) Start() error {
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http serve failed: %w", err)
	}

	return nil
}

// Shutdown stops the HTTP server using the provided context.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http shutdown failed: %w", err)
	}

	return nil
}

// Router builds the full HTTP handler tree for the control plane.
// It installs request IDs, client IP capture, access logging, panic recovery,
// auth handling, and route-level rate limiting before registering the endpoint
// handlers.
func (s *Server) Router() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(AccessLogger())
	router.Use(middleware.Recoverer)

	userLimiter := httprate.Limit(userRateLimitRequests, userRateLimitWindow, httprate.WithKeyFuncs(KeyByIdentityKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			rateLimitExceeded(w, r)
		}),
	)
	// register work os session routes so authentication happens so v1 routes can use session access
	s.auth.registerRoutes(router)

	// Health check route.
	router.Get("/healthz", func(responseWriter http.ResponseWriter, r *http.Request) {
		if err := s.dbPool.Ping(r.Context()); err != nil { // check database connectivity
			responseWriter.WriteHeader(http.StatusServiceUnavailable)

			return
		}
		responseWriter.WriteHeader(http.StatusOK)
	})

	router.Route("/v1", func(apiRouter chi.Router) {
		apiRouter.Use(s.auth.sessionMiddleware())
		apiRouter.Use(userLimiter)

		apiRouter.Post("/bootstrap-defaults", s.handleBootstrapDefaults)

		s.registerV1Routes(apiRouter)
	})

	return router
}

// registerV1Routes registers the authenticated v1 API routes.
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

	router.Get("/workspaces/{workspaceId}/projects/{projectId}/workloads", s.handleListWorkloads)
	router.Post("/workspaces/{workspaceId}/projects/{projectId}/workloads", s.handleCreateWorkload)
}

// rateLimitExceeded writes the shared 429 response used by route limiters.
func rateLimitExceeded(w http.ResponseWriter, _ *http.Request) {
	Error(w, http.StatusTooManyRequests, "rate limit exceeded")
}

// RequireCallerUser resolves the authenticated principal into a stored user row.
// If the request is unauthenticated or the lookup fails, it writes the
// canonical API error and returns false.
func RequireCallerUser(responseWriter http.ResponseWriter, request *http.Request, users *tenant.UserService) (tenant.User, bool) {
	principal, ok := PrincipalFromContext(request.Context())
	if !ok {
		Error(responseWriter, http.StatusUnauthorized, "unauthorized")
		return tenant.User{}, false
	}

	user, err := users.GetUserByIdentityKey(request.Context(), principal.IdentityKey)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			Error(responseWriter, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			Error(responseWriter, http.StatusUnauthorized, "unauthorized")
		default:
			Error(responseWriter, http.StatusInternalServerError, "internal error")
		}

		return tenant.User{}, false
	}

	return user, true
}

// KeyByIdentityKey returns the rate-limit key for the current principal.
// Unauthenticated requests fall back to a shared key.
func KeyByIdentityKey(r *http.Request) (string, error) {
	p, ok := PrincipalFromContext(r.Context())
	if !ok || p.IdentityKey == "" {
		return "identity:unknown", nil
	}

	return "identity:" + p.IdentityKey, nil
}

// PrincipalFromContext reads the authenticated principal from context.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok
}
