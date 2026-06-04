// Package auth owns HTTP authentication for the scalecp API. It verifies
// authenticated request identity, stores trusted principals in request context,
// and exposes small helpers for route wiring and request-scoped authorization.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spacescale/core/internal/scalecp/api/requestlog"
	"github.com/spacescale/core/internal/scalecp/api/respond"
	"github.com/spacescale/core/internal/shared/config"
)

const (
	authTokenLeeway    = 30 * time.Second
	subjectKeyPrefixV1 = "github:"
)

var (
	errMissingAuthorizationHeader   = errors.New("missing authorization header")
	errMalformedAuthorizationHeader = errors.New("malformed authorization header")
	errInvalidToken                 = errors.New("invalid token")
)

// Principal is the authenticated identity extracted from a verified token.
// Handlers should read this from context and never trust raw user headers.
type Principal struct {
	Subject     string
	IdentityKey string
	Email       string
	Name        string
	AvatarURL   string
}

type bffClaims struct {
	IdentityKey      string `json:"-"`
	IdentityKeyClaim string `json:"identity_key,omitempty"`
	Email            string `json:"email,omitempty"`
	Name             string `json:"name,omitempty"`
	AvatarURL        string `json:"avatar_url,omitempty"`
	jwt.RegisteredClaims
}

type principalContextKey struct{}

// Middleware verifies BFF bearer JWTs and attaches a trusted Principal to context.
func Middleware(cfg config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, err := parseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				requestlog.SetAuthFailure(r, authFailureReason(err))
				respond.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			claims, err := parseAndValidateClaims(rawToken, cfg)
			if err != nil {
				requestlog.SetAuthFailure(r, "invalid_token")
				respond.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			principal := Principal{
				Subject:     claims.Subject,
				IdentityKey: claims.IdentityKey,
				Email:       claims.Email,
				Name:        claims.Name,
				AvatarURL:   claims.AvatarURL,
			}
			ctx := withPrincipal(r.Context(), principal)
			if lc, ok := requestlog.MetadataFromContext(ctx); ok {
				lc.UserID = hashedUserIDForLogs(
					principal.IdentityKey,
					principal.Subject,
					cfg.JWTSecret,
				)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InternalMiddleware protects trusted internal endpoints with a shared secret header.
func InternalMiddleware(expectedSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := strings.TrimSpace(r.Header.Get("X-Internal-Auth"))
			if provided == "" {
				respond.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if subtle.ConstantTimeCompare([]byte(expectedSecret), []byte(provided)) != 1 {
				respond.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// KeyByIdentityKey returns a per-identity request rate-limit key.
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

func withPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

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
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithLeeway(authTokenLeeway),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return nil, errInvalidToken
	}

	sub := strings.TrimSpace(claims.Subject)
	if sub == "" {
		return nil, errInvalidToken
	}

	identityKeyFromClaim := strings.TrimSpace(claims.IdentityKeyClaim)
	if identityKeyFromClaim != "" {
		claims.IdentityKey = identityKeyFromClaim
		return claims, nil
	}

	identityKey, ok := identityKeyFromSubject(sub)
	if !ok {
		return nil, errInvalidToken
	}
	claims.IdentityKey = identityKey
	return claims, nil
}

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

func hashedUserIDForLogs(identityKey, subject, secret string) string {
	source := strings.TrimSpace(identityKey)
	if source == "" {
		source = strings.TrimSpace(subject)
	}
	if source == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(source))
	return "uidh:" + hex.EncodeToString(mac.Sum(nil))
}

func parseBearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) == 0 {
		return "", errMissingAuthorizationHeader
	}
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errMalformedAuthorizationHeader
	}
	if strings.TrimSpace(parts[1]) == "" {
		return "", errMalformedAuthorizationHeader
	}
	return parts[1], nil
}
