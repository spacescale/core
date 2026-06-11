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
	"github.com/spacescale/core/control/service/tenant"
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

// ReadAndValidateJSON decodes one JSON value and validates the result.
// When allowEmpty is true, an empty body is treated as a zero-value payload.
func ReadAndValidateJSON(r *http.Request, dst any, allowEmpty bool) error {
	err := ReadJSON(r, dst)
	if errors.Is(err, io.EOF) {
		if allowEmpty {
			return apiValidator.Struct(dst)
		}
		return err
	}
	if err != nil {
		return err
	}
	return apiValidator.Struct(dst)
}

// WriteJSONError maps request decoding and validation errors to API responses.
func WriteJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRequestBodyTooLarge):
		Error(w, http.StatusRequestEntityTooLarge, "request body too large")
	case func() bool {
		var validationErrs validator.ValidationErrors
		return errors.As(err, &validationErrs)
	}():
		Error(w, http.StatusBadRequest, "invalid input")
	default:
		Error(w, http.StatusBadRequest, "invalid json")
	}
}

// WriteTenantError maps service-layer errors to API responses.
func WriteTenantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tenant.ErrInvalidInput):
		Error(w, http.StatusBadRequest, "invalid input")
	case errors.Is(err, tenant.ErrUnauthorized):
		Error(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, tenant.ErrConflict):
		Error(w, http.StatusConflict, "conflict")
	default:
		Error(w, http.StatusInternalServerError, "internal error")
	}
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
