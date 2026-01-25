// HTTP API handlers for app and deployment routes.
// Handlers decode JSON and call the service.
// Errors are mapped to HTTP status codes.
// Success responses write JSON payloads.
// This file ties routing to service logic.

package http_api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/t0gun/spacescale/internal/service"
)

// handleCreateApp handles app creation requests.
func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req createAppReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	app, err := s.svc.CreateApp(r.Context(), service.CreateAppParams{
		Name:   req.Name,
		Image:  req.Image,
		Port:   req.Port,
		Expose: req.Expose,
		Env:    req.Env,
	})
	if err != nil {
		status, msg := mapServiceErr(err)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		writeErr(w, status, msg)
		return
	}
	writeJSON(w, http.StatusCreated, toAppResp(app))
}

// handleDeployApp handles app deployment requests.
func (s *Server) handleDeployApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	dep, err := s.svc.DeployApp(r.Context(), service.DeployAppParams{AppID: appID})
	if err != nil {
		status, msg := mapServiceErr(err)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		writeErr(w, status, msg)
		return
	}
	writeJSON(w, http.StatusAccepted, toDeploymentResp(dep))
}

// handleListDeployments lists deployments for an app.
func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	deps, err := s.svc.ListDeployments(r.Context(), service.ListDeploymentsParams{AppID: appID})
	if err != nil {
		status, msg := mapServiceErr(err)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		writeErr(w, status, msg)
		return
	}
	out := make([]deploymentResp, 0, len(deps))
	for _, d := range deps {
		out = append(out, toDeploymentResp(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleProcessNextDeployment processes the next queued deployment.
func (s *Server) handleProcessNextDeployment(w http.ResponseWriter, r *http.Request) {
	dep, err := s.svc.ProcessNextDeployment(r.Context())
	if err != nil {
		status, msg := mapServiceErr(err)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		writeErr(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, toDeploymentResp(dep))
}

// handleListApps lists all apps.
func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.svc.ListApps(r.Context())
	if err != nil {
		status, msg := mapServiceErr(err)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		writeErr(w, status, msg)
		return
	}
	out := make([]appResp, 0, len(apps))
	for _, a := range apps {
		out = append(out, toAppResp(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetAppByID returns one app by id.
func (s *Server) handleGetAppByID(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	app, err := s.svc.GetAppByID(r.Context(), appID)
	if err != nil {
		status, msg := mapServiceErr(err)
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		writeErr(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, toAppResp(app))
}
