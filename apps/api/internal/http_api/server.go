// Package http_api  routing and middleware wiring.
package http_api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/t0gun/spacescale/internal/service"
)

// Server wires HTTP handlers to the project service.
type Server struct {
	svc *service.ProjectService
}

// NewServer builds an API server with the service.
func NewServer(svc *service.ProjectService) *Server {
	return &Server{svc: svc}
}

// Router builds the HTTP routes and middleware stack.
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
		r.Post("/projects", s.handleCreateProject)
	})

	return r
}
