package http_api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/t0gun/paas/internal/service"
)

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req createAppReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
	}

	app, err := s.svc.CreateApp(r.Context(), service.CreateAppParams{
		Name:  req.Name,
		Image: req.Image,
		Port:  req.Port,
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
