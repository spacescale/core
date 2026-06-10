// This file defines the authenticated bootstrap endpoint.
// It provisions first-time default resources (workspace and project) through an
// isolated BootstrapService workflow and remains idempotent for returning users.

package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/spacescale/core/control/service/tenant"
)

// bootstrapDefaultsResponse is the response shape for default bootstrap calls.
type bootstrapDefaultsResponse struct {
	Created     bool   `json:"created"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
}

// handleBootstrapDefaults creates default workspace/project for first-time users.
// Returning users receive a no-op response with created=false.
func (s *Server) handleBootstrapDefaults(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireCallerUser(w, r, s.users)
	if !ok {
		return
	}

	// Empty body is allowed, but malformed JSON should still fail.
	var req struct{}
	if err := ReadJSON(r, &req); err != nil {
		switch {
		case errors.Is(err, io.EOF):
			// empty body is valid
		case errors.Is(err, ErrRequestBodyTooLarge):
			Error(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		default:
			Error(w, http.StatusBadRequest, "invalid json")
			return
		}
	}

	out, err := s.bootstrap.BootstrapDefaults(r.Context(), user.ID)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			Error(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			Error(w, http.StatusUnauthorized, "unauthorized")
		case errors.Is(err, tenant.ErrConflict):
			Error(w, http.StatusConflict, "conflict")
		default:
			Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	JSON(w, http.StatusOK, bootstrapDefaultsResponse{
		Created:     out.Created,
		WorkspaceID: out.WorkspaceID,
		ProjectID:   out.ProjectID,
	})
}
