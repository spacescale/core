// Package api keeps the WorkOS browser auth flow next to the HTTP server it
// serves.
//
// This file isolates the auth-specific pieces from the rest of the control
// API so the main server wiring stays easy to scan.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spacescale/core/control/service/tenant"
	"github.com/spacescale/core/shared/config"
	"github.com/workos/workos-go/v9"
)

const (
	workOSStateCookieName = "spacescale_workos_state"
	workOSStateTTL        = 10 * time.Minute
)

// workOSAuth owns the WorkOS client, cookie settings, and user sync plumbing
// for the browser login/session flow.
type workOSAuth struct {
	client *workos.Client
	config config.Control
	users  *tenant.UserService
}

// newWorkOSAuth builds the WorkOS helper from app config and the user service.
//
// If WorkOS is not configured, the helper still exists but its client stays nil
// so the router can skip auth routes and treat API requests as unauthenticated.
func newWorkOSAuth(cfg config.Control, users *tenant.UserService) *workOSAuth {
	var client *workos.Client
	if cfg.WorkOS.APIKey != "" {
		client = workos.NewClient(cfg.WorkOS.APIKey, workos.WithClientID(cfg.WorkOS.ClientID))
	}

	return &workOSAuth{client: client, config: cfg, users: users}
}

// registerRoutes mounts the WorkOS login and callback endpoints when auth is
// configured. If no WorkOS client exists, it does nothing.
func (a *workOSAuth) registerRoutes(router chi.Router) {
	if a.client == nil {
		return
	}

	router.Get("/auth/login", a.handleLogin)
	router.Get("/auth/callback", a.handleCallback)
}

// sessionMiddleware validates the long-lived WorkOS session cookie on /v1
// requests and injects the authenticated principal into request context.
func (a *workOSAuth) sessionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.client == nil {
				a.rejectSession(w)
				return
			}

			sessionCookie, err := r.Cookie(a.config.WorkOS.CookieName)
			if err != nil {
				a.rejectSession(w)
				return
			}

			session := workos.NewSession(a.client, sessionCookie.Value, a.config.WorkOS.CookiePassword)
			authn, err := session.Authenticate()
			if err != nil {
				a.rejectSession(w)
				return
			}

			if authn != nil && authn.Authenticated && authn.User != nil {
				a.serveWithPrincipal(next, w, r, principalFromWorkOSUser(authn.User))
				return
			}

			if authn != nil && authn.NeedsRefresh {
				refreshed, err := session.Refresh(r.Context())
				if err == nil && refreshed != nil && refreshed.Authenticated && refreshed.Session != nil && refreshed.Session.User != nil {
					a.setCookie(w, a.config.WorkOS.CookieName, refreshed.SealedSession, "/", 0)
					a.serveWithPrincipal(next, w, r, principalFromWorkOSUser(refreshed.Session.User))
					return
				}

				a.rejectSession(w)
				return
			}

			a.rejectSession(w)
		})
	}
}

// handleLogin starts the browser login flow.
//
// It generates a random CSRF state, stores it in a short-lived cookie, and
// redirects the browser to WorkOS's authorization URL.
func (a *workOSAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	if a.client == nil {
		writeAuthUnavailable(w)
		return
	}

	state, err := generateWorkOSState()
	if err != nil {
		writeInternalError(w)
		return
	}

	a.setCookie(w, workOSStateCookieName, state, "/auth", int(workOSStateTTL.Seconds()))
	authorizationURL := a.client.UserManagement().GetAuthorizationURL(
		&workos.UserManagementGetAuthorizationURLParams{
			RedirectURI: a.config.WorkOS.RedirectURI,
			State:       nonEmptyStringPtr(state),
			Provider:    new(workos.UserManagementAuthenticationProviderAuthkit),
		},
	)

	http.Redirect(w, r, authorizationURL, http.StatusFound)
}

