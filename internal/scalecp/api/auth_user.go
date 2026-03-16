// This file provides helpers that resolve the authenticated caller into a
// persisted user record for /v1 handlers.

package api

import (
	"errors"
	"net/http"

	"github.com/spacescale/core/internal/scalecp/service"
)

// requireCallerUser resolves the authenticated principal and loads the
// corresponding user row. It writes a normalized HTTP error response and
// returns ok=false when resolution fails.
func (s *Server) requireCallerUser(w http.ResponseWriter, r *http.Request) (service.User, bool) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return service.User{}, false
	}

	user, err := s.services.Users.GetUserByIdentityKey(r.Context(), principal.IdentityKey)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, service.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return service.User{}, false
	}

	return user, true
}
