package http_api

import (
	"errors"
	"net/http"

	"github.com/t0gun/paas/internal/service"
)

func mapServiceErr(err error) (status int, msg string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return http.StatusBadRequest, "invalid input"
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrNoRuntime):
		return http.StatusServiceUnavailable, "runtime not configured"
	case errors.Is(err, service.ErrNoWork):
		return http.StatusNoContent, ""
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
