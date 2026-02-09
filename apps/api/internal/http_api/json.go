// This file centralizes JSON request and response behavior for HTTP handlers.
// Shared helpers keep response formatting consistent so each endpoint does not
// duplicate content-type headers, encoding, and error payload structure.
// Request decoding is intentionally strict by rejecting unknown fields and
// trailing JSON values, which prevents silent input drift and ambiguous bodies.
// When onboarding, start here to understand the default JSON contract enforced
// across all handlers in this package.

// Package http_api JSON helpers for HTTP request and response bodies.
package http_api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// writeJSON writes a JSON response body with the supplied status code.
// It also sets the content type header so clients can reliably interpret
// payloads from every handler in this package.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// readJSON decodes a request body into the destination value.
// Unknown fields are rejected to prevent silent input drift, and trailing data
// is rejected so one request maps to exactly one JSON object.
func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Reject trailing data after the first JSON value.
	if dec.More() {
		return errors.New("multiple json  values")
	}
	return nil
}

// errResp is the standard error response payload.
type errResp struct {
	Error string `json:"error"`
}

// writeErr writes a standard API error payload.
// This keeps error responses uniform across handlers and status codes.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errResp{Error: msg})
}
