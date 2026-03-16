// This file centralizes JSON request and response behavior for HTTP handlers.
// Shared helpers keep response formatting consistent so each endpoint does not
// duplicate content-type headers, encoding, and error payload structure.
// Request decoding is intentionally strict by rejecting unknown fields and
// trailing JSON values, which prevents silent input drift and ambiguous bodies.
// When onboarding, start here to understand the default JSON contract enforced
// across all handlers in this package.

// Package http_api JSON helpers for HTTP request and response bodies.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var errRequestBodyTooLarge = errors.New("request body too large") // returned when request body exceeds configured max size.

// isRequestBodyTooLarge reports whether an error came from net/http body-size
// enforcement. This keeps size-limit detection logic centralized.
func isRequestBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

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
//
// Return behavior:
// - errRequestBodyTooLarge when body exceeds configured size limits
// - decoder errors for malformed/invalid JSON
// - "multiple json values" when trailing JSON exists after first value
func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if isRequestBodyTooLarge(err) {
			return errRequestBodyTooLarge
		}
		return err
	}

	// Reject trailing data after the first JSON value.
	// Decode one more token: only io.EOF means there was exactly one value.
	err := dec.Decode(&struct{}{})
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return errRequestBodyTooLarge
		}
		return errors.New("multiple json values")
	}

	// No decode error means there was at least one trailing JSON value.
	return errors.New("multiple json values")
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
