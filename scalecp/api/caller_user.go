package auth

import (
	"errors"
	"net/http"

	"github.com/spacescale/core/internal/scalecp/api/respond"
	"github.com/spacescale/core/internal/scalecp/service/tenant"
)

// RequireCallerUser resolves the authenticated principal into a persisted user row.
func RequireCallerUser(w http.ResponseWriter, r *http.Request, users *tenant.UserService) (tenant.User, bool) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "unauthorized")
		return tenant.User{}, false
	}

	user, err := users.GetUserByIdentityKey(r.Context(), principal.IdentityKey)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrInvalidInput):
			respond.Error(w, http.StatusBadRequest, "invalid input")
		case errors.Is(err, tenant.ErrUnauthorized):
			respond.Error(w, http.StatusUnauthorized, "unauthorized")
		default:
			respond.Error(w, http.StatusInternalServerError, "internal error")
		}
		return tenant.User{}, false
	}

	return user, true
}
