// Package http_api provides HTTP handlers for the API.
// Package http_api provides HTTP handlers for the API.
package http_api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/t0gun/spacescale/internal/service"
)

// createProjectRequest is the optional payload for project creation.
type createProjectRequest struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

// createProjectResponse is the project payload returned to clients.
type createProjectResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Region    string `json:"region"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// handleCreateProject creates a project for the current user.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	githubID := strings.TrimSpace(r.Header.Get("X-User-Github-ID"))
	if githubID == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createProjectRequest
	if err := readJSON(r, &req); err != nil {
		if errors.Is(err, io.EOF) {
			// empty body is allowed
		} else {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	project, err := s.svc.CreateProject(r.Context(), githubID, service.CreateProjectParams{
		Name:   req.Name,
		Region: req.Region,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			writeErr(w, http.StatusBadRequest, "invalid input")
			return
		}
		if errors.Is(err, service.ErrConflict) {
			writeErr(w, http.StatusConflict, "conflict")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Location", "/v0/projects/"+project.ID)
	writeJSON(w, http.StatusCreated, createProjectResponse{
		ID:        project.ID,
		Name:      project.Name,
		Slug:      project.Slug,
		Region:    project.Region,
		CreatedAt: project.CreatedAt.Format(time.RFC3339),
		UpdatedAt: project.UpdatedAt.Format(time.RFC3339),
	})
}
