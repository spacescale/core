// This file provides bearer-token authentication middleware for BFF-to-API calls.
// It verifies signed JWT access tokens, validates required issuer/audience claims,
// and stores the authenticated principal in request context for handlers to use.
// Keep token parsing and validation centralized here so endpoint handlers remain
// focused on transport and business behavior instead of auth plumbing.

package http_api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// authTokenLeeway gives a small clock-skew buffer when validating time-based
	// JWT claims (for example exp, nbf, iat). Distributed systems can have slightly
	// different clocks; a short leeway reduces false negatives without meaningfully
	// reducing token safety.
	authTokenLeeway = 30 * time.Second

	// githubSubjectPrefix is the canonical prefix used in sub claim values.
	// The API treats sub as the single source of identity and derives github id
	// from this prefix-based format: github:<id>.
	githubSubjectPrefix = "github:"
)

var (
	// errMissingAuthorizationHeader indicates a request did not provide Authorization.
	errMissingAuthorizationHeader = errors.New("missing authorization header")
	// errMalformedAuthorizationHeader indicates Authorization is not "Bearer <token>".
	errMalformedAuthorizationHeader = errors.New("malformed authorization header")
	// errInvalidToken indicates token signature/claims failed verification.
	errInvalidToken = errors.New("invalid token")
)

// AuthConfig defines runtime settings used to verify incoming BFF-issued JWTs.
// This is intentionally small and focused on verification requirements.
type AuthConfig struct {
	// JWTSecret is the HMAC secret used to verify token signatures.
	// If this does not match the signer secret used by the BFF, every token fails.
	JWTSecret string
	// Issuer is the expected "iss" claim value.
	// Keep this value aligned with the BFF token issuer configuration.
	Issuer string
	// Audience is the required "aud" claim that scopes who the token is meant for.
	// This prevents accepting tokens minted for a different service.
	Audience string
}

// AuthPrincipal is the authenticated identity extracted from a verified BFF JWT.
// Handlers should read this from context and never trust raw user headers.
type AuthPrincipal struct {
	Subject   string // Subject is the canonical identity "sub" claim, usually stable per user.
	GithubID  string
	Email     string
	Name      string
	AvatarURL string
}

