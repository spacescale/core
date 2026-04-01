// This file implements HTTP transport for app endpoints.
//
// Responsibilities in this file:
// - Decode and validate request envelopes at transport boundaries.
// - Resolve authenticated caller context to a persisted user.
// - Translate service-layer outcomes into stable HTTP status/error payloads.
// - Serialize service models into API response contracts consumed by clients.
//
// Design intent:
// - Keep handler logic thin and deterministic.
// - Keep business invariants and persistence orchestration in service package.

package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spacescale/core/internal/scalecp/fabric/dispatch"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

type createAppEnvVarRequest struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}

type createAppRequest struct {
	Name                 string                   `json:"name"`
	ImageRef             string                   `json:"imageRef"`
	Tier                 string                   `json:"tier"`
	PrimaryRegion        string                   `json:"primaryRegion"`
	RuntimePort          *int                     `json:"runtimePort"`
	IsPublic             *bool                    `json:"isPublic"`
	RegistryCredentialID string                   `json:"registryCredentialId"`
	EnvVars              []createAppEnvVarRequest `json:"envVars"`
}

type appResponse struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Subdomain     string `json:"subdomain"`
	ImageRef      string `json:"imageRef"`
	Tier          string `json:"tier"`
	PrimaryRegion string `json:"primaryRegion"`
	RuntimePort   int32  `json:"runtimePort"`
	Status        string `json:"status"`
	IsPublic      bool   `json:"isPublic"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type listAppsResponse struct {
	Apps []appResponse `json:"apps"`
}

func appResponseFromModel(app tenant.App) appResponse {
	return appResponse{
		ID:            app.ID,
		ProjectID:     app.ProjectID,
		Name:          app.Name,
		Slug:          app.Slug,
		Subdomain:     app.Subdomain,
		ImageRef:      app.ImageRef,
		Tier:          app.Tier,
		PrimaryRegion: app.PrimaryRegion,
		RuntimePort:   app.RuntimePort,
		Status:        app.Status,
		IsPublic:      app.IsPublic,
		CreatedAt:     app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     app.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}

	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
	projectID := strings.TrimSpace(chi.URLParam(r, "projectId"))
	if workspaceID == "" || projectID == "" {
		writeErr(w, http.StatusBadRequest, "invalid input")
		return
	}

	apps, err := s.apps.ListApps(r.Context(), user.ID, workspaceID, projectID)
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

	items := make([]appResponse, 0, len(apps))
	for _, app := range apps {
		items = append(items, appResponseFromModel(app))
	}

	writeJSON(w, http.StatusOK, listAppsResponse{Apps: items})
}

// handleCreateApp creates one app in an owned workspace/project.
func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCallerUser(w, r)
	if !ok {
		return
	}

	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
	projectID := strings.TrimSpace(chi.URLParam(r, "projectId"))
	if workspaceID == "" || projectID == "" {
		writeErr(w, http.StatusBadRequest, "invalid input")
		return
	}

	var req createAppRequest
	if err := readJSON(r, &req); err != nil {
		switch {
		case errors.Is(err, errRequestBodyTooLarge):
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			writeErr(w, http.StatusBadRequest, "invalid json")
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

	result, err := s.apps.CreateApp(r.Context(), user.ID, workspaceID, projectID, tenant.CreateAppParams{
		Name:                 req.Name,
		ImageRef:             req.ImageRef,
		Tier:                 req.Tier,
		PrimaryRegion:        req.PrimaryRegion,
		RuntimePort:          req.RuntimePort,
		IsPublic:             req.IsPublic,
		RegistryCredentialID: req.RegistryCredentialID,
		EnvVars:              envVars,
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

	app := result.App

	if s.dispatcher != nil {
		if err := s.dispatcher.Launch(r.Context(), dispatch.Request{
			AppID:        result.AppID,
			DeploymentID: result.DeploymentID,
			MachineID:    result.MachineID,
			Region:       result.Region,
			Tier:         result.Tier,
			ImageRef:     result.ImageRef,
			Env:          result.Env,
		}); err != nil {
			switch {
			case errors.Is(err, dispatch.ErrNoAuctionBids), errors.Is(err, dispatch.ErrLaunchRejected):
				writeErr(w, http.StatusServiceUnavailable, "no capacity available")
			default:
				writeErr(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		app.Status = "deploying"
	}

	if lc, ok := logContextFromContext(r.Context()); ok {
		lc.ProjectID = app.ProjectID
		lc.AppID = app.ID
	}

	w.Header().Set(
		"Location",
		"/v1/workspaces/"+url.PathEscape(workspaceID)+"/projects/"+url.PathEscape(projectID)+"/apps/"+url.PathEscape(app.ID),
	)
	writeJSON(w, http.StatusCreated, appResponseFromModel(app))
}
