// Package http_api
// This file provides authenticated workspace CRUD HTTP handlers.
package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

type updateWorkspaceRequest struct {
	Name string `json:"name"`
}

type workspaceResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type listWorkspacesResponse struct {
	Workspaces []workspaceResponse `json:"workspaces"`
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}
	var req createWorkspaceRequest
	if err := readJSON(r, &req); err != nil {
		switch {
		case errors.Is(err, errRequestBodyTooLarge):
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			writeErr(w, http.StatusBadRequest, "invalid json")
		}
		return
	}

	out, err := s.workspaces.CreateWorkspace(r.Context(), user.ID, tenant.CreateWorkspaceParams{Name: req.Name})
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		case errors.Is(err, tenant.ErrConflict):
			writeErr(w, http.StatusConflict, "conflict")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.Header().Set("Location", "/v1/workspaces/"+url.PathEscape(out.ID))
	writeJSON(w, http.StatusCreated, workspaceResponse{
		ID:        out.ID,
		Name:      out.Name,
		CreatedAt: out.CreatedAt.Format(time.RFC3339),
		UpdatedAt: out.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}
	workspaces, err := s.workspaces.ListWorkspaces(r.Context(), user.ID)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	out := make([]workspaceResponse, 0, len(workspaces))
	for _, ws := range workspaces {
		out = append(out, workspaceResponse{
			ID:        ws.ID,
			Name:      ws.Name,
			CreatedAt: ws.CreatedAt.Format(time.RFC3339),
			UpdatedAt: ws.UpdatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, listWorkspacesResponse{Workspaces: out})
}

func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
	if workspaceID == "" {
		writeErr(w, http.StatusBadRequest, "invalid input")
		return
	}
	ws, err := s.workspaces.GetWorkspace(r.Context(), user.ID, workspaceID)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, workspaceResponse{
		ID:        ws.ID,
		Name:      ws.Name,
		CreatedAt: ws.CreatedAt.Format(time.RFC3339),
		UpdatedAt: ws.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
	if workspaceID == "" {
		writeErr(w, http.StatusBadRequest, "invalid input")
		return
	}
	var req updateWorkspaceRequest
	if err := readJSON(r, &req); err != nil {
		switch {
		case errors.Is(err, errRequestBodyTooLarge):
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			writeErr(w, http.StatusBadRequest, "invalid json")
		}
		return
	}
	ws, err := s.workspaces.UpdateWorkspace(r.Context(), user.ID, workspaceID, tenant.UpdateWorkspaceParams{
		Name: req.Name,
	})
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		case errors.Is(err, tenant.ErrConflict):
			writeErr(w, http.StatusConflict, "conflict")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, workspaceResponse{
		ID:        ws.ID,
		Name:      ws.Name,
		CreatedAt: ws.CreatedAt.Format(time.RFC3339),
		UpdatedAt: ws.UpdatedAt.Format(time.RFC3339),
	})
}
func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
	if workspaceID == "" {
		writeErr(w, http.StatusBadRequest, "invalid input")
		return
	}
	if err := s.workspaces.DeleteWorkspace(r.Context(), user.ID, workspaceID); err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
