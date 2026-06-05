package api

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

	"github.com/go-chi/httprate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spacescale/core/scalecp/service/tenant"
	"github.com/spacescale/core/shared/config"
)

const (
	authTokenLeeway             = 30 * time.Second
	subjectKeyPrefixV1          = "github:"
	syncIdentityRateLimit       = 60
	syncIdentityRateLimitWindow = time.Minute
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
	jwt.RegisteredClaims

	IdentityKey      string `json:"-"`
	IdentityKeyClaim string `json:"identity_key,omitempty"`
	Email            string `json:"email,omitempty"`
	Name             string `json:"name,omitempty"`
	AvatarURL        string `json:"avatar_url,omitempty"`
}

type syncUserRequest struct {
	IdentityKey string `json:"identityKey"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatarUrl"`
}

type syncUserResponse struct {
	ID                  string `json:"id"`
	OnboardingCompleted bool   `json:"onboardingCompleted"`
}

type principalContextKey struct{}

// AuthMiddleware verifies bearer JWTs and attaches a trusted Principal to context.
func AuthMiddleware(cfg config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			rawToken, err := parseBearerToken(request.Header.Get("Authorization"))
			if err != nil {
				SetAuthFailure(request, authFailureReason(err))
				Error(responseWriter, http.StatusUnauthorized, "unauthorized")

				return
			}

			claims, err := parseAndValidateClaims(rawToken, cfg)
			if err != nil {
				SetAuthFailure(request, "invalid_token")
				Error(responseWriter, http.StatusUnauthorized, "unauthorized")

				return
			}

			principal := Principal{
				Subject:     claims.Subject,
				IdentityKey: claims.IdentityKey,
				Email:       claims.Email,
				Name:        claims.Name,
				AvatarURL:   claims.AvatarURL,
			}
			ctx := withPrincipal(request.Context(), principal)
			if lc, ok := MetadataFromContext(ctx); ok {
				lc.UserID = hashedUserIDForLogs(principal.IdentityKey, principal.Subject, cfg.JWTSecret)
			}
			next.ServeHTTP(responseWriter, request.WithContext(ctx))
		})
	}
}

// InternalAuthMiddleware protects trusted internal endpoints with a shared secret header.
func InternalAuthMiddleware(expectedSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			provided := strings.TrimSpace(request.Header.Get("X-Internal-Auth"))
			if provided == "" {
				Error(responseWriter, http.StatusUnauthorized, "unauthorized")

				return
			}
			if subtle.ConstantTimeCompare([]byte(expectedSecret), []byte(provided)) != 1 {
				Error(responseWriter, http.StatusUnauthorized, "unauthorized")

				return
			}
			next.ServeHTTP(responseWriter, request)
		})
	}
}

// RequireCallerUser resolves the authenticated principal into a persisted user row.
func RequireCallerUser(responseWriter http.ResponseWriter, request *http.Request, users *tenant.UserService) (tenant.User, bool) {
	principal, ok := PrincipalFromContext(request.Context())
	if !ok {
		Error(responseWriter, http.StatusUnauthorized, "unauthorized")

		return emptyTenantUser(), false
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

		return emptyTenantUser(), false
	}

	return user, true
}

// NewSyncIdentityLimiter returns the per-identity limiter for trusted auth sync calls.
func NewSyncIdentityLimiter() *httprate.RateLimiter {
	return httprate.NewRateLimiter(
		syncIdentityRateLimit,
		syncIdentityRateLimitWindow,
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			Error(w, http.StatusTooManyRequests, "rate limit exceeded")
		}),
	)
}

// SyncUserHandler persists profile fields for a trusted auth user payload.
func SyncUserHandler(users *tenant.UserService, limiter *httprate.RateLimiter) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		var req syncUserRequest
		if err := ReadJSON(request, &req); err != nil {
			Error(responseWriter, http.StatusBadRequest, "invalid json")

			return
		}
		if strings.TrimSpace(req.IdentityKey) == "" {
			Error(responseWriter, http.StatusBadRequest, "invalid input")

			return
		}
		if limiter.RespondOnLimit(responseWriter, request, syncIdentityLimiterKey(req.IdentityKey)) {
			return
		}

		user, err := users.SyncAuthUser(request.Context(), tenant.SyncAuthUserParams{
			IdentityKey: req.IdentityKey,
			Email:       req.Email,
			Name:        req.Name,
			AvatarURL:   req.AvatarURL,
		})
		if err != nil {
			if errors.Is(err, tenant.ErrInvalidInput) {
				Error(responseWriter, http.StatusBadRequest, "invalid input")

				return
			}
			Error(responseWriter, http.StatusInternalServerError, "internal error")

			return
		}

		JSON(responseWriter, http.StatusOK, syncUserResponse{
			ID:                  user.ID,
			OnboardingCompleted: user.OnboardingCompleted,
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
	claims := &bffClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "",
			Subject:   "",
			Audience:  nil,
			ExpiresAt: nil,
			NotBefore: nil,
			IssuedAt:  nil,
			ID:        "",
		},
		IdentityKey:      "",
		IdentityKeyClaim: "",
		Email:            "",
		Name:             "",
		AvatarURL:        "",
	}
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

func emptyTenantUser() tenant.User {
	return tenant.User{
		ID:                  "",
		IdentityKey:         "",
		Email:               "",
		Name:                "",
		AvatarURL:           "",
		OnboardingCompleted: false,
		CreatedAt:           time.Time{},
		UpdatedAt:           time.Time{},
	}
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

func syncIdentityLimiterKey(identityKey string) string {
	trimmed := strings.TrimSpace(identityKey)
	if trimmed == "" {
		return "internal-identity:unknown"
	}
	sum := sha256.Sum256([]byte(trimmed))

	return "internal-identity:" + hex.EncodeToString(sum[:])
}