// bffClaims models JWT claims expected from the Next.js BFF token.
// RegisteredClaims provides iss, aud, sub, exp, iat validation support.
type bffClaims struct {
	// GithubID is derived from Subject after successful token verification.
	// It is not decoded directly from token payload; identity source of truth is sub.
	GithubID  string `json:"-"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	// RegisteredClaims carries standard JWT claims:
	// - iss: issuer
	// - sub: subject/user identity
	// - aud: audience
	// - exp/iat/nbf: time-based validity fields
	// Embedding this enables jwt/v5 validation helpers for standard fields.
	jwt.RegisteredClaims
}

// principalContextKey is an unexported, zero-sized type used as the context key.
// Using a private struct type avoids collisions with keys from other packages.
type principalContextKey struct{}

// Validate verifies that AuthConfig has all required values before middleware
// is used to authenticate real requests.
//
// Why this check is done at startup:
// - Missing configuration would cause every request to fail authentication.
// - Failing fast makes deployment/config issues obvious and easier to debug.
//
// What this validates:
// - JWTSecret must be non-empty after trimming whitespace.
// - Issuer must be non-empty after trimming whitespace.
// - Audience must be non-empty after trimming whitespace.
//
// Return behavior:
// - Returns a descriptive error for the first missing required value.
// - Returns nil only when all required fields are present.
func (c AuthConfig) Validate() error {
	// JWT signature verification cannot run without a non-empty secret.
	if strings.TrimSpace(c.JWTSecret) == "" {
		return errors.New("auth config JWTSecret is required")
	}

	// Issuer matching is required to ensure the token came from our trusted BFF.
	if strings.TrimSpace(c.Issuer) == "" {
		return errors.New("auth config Issuer is required")
	}

	// Audience matching is required to ensure the token is intended for this API.
	if strings.TrimSpace(c.Audience) == "" {
		return errors.New("auth config Audience is required")
	}
	return nil
}

// authMiddleware creates a standard net/http middleware that performs bearer
// token authentication for each incoming request and enriches request context
// with a trusted AuthPrincipal for downstream handlers.
//
// End-to-end request flow:
// 1. Read Authorization header and extract a bearer token string.
// 2. Parse and validate the token signature and claims.
// 3. Build an AuthPrincipal from verified claims.
// 4. Attach principal to request context and call the next handler.
//
// Security behavior:
//   - Any token extraction/validation failure returns HTTP 401 Unauthorized.
//   - Error response body is intentionally generic ("unauthorized") so clients
//     are not given implementation details about why auth failed.
//
// Design intent:
//   - Keep authentication mechanics centralized here so endpoint handlers remain
//     focused on request/response handling and business logic.
func authMiddleware(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Expected format: "Bearer <jwt>".
			rawToken, err := parseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				// Always return a generic auth error to avoid leaking parse details.
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			claims, err := parseAndValidateClaims(rawToken, cfg)
			if err != nil {
				// Again, keep failure response intentionally generic.
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			// Handlers consume this trusted object from context.
			principal := AuthPrincipal{
				Subject:   claims.Subject,
				GithubID:  claims.GithubID,
				Email:     claims.Email,
				Name:      claims.Name,
				AvatarURL: claims.AvatarURL,
			}

			next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
		})
	}
}

// parseAndValidateClaims parses a JWT string into bffClaims and applies all
// verification rules required by this API before claims are trusted.
//
// Expected verification responsibilities of this helper:
// - Validate token signature against configured secret and algorithm.
// - Enforce standard claim requirements (issuer, audience, time validity).
// - Ensure required identity claims exist before principal construction.
// - Derive github id from canonical subject format: github:<id>.
// Return behavior:
// - On success, returns fully validated claims ready for middleware use.
// - On failure, returns an auth-safe error indicating token is invalid.
func parseAndValidateClaims(tokenString string, cfg AuthConfig) (*bffClaims, error) {
	claims := &bffClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errInvalidToken
			}
			return []byte(cfg.JWTSecret), nil
		},
		// Accept only HS256 to prevent algorithm confusion attacks.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		// Require the expected token issuer ("iss") from our trusted BFF.
		jwt.WithIssuer(cfg.Issuer),
		// Require the expected audience ("aud") for this API.
		jwt.WithAudience(cfg.Audience),
		// Allow a small clock-skew tolerance for time-based claim checks.
		jwt.WithLeeway(authTokenLeeway),
		jwt.WithExpirationRequired(), // prevents token that omits  expiration to pass validation
	)
	if err != nil || !token.Valid {
		return nil, errInvalidToken
	}

	sub := strings.TrimSpace(claims.Subject)
	githubID, ok := githubIDFromSubject(sub)
	if !ok {
		return nil, errInvalidToken
	}

	// Normalize and persist derived identity fields used by downstream handlers.
	claims.Subject = githubSubjectPrefix + githubID
	claims.GithubID = githubID
	return claims, nil
}

// githubIDFromSubject extracts GitHub identity from the canonical sub claim.
// Expected format is: github:<id>
// Returns false when subject is empty, has a different prefix, or has no id.
func githubIDFromSubject(subject string) (string, bool) {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, githubSubjectPrefix) {
		return "", false
	}

	githubID := strings.TrimSpace(strings.TrimPrefix(subject, githubSubjectPrefix))
	if githubID == "" {
		return "", false
	}
	return githubID, true
}

// withPrincipal returns a new context that carries the authenticated principal
// produced by auth middleware after successful token verification.
//
// Why this helper exists:
// - It centralizes how principal data is written to context.
// - It ensures the package-specific context key type is used consistently.
//
// Usage contract:
// - Call this only with trusted principal data derived from validated JWT claims.
// - The returned context must replace the previous request context (it is immutable).
func withPrincipal(ctx context.Context, p AuthPrincipal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// principalFromContext reads AuthPrincipal from context and reports whether a
// valid principal value was present under the expected key/type.
//
// Typical handler usage:
// - Call this at the start of an authenticated endpoint.
// - If ok is false, treat the request as unauthorized.
// - If ok is true, use the returned principal as the caller identity.
//
// Safety note:
//   - The type assertion protects handlers from panics if context is missing the
//     value or contains an unexpected type.
func principalFromContext(ctx context.Context) (AuthPrincipal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(AuthPrincipal)
	return p, ok
}

// parseBearerToken validates and extracts the JWT from an HTTP Authorization
// header expected in bearer format.
//
// Accepted header shape:
// - "Bearer <token>"
// - "bearer <token>" (scheme is case-insensitive by RFC behavior)
//
// Rejected as malformed:
// - Missing scheme or token
// - Extra space-separated parts (for example: "Bearer a b")
// - Any scheme other than Bearer
//
// Return behavior:
// - Returns the raw token string when format is valid.
// - Returns errMissingAuthorizationHeader when header is empty/whitespace.
// - Returns errMalformedAuthorizationHeader for all other format failures.
func parseBearerToken(header string) (string, error) {
	// strings.Fields splits on any whitespace and drops empty chunks, which
	// makes the parser robust to repeated spaces/tabs in the header value.
	parts := strings.Fields(header)

	// No parts means header was missing or blank.
	if len(parts) == 0 {
		return "", errMissingAuthorizationHeader
	}

	// Must be exactly two parts: "<scheme> <token>", and scheme must be Bearer.
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errMalformedAuthorizationHeader
	}

	// Guard against "Bearer    " or other empty-token edge cases.
	if strings.TrimSpace(parts[1]) == "" {
		return "", errMalformedAuthorizationHeader
	}

	// Token format is valid; return raw token for JWT verification stage.
	return parts[1], nil
}