// handleCallback completes the browser login flow after WorkOS redirects back.
//
// It checks the OAuth code and state, swaps the auth code for tokens, syncs the
// user locally, seals a session cookie, and redirects back into the app.
func (a *workOSAuth) handleCallback(w http.ResponseWriter, r *http.Request) {
	if a.client == nil {
		writeAuthUnavailable(w)
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
	if err != nil || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		writeUnauthorized(w)
		return
	}

	a.setCookie(w, workOSStateCookieName, "", "/auth", -1)

	authResponse, err := a.client.UserManagement().AuthenticateWithCode(
		r.Context(),
		&workos.UserManagementAuthenticateWithCodeParams{
			Code:      code,
			IPAddress: nonEmptyStringPtr(clientIP(r.RemoteAddr)),
			UserAgent: nonEmptyStringPtr(r.UserAgent()),
		},
	)
	if err != nil {
		writeUnauthorized(w)
		return
	}
	if authResponse == nil || authResponse.User == nil {
		writeInternalError(w)
		return
	}

	if _, err := a.users.SyncAuthUser(r.Context(), tenant.SyncAuthUserParams{
		IdentityKey: "workos:" + authResponse.User.ID,
		Email:       authResponse.User.Email,
		Name:        "",
		AvatarURL:   "",
	}); err != nil {
		writeInternalError(w)
		return
	}

	sealedSession, err := workos.SealSessionFromAuthResponse(
		authResponse.AccessToken,
		authResponse.RefreshToken,
		authResponse.User,
		authResponse.Impersonator,
		a.config.WorkOS.CookiePassword,
	)
	if err != nil {
		writeInternalError(w)
		return
	}

	a.setCookie(w, a.config.WorkOS.CookieName, sealedSession, "/", 0)
	http.Redirect(w, r, a.config.WorkOS.PostLoginRedirectURI, http.StatusSeeOther)
}

// rejectSession clears the session cookie and returns the shared unauthorized
// response used for invalid or missing WorkOS sessions.
func (a *workOSAuth) rejectSession(w http.ResponseWriter) {
	a.setCookie(w, a.config.WorkOS.CookieName, "", "/", -1)
	writeUnauthorized(w)
}

// setCookie writes the common cookie shape used by the WorkOS auth flow.
//
// The helper centralizes the HttpOnly/SameSite/Secure settings so login,
// callback, refresh, and rejection paths all behave consistently.
func (a *workOSAuth) setCookie(w http.ResponseWriter, name, value, path string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		Secure:   workOSCookieSecure(a.config.Environment),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// serveWithPrincipal attaches a Principal to the request context and continues
// the handler chain.
func (a *workOSAuth) serveWithPrincipal(next http.Handler, w http.ResponseWriter, r *http.Request, principal Principal) {
	ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// principalFromWorkOSUser converts the WorkOS user payload into the API's
// internal principal shape.
func principalFromWorkOSUser(user *workos.User) Principal {
	identityKey := "workos:" + user.ID
	return Principal{
		Subject:     identityKey,
		IdentityKey: identityKey,
		Email:       strings.TrimSpace(user.Email),
		Name:        workOSDisplayName(user),
		AvatarURL:   workOSProfilePictureURL(user),
	}
}

// workOSDisplayName joins first and last name when present and trims the
// result.
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

// workOSProfilePictureURL returns the trimmed profile image URL when WorkOS
// provides one.
func workOSProfilePictureURL(user *workos.User) string {
	if user.ProfilePictureURL == nil {
		return ""
	}

	return strings.TrimSpace(*user.ProfilePictureURL)
}

// generateWorkOSState creates a URL-safe random state token for CSRF
// protection during the login redirect.
func generateWorkOSState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// nonEmptyStringPtr returns nil for blank strings so WorkOS omits optional
// fields rather than sending empty values.
func nonEmptyStringPtr(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// workOSCookieSecure enables the Secure cookie flag only in production.
func workOSCookieSecure(environment string) bool {
	return strings.TrimSpace(environment) == "production"
}

// writeAuthUnavailable returns the standard response used when WorkOS is not
// configured for this process.
func writeAuthUnavailable(w http.ResponseWriter) {
	Error(w, http.StatusServiceUnavailable, "auth unavailable")
}

// writeUnauthorized returns the shared unauthorized API response.
func writeUnauthorized(w http.ResponseWriter) {
	Error(w, http.StatusUnauthorized, "unauthorized")
}

// writeInternalError returns the shared internal-error API response.
func writeInternalError(w http.ResponseWriter) {
	Error(w, http.StatusInternalServerError, "internal error")
}
