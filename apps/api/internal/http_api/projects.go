// This file contains transport models and handler logic for project creation.
// It defines the request and response JSON shapes used by POST /v0/projects and
// translates HTTP protocol concerns into service-layer calls.
//
// Responsibilities in this file:
// - Read authenticated principal from request context (set by auth middleware).
// - Decode and validate JSON request transport shape.
// - Map service-level errors to stable HTTP status codes and API messages.
// - Serialize successful responses with API-facing field names and formats.
//
// Responsibilities intentionally NOT in this file:
// - Business rules, validation defaults, slug generation, and persistence rules.
//   Those stay in service/project.go so HTTP handlers remain thin and predictable.

// Package http_api provides HTTP handlers for the API.
package http_api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/t0gun/spacescale/internal/service"
)

// createProjectRequest is the optional request payload for project creation.
// Both fields are optional; missing values are defaulted in the service layer.
// Keeping transport model optional allows clients to create projects with "{}".
type createProjectRequest struct {
	// Name is an optional project display name from the client.
	// When empty, the service may generate a fallback name.
	Name string `json:"name"`
	// Region is an optional project region override from the client.
	// When empty, the service applies its default region.
	Region string `json:"region"`
}

// createProjectResponse is the API payload returned on successful creation.
// Time fields are serialized as RFC3339 strings to keep JSON stable for clients.
type createProjectResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Region    string `json:"region"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// handleCreateProject creates a new project for the currently authenticated user.
//
// Request lifecycle in this handler:
// - Read AuthPrincipal from request context (set by auth middleware).
// - Decode optional JSON body into the transport request model.
// - Delegate project creation business logic to ProjectService.
// - Map service errors to stable API status codes/messages.
// - Return 201 Created with Location header and response payload.
//
// Authentication contract:
// - This handler trusts only context principal inserted by middleware.
// - If principal is missing, request is treated as unauthorized.
//
// JSON contract:
// - Empty request body is allowed and interpreted as default creation.
// - Malformed JSON returns 400 "invalid json".
// - Request bodies that exceed configured limits return 413.
//
// Error mapping contract:
// - service.ErrInvalidInput => 400 "invalid input"
// - service.ErrUnauthorized => 401 "unauthorized"
// - service.ErrConflict => 409 "conflict"
// - any other error => 500 "internal error"
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	// Ensure auth middleware provided a trusted principal.
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Decode optional request payload.
	// An empty body is valid and means "create with service defaults".
	var req createProjectRequest
	if err := readJSON(r, &req); err != nil {
		switch {
		case errors.Is(err, io.EOF):
			// Empty body is allowed and treated as "use defaults".
		case errors.Is(err, errRequestBodyTooLarge):
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		default:
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
	}

	user, err := s.services.Users.GetUserByIdentityKey(r.Context(), principal.IdentityKey)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
			return
		case errors.Is(err, service.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Delegate business behavior to service layer.
	project, err := s.services.Projects.CreateProject(r.Context(), user.ID, service.CreateProjectParams{
		Name:   req.Name,
		Region: req.Region,
	})

	// Convert service errors into stable HTTP API responses.
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
			return
		case errors.Is(err, service.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		case errors.Is(err, service.ErrConflict):
			writeErr(w, http.StatusConflict, "conflict")
			return
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Enrich request-scoped access logging metadata with the created project id.
	// Access logs are emitted by outer middleware after this handler returns, so
	// writing project id here allows the completion log to include project_id.
	if lc, ok := logContextFromContext(r.Context()); ok {
		lc.ProjectID = project.ID
	}

	// Return resource location and serialized payload.
	w.Header().Set("Location", "/v0/projects/"+url.PathEscape(project.ID))
	writeJSON(w, http.StatusCreated, createProjectResponse{
		ID:        project.ID,
		Name:      project.Name,
		Slug:      project.Slug,
		Region:    project.Region,
		CreatedAt: project.CreatedAt.Format(time.RFC3339),
		UpdatedAt: project.UpdatedAt.Format(time.RFC3339),
	})
}
