package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spacescale/core/scalecp/fabric"
	"github.com/spacescale/core/scalecp/service/tenant"
)

const createAppDispatchTimeout = 20 * time.Second

type createAppEnvVarRequest struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}

type createAppComputeRequest struct {
	VCPU      uint32 `json:"vcpu"`
	MemoryMB  uint64 `json:"memoryMb"`
	Dedicated bool   `json:"dedicated"`
}

type createAppRequest struct {
	Name                 string                   `json:"name"`
	ImageRef             string                   `json:"imageRef"`
	Compute              createAppComputeRequest  `json:"compute"`
	PrimaryRegion        string                   `json:"primaryRegion"`
	RuntimePort          *int                     `json:"runtimePort"`
	IsPublic             *bool                    `json:"isPublic"`
	RegistryCredentialID string                   `json:"registryCredentialId"`
	EnvVars              []createAppEnvVarRequest `json:"envVars"`
}

type appResponse struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Subdomain      string `json:"subdomain"`
	ImageRef       string `json:"imageRef"`
	TargetReplicas int32  `json:"targetReplicas"`
	PrimaryRegion  string `json:"primaryRegion"`
	RuntimePort    int32  `json:"runtimePort"`
	Status         string `json:"status"`
	IsPublic       bool   `json:"isPublic"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type listAppsResponse struct {
	Apps []appResponse `json:"apps"`
}

func (s *Server) handleListApps(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID := strings.TrimSpace(chi.URLParam(request, "workspaceId"))
	projectID := strings.TrimSpace(chi.URLParam(request, "projectId"))
	if workspaceID == "" || projectID == "" {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	apps, err := s.apps.ListApps(request.Context(), user.ID, workspaceID, projectID)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			Error(responseWriter, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			Error(responseWriter, http.StatusUnauthorized, "unauthorized")
		default:
			Error(responseWriter, http.StatusInternalServerError, "internal error")
		}

		return
	}

	items := make([]appResponse, 0, len(apps))
	for _, app := range apps {
		items = append(items, appResponseFromModel(app))
	}
	JSON(responseWriter, http.StatusOK, listAppsResponse{Apps: items})
}

func (s *Server) handleCreateApp(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID := strings.TrimSpace(chi.URLParam(request, "workspaceId"))
	projectID := strings.TrimSpace(chi.URLParam(request, "projectId"))
	if workspaceID == "" || projectID == "" {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	var req createAppRequest
	if err := ReadJSON(request, &req); err != nil {
		switch {
		case errors.Is(err, ErrRequestBodyTooLarge):
			Error(responseWriter, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			Error(responseWriter, http.StatusBadRequest, "invalid json")
		}

		return
	}

	envVars := make([]tenant.AppEnvVarInput, 0, len(req.EnvVars))
	for _, item := range req.EnvVars {
		envVars = append(envVars, tenant.AppEnvVarInput{
			Key:      item.Key,
			Value:    item.Value,
			IsSecret: item.IsSecret,
		})
	}

	result, err := s.apps.CreateApp(request.Context(), user.ID, workspaceID, projectID, tenant.CreateAppParams{
		Name:     req.Name,
		ImageRef: req.ImageRef,
		Compute: tenant.AppComputeInput{
			VCPU:      req.Compute.VCPU,
			MemoryMB:  req.Compute.MemoryMB,
			Dedicated: req.Compute.Dedicated,
		},
		PrimaryRegion:        req.PrimaryRegion,
		RuntimePort:          req.RuntimePort,
		IsPublic:             req.IsPublic,
		RegistryCredentialID: req.RegistryCredentialID,
		EnvVars:              envVars,
	})
	if err != nil {
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

		return
	}

	app := result.App
	if lc, ok := MetadataFromContext(request.Context()); ok {
		lc.ProjectID = app.ProjectID
		lc.AppID = app.ID
	}

	if s.dispatcher != nil {
		dispatchCtx, cancel := newCreateAppDispatchContext(request.Context())
		defer cancel()

		dispatchErr := s.dispatcher.Launch(dispatchCtx, fabric.Request{
			AppID:        result.AppID,
			DeploymentID: result.DeploymentID,
			MicroVMID:    result.MicroVMID,
			Region:       result.Region,
			Shape:        result.Shape,
			ImageRef:     result.ImageRef,
			Env:          result.Env,
			RuntimePort:  result.RuntimePort,
		})

		refreshed, refreshErr := s.apps.GetApp(dispatchCtx, user.ID, workspaceID, projectID, app.ID)
		app = resolveCreateAppAfterDispatch(app, dispatchErr, refreshed, refreshErr)
	}

	responseWriter.Header().Set(
		"Location",
		"/v1/workspaces/"+url.PathEscape(workspaceID)+"/projects/"+url.PathEscape(projectID)+"/apps/"+url.PathEscape(app.ID),
	)
	JSON(responseWriter, http.StatusCreated, appResponseFromModel(app))
}

func appResponseFromModel(app tenant.App) appResponse {
	return appResponse{
		ID:             app.ID,
		ProjectID:      app.ProjectID,
		Name:           app.Name,
		Slug:           app.Slug,
		Subdomain:      app.Subdomain,
		ImageRef:       app.ImageRef,
		TargetReplicas: app.TargetReplicas,
		PrimaryRegion:  app.PrimaryRegion,
		RuntimePort:    app.RuntimePort,
		Status:         app.Status,
		IsPublic:       app.IsPublic,
		CreatedAt:      app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      app.UpdatedAt.Format(time.RFC3339),
	}
}

func newCreateAppDispatchContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), createAppDispatchTimeout)
}

func resolveCreateAppAfterDispatch(current tenant.App, dispatchErr error, refreshed tenant.App, refreshErr error) tenant.App {
	if refreshErr == nil {
		return refreshed
	}
	if dispatchErr == nil {
		current.Status = "deploying"
	}

	return current
}
