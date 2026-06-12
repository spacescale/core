// Package api keeps the browser-based WorkOS login flow in the same package as
// the HTTP server that uses it.
//
// The auth flow here is intentionally separated from the rest of the routing
// code so a new reader can find the login, callback, cookie, and session
// refresh logic in one place without scanning the whole server file.
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
	"github.com/spacescale/core/control/tenant"
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

// newWorkOSAuth builds the WorkOS helper from workload config and the user service.
//
	// Normal runtime uses cfg.WorkOS to construct the WorkOS client.
	// Tests can inject a preconfigured client (for example one pointed at an
	// httptest server) so the full login/callback/logout flow can be exercised
	// deterministically without talking to the real WorkOS API.
	//
	// If WorkOS is not configured, the helper still exists but its client stays
	// nil so the router can skip auth routes and treat API requests as
	// unauthenticated.
func newWorkOSAuth(cfg config.Control, users *tenant.UserService, client *workos.Client) *workOSAuth {
	if client == nil && cfg.WorkOS.APIKey != "" {
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
	router.Get("/auth/logout", a.handleLogout)
}

// sessionMiddleware validates the long-lived WorkOS session cookie on /v1
// requests and injects the authenticated principal into request context.
//
// This is the main request-time auth path:
// 1. Read the sealed WorkOS session cookie we set during callback.
// 2. Ask the WorkOS SDK to authenticate the sealed session locally.
// 3. If the access token inside the sealed cookie is still valid, attach the
//    authenticated principal to request context and continue.
// 4. If the access token is expired but refreshable, refresh through WorkOS,
//    reseal the session cookie, attach the refreshed principal, and continue.
// 5. If anything fails, clear the local cookie and return unauthorized.
//
// The API trusts WorkOS for human-session lifecycle and only maps the resulting
// authenticated user into Spacescale's local authorization model.
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
// This route does not authenticate the user itself. Its only job is to start
// the provider-owned login flow safely.
//
// Flow:
// 1. Generate a random state token for CSRF protection.
// 2. Store that state in a short-lived HTTP-only cookie scoped to /auth.
// 3. Build the WorkOS authorization URL.
// 4. Redirect the browser to WorkOS AuthKit.
//
// Later, WorkOS sends the browser back to /auth/callback with the code and the
// same state value so we can verify the redirect is one we initiated.
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
// Flow:
// 1. Validate the callback query parameters from WorkOS.
// 2. Compare the returned state against the short-lived cookie from /auth/login.
// 3. Exchange the authorization code with WorkOS for access/refresh tokens.
// 4. Upsert a local Spacescale user row keyed by `workos:<user_id>` so our own
//    authorization layer can reason about ownership in Postgres.
// 5. Seal the WorkOS session into one HTTP-only cookie.
// 6. Redirect the browser back to the configured post-login URL.
//
// The sealed cookie is the only browser credential we keep locally. We do not
// mint our own human JWT here; we trust WorkOS for login and session lifecycle.
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
		Name:        workOSDisplayName(authResponse.User),
		AvatarURL:   workOSProfilePictureURL(authResponse.User),
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

// handleLogout starts provider-backed logout for the current browser session.
//
// The goal here is to log the user out from both places that matter:
// 1. Locally, by clearing the sealed `spacescale_session` cookie.
// 2. Remotely, by sending the browser through WorkOS's session logout URL when
//    we can still recover a valid WorkOS session ID from the sealed cookie.
//
// We intentionally make logout best-effort. Even if the local cookie is missing
// or malformed, the handler still clears the cookie shape and redirects the user
// to the configured post-logout URL so the browser is left in a logged-out
// state from Spacescale's perspective.
func (a *workOSAuth) handleLogout(w http.ResponseWriter, r *http.Request) {
	if a.client == nil {
		writeAuthUnavailable(w)
		return
	}

	logoutURL := a.config.WorkOS.LogoutRedirectURI
	if sessionCookie, err := r.Cookie(a.config.WorkOS.CookieName); err == nil {
		sealedSession := strings.TrimSpace(sessionCookie.Value)
		if sealedSession != "" {
			session := workos.NewSession(a.client, sealedSession, a.config.WorkOS.CookiePassword)
			if workosLogoutURL, err := session.GetLogoutURL(r.Context(), a.config.WorkOS.LogoutRedirectURI); err == nil && workosLogoutURL != "" {
				logoutURL = workosLogoutURL
			}
		}
	}

	a.setCookie(w, a.config.WorkOS.CookieName, "", "/", -1)
	http.Redirect(w, r, logoutURL, http.StatusSeeOther)
}

// rejectSession clears the session cookie and returns the shared unauthorized
// response used for invalid or missing WorkOS sessions.
//
// We aggressively clear the cookie on auth failures so the browser does not
// keep retrying a broken sealed session on every /v1 request.
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
