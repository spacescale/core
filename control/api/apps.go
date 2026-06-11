package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spacescale/core/control/fabric"
	"github.com/spacescale/core/control/service/tenant"
)

const createAppDispatchTimeout = 20 * time.Second

type createAppEnvVarRequest struct {
	Key      string `json:"key" validate:"required,notblank,envkey"`
	Value    string `json:"value" validate:"max=8192"`
	IsSecret bool   `json:"isSecret"`
}

type createAppComputeRequest struct {
	VCPU      uint32 `json:"vcpu" validate:"gt=0,lte=2147483647"`
	MemoryMB  uint64 `json:"memoryMb" validate:"gt=0,lte=9223372036854775807"`
	Dedicated bool   `json:"dedicated"`
}

type createAppRequest struct {
	Name                 string                   `json:"name" validate:"omitempty,notblank,max=63"`
	ImageRef             string                   `json:"imageRef" validate:"required,notblank,max=1024,excludesall= \t\r\n"`
	Compute              createAppComputeRequest  `json:"compute" validate:"required"`
	PrimaryRegion        string                   `json:"primaryRegion" validate:"required,notblank,max=32"`
	RuntimePort          *int                     `json:"runtimePort" validate:"omitempty,min=1,max=65535"`
	IsPublic             *bool                    `json:"isPublic"`
	RegistryCredentialID string                   `json:"registryCredentialId" validate:"omitempty,uuid"`
	EnvVars              []createAppEnvVarRequest `json:"envVars" validate:"max=50,dive"`
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

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	apps, err := s.apps.ListApps(request.Context(), user.ID, workspaceID, projectID)
	if err != nil {
		WriteTenantError(responseWriter, err)

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

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	var req createAppRequest
	if err := ReadAndValidateJSON(request, &req, false); err != nil {
		WriteJSONError(responseWriter, err)

		return
	}

	envVars := make([]tenant.AppEnvVarInput, 0, len(req.EnvVars))
	for _, item := range req.EnvVars {
		envVars = append(envVars, tenant.AppEnvVarInput{
			Key:      strings.ToUpper(strings.TrimSpace(item.Key)),
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
		WriteTenantError(responseWriter, err)

		return
	}

	app := result.App
	if s.dispatcher != nil {
		dispatchCtx, cancel := newCreateAppDispatchContext(request.Context())
		defer cancel()

		dispatchErr := s.dispatcher.Launch(dispatchCtx, fabric.Request{
			AppID:        result.AppID,
			DeploymentID: result.DeploymentID,
			MicroVMID:    result.MicroVMID,
			Region:       result.App.PrimaryRegion,
			Shape:        result.Shape,
			ImageRef:     result.App.ImageRef,
			Env:          result.Env,
			RuntimePort:  uint32(result.App.RuntimePort),
		})

		if dispatchErr != nil {
			switch {
			case errors.Is(dispatchErr, fabric.ErrNoAuctionBids):
				Error(responseWriter, http.StatusServiceUnavailable, "no capacity available")

				return
			case errors.Is(dispatchErr, fabric.ErrLaunchRejected):
				Error(responseWriter, http.StatusBadGateway, "launch rejected")

				return
			}
		}

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
