// Package api configures and runs the primary HTTP control plane server.
// It sets up route routing, rate limiting, and middleware chains like CORS and recoverers.
// It binds core multi-tenant business services to their corresponding HTTP handlers.
// It also initializes session authentication using work OS Client Libary.
// The package keeps lifecycle operations clean, structured, and easy to orchestrate.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/control/fabric"
	"github.com/spacescale/core/control/service"
	"github.com/spacescale/core/control/service/tenant"
	"github.com/spacescale/core/shared/config"
	"github.com/workos/workos-go/v9"
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
	dbPool       *pgxpool.Pool
	config       config.Control
	server       *http.Server
	workosClient *workos.Client

	// fabric dependencies
	dispatcher *fabric.Dispatcher
}

// Principal is the authenticated identity attached to the request context.
// It is the single app-level auth shape used by handlers.
type Principal struct {
	Subject     string
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
}

type principalContextKey struct{}

const (
	maxRequestBodyBytes     = 1 << 20
	serverReadHeaderTimeout = 5 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 120 * time.Second
	userRateLimitRequests   = 200
	userRateLimitWindow     = time.Minute
	workOSStateCookieName   = "spacescale_workos_state"
	workOSStateTTL          = 10 * time.Minute
)

// ServerDeps groups dependencies required to construct the API server.
// It keeps startup and test wiring concise while making required inputs
// explicit at one call site.
type ServerDeps struct {
	Services *service.Services
	DBPool   *pgxpool.Pool
	Config   config.Control

	// fabric dependencies
	Dispatcher *fabric.Dispatcher
}

// NewServer constructs a control API server from service dependencies.
func NewServer(deps ServerDeps) *Server {
	var workosClient *workos.Client
	if deps.Config.WorkOS.APIKey != "" {
		workosClient = workos.NewClient(deps.Config.WorkOS.APIKey, workos.WithClientID(deps.Config.WorkOS.ClientID))
	}

	tenantServices := deps.Services.Tenant
	apiServer := &Server{
		users:      tenantServices.Users,
		projects:   tenantServices.Projects,
		workspaces: tenantServices.Workspaces,
		bootstrap:  tenantServices.Bootstrap,
		apps:       tenantServices.Apps,
		dbPool:     deps.DBPool,
		config:     deps.Config,

		// fabric dependencies
		dispatcher:   deps.Dispatcher,
		server:       nil,
		workosClient: workosClient,
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

// Router initializes and returns the complete HTTP handler for the API.
// It sets up core middlewares, rate limiters, and maps handlers to all API endpoints.
// Callers should use the returned handler to serve incoming client requests.
func (s *Server) Router() http.Handler {
	router := chi.NewRouter()

	// Base middleware stack.
	router.Use(middleware.RequestID)
	router.Use(middleware.ClientIPFromRemoteAddr)
	router.Use(Middleware())
	router.Use(Recoverer())

	userLimiter := httprate.Limit(userRateLimitRequests, userRateLimitWindow, httprate.WithKeyFuncs(KeyByIdentityKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			rateLimitExceeded(w, r)
		}),
	)

	if s.workosClient != nil {
		router.Get("/auth/login", s.handleWorkOSLogin)
		router.Get("/auth/callback", s.handleWorkOSCallback)
	}

	// Health check route.
	router.Get("/healthz", func(responseWriter http.ResponseWriter, r *http.Request) {
		if err := s.dbPool.Ping(r.Context()); err != nil { // check database connectivity
			responseWriter.WriteHeader(http.StatusServiceUnavailable)

			return
		}
		responseWriter.WriteHeader(http.StatusOK)
	})

	router.Route("/v1", func(apiRouter chi.Router) {
		apiRouter.Use(s.workOSSessionMiddleware())
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

// RequireCallerUser resolves the authenticated principal into a persisted user row.
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

// KeyByIdentityKey returns the rate-limit key for the authenticated caller.
func KeyByIdentityKey(r *http.Request) (string, error) {
	p, ok := PrincipalFromContext(r.Context())
	if !ok || p.IdentityKey == "" {
		return "identity:unknown", nil
	}

	return "identity:" + p.IdentityKey, nil
}

// PrincipalFromContext reads Principal from context.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok
}

func (s *Server) handleWorkOSLogin(w http.ResponseWriter, r *http.Request) {
	if s.workosClient == nil {
		Error(w, http.StatusServiceUnavailable, "auth unavailable")
		return
	}

	state, err := generateWorkOSState()
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     workOSStateCookieName,
		Value:    state,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   workOSCookieSecure(s.config.Environment),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(workOSStateTTL.Seconds()),
	})
	provider := workos.UserManagementAuthenticationProviderAuthkit
	authorizationURL := s.workosClient.UserManagement().GetAuthorizationURL(
		&workos.UserManagementGetAuthorizationURLParams{
			RedirectURI: s.config.WorkOS.RedirectURI,
			State:       nonEmptyStringPtr(state),
			Provider:    &provider,
		},
	)

	http.Redirect(w, r, authorizationURL, http.StatusFound)
}

func (s *Server) handleWorkOSCallback(w http.ResponseWriter, r *http.Request) {
	if s.workosClient == nil {
		Error(w, http.StatusServiceUnavailable, "auth unavailable")
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		Error(w, http.StatusBadRequest, "missing code")
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		Error(w, http.StatusBadRequest, "missing state")
		return
	}

	stateCookie, err := r.Cookie(workOSStateCookieName)
	if err != nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     workOSStateCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   workOSCookieSecure(s.config.Environment),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	authResponse, err := s.workosClient.UserManagement().AuthenticateWithCode(
		r.Context(),
		&workos.UserManagementAuthenticateWithCodeParams{
			Code:      code,
			IPAddress: nonEmptyStringPtr(clientIP(r.RemoteAddr)),
			UserAgent: nonEmptyStringPtr(r.UserAgent()),
		},
	)
	if err != nil {
		Error(w, http.StatusUnauthorized, "authentication failed")
		return
	}
	if authResponse == nil || authResponse.User == nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	identityKey := "workos:" + authResponse.User.ID
	if _, err := s.users.SyncAuthUser(r.Context(), tenant.SyncAuthUserParams{
		IdentityKey: identityKey,
		Email:       authResponse.User.Email,
		Name:        "",
		AvatarURL:   "",
	}); err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	sealedSession, err := workos.SealSessionFromAuthResponse(
		authResponse.AccessToken,
		authResponse.RefreshToken,
		authResponse.User,
		authResponse.Impersonator,
		s.config.WorkOS.CookiePassword,
	)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.config.WorkOS.CookieName,
		Value:    sealedSession,
		Path:     "/",
		HttpOnly: true,
		Secure:   workOSCookieSecure(s.config.Environment),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, s.config.WorkOS.PostLoginRedirectURI, http.StatusSeeOther)
}

