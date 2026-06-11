// Package api owns JSON request decoding and response writing for the
// control HTTP API. It keeps transport envelopes consistent without tying
// feature packages back to the parent api package.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ErrRequestBodyTooLarge is returned when request decoding hits net/http's body-size limit.
var ErrRequestBodyTooLarge = errors.New("request body too large")

var apiValidator = newAPIValidator()

type errorResponse struct {
	Error string `json:"error"`
}

// JSON writes a JSON response body with the supplied HTTP status code.
func JSON(w http.ResponseWriter, status int, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

// Error writes the API's canonical JSON error envelope.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, errorResponse{Error: msg})
}

// ReadJSON strictly decodes one JSON value from the request body.
func ReadJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if isRequestBodyTooLarge(err) {
			return ErrRequestBodyTooLarge
		}
		return err
	}

	err := dec.Decode(&struct{}{})
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return ErrRequestBodyTooLarge
		}
		return errors.New("multiple json values")
	}

	return errors.New("multiple json values")
}

// ValidateStruct validates a decoded API request payload.
func ValidateStruct(v any) error {
	return apiValidator.Struct(v)
}

func isRequestBodyTooLarge(err error) bool {
	_, ok := errors.AsType[*http.MaxBytesError](err)
	return ok
}

func newAPIValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
		field := fl.Field()
		if field.Kind().String() != "string" {
			return false
		}
		return strings.TrimSpace(field.String()) != ""
	})
	return v
}
