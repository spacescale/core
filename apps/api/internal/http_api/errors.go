// HTTP error mapping helpers.
package http_api

import (
	"errors"
	"net/http"

	"github.com/t0gun/spacescale/internal/service"
)

// mapServiceErr converts service errors into HTTP status codes and messages.
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
