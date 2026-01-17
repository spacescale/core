package http_api

import "net/http"

type WorkerAuth struct {
	Token string
}

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
