// This file provides bearer-token authentication middleware for BFF-to-API calls.
// It verifies signed JWT access tokens, validates required issuer/audience claims,
// and stores the authenticated principal in request context for handlers to use.
// Keep token parsing and validation centralized here so endpoint handlers remain
// focused on transport and business behavior instead of auth plumbing.

package http_api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/t0gun/spacescale/internal/config"
)

const (
	authTokenLeeway    = 30 * time.Second // small clock-skew buffer for JWT time-claim validation.
	subjectKeyPrefixV1 = "github:"        // legacy canonical subject prefix used to derive identity key.
)

var (
	errMissingAuthorizationHeader   = errors.New("missing authorization header")   // request did not provide Authorization header.
	errMalformedAuthorizationHeader = errors.New("malformed authorization header") // Authorization is not in "Bearer <token>" shape.
	errInvalidToken                 = errors.New("invalid token")                  // token signature or claims failed verification.
)

// AuthPrincipal is the authenticated identity extracted from a verified BFF JWT.
// Handlers should read this from context and never trust raw user headers.
type AuthPrincipal struct {
	Subject     string // Subject is the canonical identity "sub" claim, usually stable per user.
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
}

// bffClaims models JWT claims expected from the Next.js BFF token.
// RegisteredClaims provides iss, aud, sub, exp, iat validation support.
type bffClaims struct {
	// IdentityKey is derived from Subject after successful token verification.
	// It is not decoded directly from token payload; identity source of truth is sub.
	IdentityKey string `json:"-"`
	Email       string `json:"email,omitempty"`
	Name        string `json:"name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
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
func authMiddleware(cfg config.AuthConfig, logCfg config.LogPrivacyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Expected format: "Bearer <jwt>".
			rawToken, err := parseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				logAuthFailure(r, authFailureReason(err), logCfg)
				// Always return a generic auth error to avoid leaking parse details.
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			claims, err := parseAndValidateClaims(rawToken, cfg)
			if err != nil {
				logAuthFailure(r, "invalid_token", logCfg)
				// Again, keep failure response intentionally generic.
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			// Handlers consume this trusted object from context.
			principal := AuthPrincipal{
				Subject:     claims.Subject,
				IdentityKey: claims.IdentityKey,
				Email:       claims.Email,
				Name:        claims.Name,
				AvatarURL:   claims.AvatarURL,
			}
			ctx := withPrincipal(r.Context(), principal)
			if lc, ok := logContextFromContext(ctx); ok {
				lc.UserID = principal.Subject
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// authFailureReason converts internal auth parse errors into stable reason codes.
//
// Why we emit codes instead of raw errors:
// - Keeps logs queryable and consistent.
// - Avoids accidentally leaking sensitive token/header details.
func authFailureReason(err error) string {
	switch {
	case errors.Is(err, errMissingAuthorizationHeader):
		return "missing_authorization_header"
	case errors.Is(err, errMalformedAuthorizationHeader):
		return "malformed_authorization_header"
	default:
		return "invalid_token"
	}
}

// logAuthFailure emits a structured warning event for unauthorized request paths.
//
// Redaction/safety rules:
// - Do not log Authorization header values.
// - Do not log token text, cookies, or request bodies.
// - Emit only stable reason codes and request metadata.
// - Emit user-agent metadata using configured privacy policy.
func logAuthFailure(r *http.Request, reason string, logCfg config.LogPrivacyConfig) {
	attrs := []any{
		"event", "auth_failure",
		"request_id", middleware.GetReqID(r.Context()),
		"method", r.Method,
		"route", routePatternFromContext(r.Context()),
		"path", r.URL.Path,
		"status_code", http.StatusUnauthorized,
		"reason", reason,
		"client_ip", clientIP(r.RemoteAddr),
	}

	// Apply shared user-agent privacy behavior so auth failure logs stay aligned
	// with access and panic log output policy.
	if key, value, ok := userAgentLogAttr(r.UserAgent(), logCfg); ok {
		attrs = append(attrs, key, value)
	}

	slog.Warn("auth failure", attrs...)
}

// parseAndValidateClaims parses a JWT string into bffClaims and applies all
// verification rules required by this API before claims are trusted.
//
// Expected verification responsibilities of this helper:
// - Validate token signature against configured secret and algorithm.
// - Enforce standard claim requirements (issuer, audience, time validity).
// - Ensure required identity claims exist before principal construction.
// - Derive identity key from canonical subject format: github:<identity-key>.
// Return behavior:
// - On success, returns fully validated claims ready for middleware use.
// - On failure, returns an auth-safe error indicating token is invalid.
func parseAndValidateClaims(tokenString string, cfg config.AuthConfig) (*bffClaims, error) {
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
		jwt.WithExpirationRequired(), // Reject tokens that omit the exp claim.
	)
	if err != nil || !token.Valid {
		return nil, errInvalidToken
	}

	sub := strings.TrimSpace(claims.Subject)
	identityKey, ok := identityKeyFromSubject(sub)
	if !ok {
		return nil, errInvalidToken
	}

	// Store the extracted identity key for downstream handlers.
	// Keep the original Subject unchanged to preserve the validated JWT claim.
	claims.IdentityKey = identityKey
	return claims, nil
}

// identityKeyFromSubject extracts identity key from the canonical subclaim.
// Current expected format is: github:<identity-key>
// Returns false when subject is empty, has a different prefix, or has no id.
func identityKeyFromSubject(subject string) (string, bool) {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, subjectKeyPrefixV1) {
		return "", false
	}

	identityKey := strings.TrimSpace(strings.TrimPrefix(subject, subjectKeyPrefixV1))
	if identityKey == "" {
		return "", false
	}
	return identityKey, true
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
