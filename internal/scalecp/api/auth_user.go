// This file provides helpers that resolve the authenticated caller into a
// persisted user record for /v1 handlers.

package api

import (
	"errors"
	"net/http"

	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

// requireCallerUser resolves the authenticated principal and loads the
// corresponding user row. It writes a normalized HTTP error response and
// returns ok=false when resolution fails.
func (s *Server) requireCallerUser(w http.ResponseWriter, r *http.Request) (tenant.User, bool) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return tenant.User{}, false
	}

	user, err := s.users.GetUserByIdentityKey(r.Context(), principal.IdentityKey)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			writeErr(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			writeErr(w, http.StatusUnauthorized, "unauthorized")
		default:
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return tenant.User{}, false
	}

	return user, true
}
