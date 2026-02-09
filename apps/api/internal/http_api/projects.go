// This file contains transport models and handler logic for project creation.
// It defines the request and response JSON shapes used by POST /v0/projects and
// translates HTTP concerns into service-layer calls.
// Responsibilities here include auth header extraction, request decoding,
// service error-to-status mapping, and response serialization.
// Keep business rules out of this file; those belong in service/project.go so
// HTTP code remains thin and focused on protocol behavior.

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

// handleCreateProject handles project creation for the authenticated user.
// It reads the GitHub identity from request headers, accepts an optional JSON
// payload, and delegates business rules to the service layer.
// Service outcomes are mapped to stable HTTP status codes before returning the
// created project payload and a Location header.
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
