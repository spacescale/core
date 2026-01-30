// JSON helpers for HTTP request and response bodies.
package http_api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// writeJSON sends a JSON response with the provided status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// readJSON decodes a JSON request body into the destination struct.
func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Prevent Trailing garbage JSON
	if dec.More() {
		return errors.New("multiple json  values")
	}
	return nil
}

// errResp is the standard error response payload.
type errResp struct {
	Error string `json:"error"`
}

// writeErr sends a standard error response with a message.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errResp{Error: msg})
}
