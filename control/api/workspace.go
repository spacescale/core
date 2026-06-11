package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spacescale/core/control/service/tenant"
)

type createWorkspaceRequest struct {
	Name string `json:"name" validate:"required,notblank,max=255"`
}

type updateWorkspaceRequest struct {
	Name string `json:"name" validate:"required,notblank,max=255"`
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

func (s *Server) handleCreateWorkspace(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	var req createWorkspaceRequest
	if err := ReadJSON(request, &req); err != nil {
		switch {
		case errors.Is(err, ErrRequestBodyTooLarge):
			Error(responseWriter, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			Error(responseWriter, http.StatusBadRequest, "invalid json")
		}

		return
	}
	if err := ValidateStruct(req); err != nil {
		Error(responseWriter, http.StatusBadRequest, "invalid input")
		return
	}

	out, err := s.workspaces.CreateWorkspace(request.Context(), user.ID, tenant.CreateWorkspaceParams{Name: req.Name})
	if err != nil {
		writeTenantError(responseWriter, err)

		return
	}

	responseWriter.Header().Set("Location", "/v1/workspaces/"+url.PathEscape(out.ID))
	JSON(responseWriter, http.StatusCreated, workspaceResponse{
		ID:        out.ID,
		Name:      out.Name,
		CreatedAt: out.CreatedAt.Format(time.RFC3339),
		UpdatedAt: out.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleListWorkspaces(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaces, err := s.workspaces.ListWorkspaces(request.Context(), user.ID)
	if err != nil {
		writeTenantError(responseWriter, err)

		return
	}

	out := make([]workspaceResponse, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, workspaceResponse{
			ID:        workspace.ID,
			Name:      workspace.Name,
			CreatedAt: workspace.CreatedAt.Format(time.RFC3339),
			UpdatedAt: workspace.UpdatedAt.Format(time.RFC3339),
		})
	}
	JSON(responseWriter, http.StatusOK, listWorkspacesResponse{Workspaces: out})
}

func (s *Server) handleGetWorkspace(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, ok := workspaceIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	workspace, err := s.workspaces.GetWorkspace(request.Context(), user.ID, workspaceID)
	if err != nil {
		writeTenantError(responseWriter, err)

		return
	}

	JSON(responseWriter, http.StatusOK, workspaceResponse{
		ID:        workspace.ID,
		Name:      workspace.Name,
		CreatedAt: workspace.CreatedAt.Format(time.RFC3339),
		UpdatedAt: workspace.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleUpdateWorkspace(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, ok := workspaceIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	var req updateWorkspaceRequest
	if err := ReadJSON(request, &req); err != nil {
		switch {
		case errors.Is(err, ErrRequestBodyTooLarge):
			Error(responseWriter, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			Error(responseWriter, http.StatusBadRequest, "invalid json")
		}

		return
	}
	if err := ValidateStruct(req); err != nil {
		Error(responseWriter, http.StatusBadRequest, "invalid input")
		return
	}

	workspace, err := s.workspaces.UpdateWorkspace(request.Context(), user.ID, workspaceID, tenant.UpdateWorkspaceParams{Name: req.Name})
	if err != nil {
		writeTenantError(responseWriter, err)

		return
	}

	JSON(responseWriter, http.StatusOK, workspaceResponse{
		ID:        workspace.ID,
		Name:      workspace.Name,
		CreatedAt: workspace.CreatedAt.Format(time.RFC3339),
		UpdatedAt: workspace.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleDeleteWorkspace(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, ok := workspaceIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	if err := s.workspaces.DeleteWorkspace(request.Context(), user.ID, workspaceID); err != nil {
		writeTenantError(responseWriter, err)

		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

func writeTenantError(responseWriter http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tenant.ErrInvalidInput):
		Error(responseWriter, http.StatusBadRequest, "invalid input")
	case errors.Is(err, tenant.ErrUnauthorized):
		Error(responseWriter, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, tenant.ErrConflict):
		Error(responseWriter, http.StatusConflict, "conflict")
	default:
		Error(responseWriter, http.StatusInternalServerError, "internal error")
	}
}

func workspaceIDFromRequest(request *http.Request) (string, bool) {
	workspaceID := strings.TrimSpace(chi.URLParam(request, "workspaceId"))
	if workspaceID == "" {
		return "", false
	}

	return workspaceID, true
}