func (s *Server) workOSSessionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.workosClient == nil {
				SetAuthFailure(r, "workos_unconfigured")
				Error(w, http.StatusServiceUnavailable, "auth unavailable")
				return
			}

			sessionCookie, err := r.Cookie(s.config.WorkOS.CookieName)
			if err != nil {
				SetAuthFailure(r, "missing_session_cookie")
				Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			session := workos.NewSession(s.workosClient, sessionCookie.Value, s.config.WorkOS.CookiePassword)
			authn, err := session.Authenticate()
			if err != nil {
				s.clearWorkOSSessionCookie(w)
				SetAuthFailure(r, "invalid_session")
				Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if authn != nil && authn.Authenticated && authn.User != nil {
				principal := workOSPrincipalFromUser(authn.User)
				ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
				if lc, ok := MetadataFromContext(ctx); ok {
					lc.UserID = principal.IdentityKey
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if authn != nil && authn.NeedsRefresh {
				refreshed, err := session.Refresh(r.Context())
				if err == nil && refreshed != nil && refreshed.Authenticated && refreshed.Session != nil && refreshed.Session.User != nil {
					s.setWorkOSSessionCookie(w, refreshed.SealedSession)

					principal := workOSPrincipalFromUser(refreshed.Session.User)
					ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
					if lc, ok := MetadataFromContext(ctx); ok {
						lc.UserID = principal.IdentityKey
					}
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				s.clearWorkOSSessionCookie(w)
				SetAuthFailure(r, "session_refresh_failed")
				Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			s.clearWorkOSSessionCookie(w)
			SetAuthFailure(r, "invalid_session")
			Error(w, http.StatusUnauthorized, "unauthorized")
		})
	}
}

func (s *Server) setWorkOSSessionCookie(w http.ResponseWriter, sealedSession string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.config.WorkOS.CookieName,
		Value:    sealedSession,
		Path:     "/",
		HttpOnly: true,
		Secure:   workOSCookieSecure(s.config.Environment),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearWorkOSSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.config.WorkOS.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   workOSCookieSecure(s.config.Environment),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func workOSPrincipalFromUser(user *workos.User) Principal {
	identityKey := "workos:" + user.ID
	return Principal{
		Subject:     identityKey,
		IdentityKey: identityKey,
		Email:       strings.TrimSpace(user.Email),
		Name:        workOSDisplayName(user),
		AvatarURL:   workOSProfilePictureURL(user),
	}
}

func workOSDisplayName(user *workos.User) string {
	parts := make([]string, 0, 2)
	if user.FirstName != nil {
		first := strings.TrimSpace(*user.FirstName)
		if first != "" {
			parts = append(parts, first)
		}
	}
	if user.LastName != nil {
		last := strings.TrimSpace(*user.LastName)
		if last != "" {
			parts = append(parts, last)
		}
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}

func workOSProfilePictureURL(user *workos.User) string {
	if user.ProfilePictureURL == nil {
		return ""
	}

	return strings.TrimSpace(*user.ProfilePictureURL)
}

func generateWorkOSState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func nonEmptyStringPtr(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func workOSCookieSecure(environment string) bool {
	return strings.TrimSpace(environment) == "production"
}
