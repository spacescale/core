package http_api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/t0gun/paas/internal/service"
)

type Server struct {
	svc *service.AppService
}

func NewServer(svc *service.AppService) *Server {
	return &Server{svc: svc}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health Check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/v0", func(r chi.Router) {
		r.Post("/apps", s.handleCreateApp)
		r.Post("/apps/{appID}/deploy", s.handleDeployApp)
		r.Get("/apps/{appID}/deployments", s.handleListDeployments)
		r.Post("/deployments/next:process", s.handleProcessNextDeployment)
	})

	return r
}
