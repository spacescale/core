// This file defines user-focused HTTP handlers.
// It currently contains the trusted internal auth-sync endpoint used by the
// web auth callback to persist user profile records.
// The handler is transport-focused: decode request payload, delegate to service
// workflow, map service errors, and serialize a compact response.
// Security is enforced by middleware in server.go using a shared internal
// header secret so this endpoint is not exposed for public client traffic.

package http_api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/t0gun/spacescale/internal/service"
)

// syncAuthUserRequest is the request payload accepted by POST /v0/internal/auth-sync.
// IdentityKey is required; profile fields are optional.
type syncAuthUserRequest struct {
	IdentityKey string `json:"identityKey"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatarUrl"`
}

// syncAuthUserResponse returns the persisted user identity metadata consumed by
// the web auth callback.
type syncAuthUserResponse struct {
	ID                  string `json:"id"`
	OnboardingCompleted bool   `json:"onboardingCompleted"`
}

// handleSyncAuthUser persists profile fields for a trusted auth user payload.
//
// Error mapping contract:
// - service.ErrInvalidInput => 400 "invalid input"
// - any other error => 500 "internal error"
func (s *Server) handleSyncAuthUser(w http.ResponseWriter, r *http.Request) {
	var req syncAuthUserRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.IdentityKey) == "" {
		writeErr(w, http.StatusBadRequest, "invalid input")
		return
	}
	if s.internalIdentityLimiter.RespondOnLimit(w, r, internalIdentityLimiterKey(req.IdentityKey)) {
		return
	}

	user, err := s.services.Users.SyncAuthUser(r.Context(), service.SyncAuthUserParams{
		IdentityKey: req.IdentityKey,
		Email:       req.Email,
		Name:        req.Name,
		AvatarURL:   req.AvatarURL,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			writeErr(w, http.StatusBadRequest, "invalid input")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, syncAuthUserResponse{
		ID:                  user.ID,
		OnboardingCompleted: user.OnboardingCompleted,
	})
}

func internalIdentityLimiterKey(identityKey string) string {
	trimmed := strings.TrimSpace(identityKey)
	if trimmed == "" {
		return "internal-identity:unknown"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "internal-identity:" + hex.EncodeToString(sum[:])
}
