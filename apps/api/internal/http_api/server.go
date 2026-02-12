// This file builds the API router and middleware stack.
// It defines the Server wrapper that receives service dependencies and exposes
// one Router method used by main and integration tests.
// Route registration for health checks and versioned endpoints is centralized
// here so application wiring is discoverable in one place.
// When adding new endpoints, most HTTP route setup changes should begin here.

// Package http_api  routing and middleware wiring.
package http_api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/t0gun/spacescale/internal/service"
)

// Server wires HTTP handlers to service dependencies and auth configuration.
type Server struct {
	svc     *service.ProjectService
	authCfg AuthConfig
}

// NewServer creates a Server bound to the provided project service.
// Keeping wiring here makes dependencies explicit for startup and tests.
func NewServer(svc *service.ProjectService, authCfg AuthConfig) *Server {
	return &Server{
		svc:     svc,
		authCfg: authCfg,
	}
}

// Router builds the full HTTP router and middleware stack.
// It registers health and versioned API routes, then applies request-level
// middleware for traceability, logging, panic recovery, and authentication.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Base middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health check route.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/v0", func(r chi.Router) {
		r.Use(authMiddleware(s.authCfg))
		r.Post("/projects", s.handleCreateProject)
	})

	return r
}
