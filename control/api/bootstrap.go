// This file defines the authenticated bootstrap endpoint.
// It provisions first-time default resources (workspace and project) through an
// isolated BootstrapService workflow and remains idempotent for returning users.

package api

import (
	"net/http"
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
	if err := ReadAndValidateJSON(r, &req, true); err != nil {
		WriteJSONError(w, err)
		return
	}

	out, err := s.bootstrap.BootstrapDefaults(r.Context(), user.ID)
	if err != nil {
		WriteTenantError(w, err)
		return
	}

	JSON(w, http.StatusOK, bootstrapDefaultsResponse{
		Created:     out.Created,
		WorkspaceID: out.WorkspaceID,
		ProjectID:   out.ProjectID,
	})
}
