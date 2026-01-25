// Package http_api Worker authentication middleware for privileged endpoints.
package http_api

import "net/http"

// WorkerAuth protects worker only endpoints with a shared token.
type WorkerAuth struct {
	Token string
}

// Middleware enforces the worker token when it is configured.
func (a WorkerAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Token == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Worker-Token") != a.Token {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
