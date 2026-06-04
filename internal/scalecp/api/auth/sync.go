package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
	"github.com/spacescale/core/internal/scalecp/api/respond"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

const (
	syncIdentityRateLimit       = 60
	syncIdentityRateLimitWindow = time.Minute
)

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

// NewSyncIdentityLimiter returns the per-identity limiter for trusted auth sync calls.
func NewSyncIdentityLimiter() *httprate.RateLimiter {
	return httprate.NewRateLimiter(
		syncIdentityRateLimit,
		syncIdentityRateLimitWindow,
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			respond.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
		}),
	)
}

// SyncUserHandler persists profile fields for a trusted auth user payload.
func SyncUserHandler(users *tenant.UserService, limiter *httprate.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req syncUserRequest
		if err := respond.ReadJSON(r, &req); err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(req.IdentityKey) == "" {
			respond.Error(w, http.StatusBadRequest, "invalid input")
			return
		}
		if limiter.RespondOnLimit(w, r, syncIdentityLimiterKey(req.IdentityKey)) {
			return
		}

		user, err := users.SyncAuthUser(r.Context(), tenant.SyncAuthUserParams{
			IdentityKey: req.IdentityKey,
			Email:       req.Email,
			Name:        req.Name,
			AvatarURL:   req.AvatarURL,
		})
		if err != nil {
			if errors.Is(err, tenant.ErrInvalidInput) {
				respond.Error(w, http.StatusBadRequest, "invalid input")
				return
			}
			respond.Error(w, http.StatusInternalServerError, "internal error")
			return
		}

		respond.JSON(w, http.StatusOK, syncUserResponse{
			ID:                  user.ID,
			OnboardingCompleted: user.OnboardingCompleted,
		})
	}
}

func syncIdentityLimiterKey(identityKey string) string {
	trimmed := strings.TrimSpace(identityKey)
	if trimmed == "" {
		return "internal-identity:unknown"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "internal-identity:" + hex.EncodeToString(sum[:])
}
