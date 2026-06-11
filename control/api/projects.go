package api

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spacescale/core/control/service/tenant"
)

type createProjectRequest struct {
	Name string `json:"name" validate:"omitempty,notblank,max=120"`
}

type updateProjectRequest struct {
	Name string `json:"name" validate:"required,notblank,max=120"`
}

type projectResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type listProjectsResponse struct {
	Projects []projectResponse `json:"projects"`
}

func (s *Server) handleCreateProject(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	var req createProjectRequest
	if err := ReadAndValidateJSON(request, &req, true); err != nil {
		WriteJSONError(responseWriter, err)

		return
	}

	workspaceID, ok := workspaceIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	project, err := s.projects.CreateProject(request.Context(), user.ID, workspaceID, tenant.CreateProjectParams{Name: req.Name})
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	responseWriter.Header().Set(
		"Location",
		"/v1/workspaces/"+url.PathEscape(project.WorkspaceID)+"/projects/"+url.PathEscape(project.ID),
	)
	JSON(responseWriter, http.StatusCreated, projectResponseFromModel(project))
}

func (s *Server) handleListProjects(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, ok := workspaceIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	projects, err := s.projects.ListProjects(request.Context(), user.ID, workspaceID)
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	items := make([]projectResponse, 0, len(projects))
	for _, project := range projects {
		items = append(items, projectResponseFromModel(project))
	}
	JSON(responseWriter, http.StatusOK, listProjectsResponse{Projects: items})
}

func (s *Server) handleGetProject(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	project, err := s.projects.GetProject(request.Context(), user.ID, workspaceID, projectID)
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	JSON(responseWriter, http.StatusOK, projectResponseFromModel(project))
}

func (s *Server) handleUpdateProject(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	var req updateProjectRequest
	if err := ReadAndValidateJSON(request, &req, false); err != nil {
		WriteJSONError(responseWriter, err)

		return
	}

	project, err := s.projects.UpdateProject(request.Context(), user.ID, workspaceID, projectID, tenant.UpdateProjectParams{Name: req.Name})
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	JSON(responseWriter, http.StatusOK, projectResponseFromModel(project))
}

func (s *Server) handleDeleteProject(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	err := s.projects.DeleteProject(request.Context(), user.ID, workspaceID, projectID)
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	responseWriter.WriteHeader(http.StatusNoContent)
}

func projectResponseFromModel(project tenant.Project) projectResponse {
	return projectResponse{
		ID:          project.ID,
		WorkspaceID: project.WorkspaceID,
		Name:        project.Name,
		Slug:        project.Slug,
		CreatedAt:   project.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   project.UpdatedAt.Format(time.RFC3339),
	}
}

func workspaceAndProjectIDFromRequest(request *http.Request) (string, string, bool) {
	workspaceID := strings.TrimSpace(chi.URLParam(request, "workspaceId"))
	projectID := strings.TrimSpace(chi.URLParam(request, "projectId"))
	if workspaceID == "" || projectID == "" {
		return "", "", false
	}

	return workspaceID, projectID, true
}
