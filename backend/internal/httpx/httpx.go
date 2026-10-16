// Package httpx contains small JSON request/response helpers shared by all handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"yozigo/internal/apperr"
)

const maxBodyBytes = 1 << 20

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

var statusByKind = map[apperr.Kind]int{
	apperr.Invalid:       http.StatusBadRequest,
	apperr.Unauthorized:  http.StatusUnauthorized,
	apperr.PaymentFailed: http.StatusPaymentRequired,
	apperr.Forbidden:     http.StatusForbidden,
	apperr.NotFound:      http.StatusNotFound,
	apperr.Conflict:      http.StatusConflict,
	apperr.Gone:          http.StatusGone,
}

// Error writes err as {"error": "...", "code": "...", ...details}. Unknown errors are logged
// and reported as a generic 500 so internal details never reach the client.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	if e, ok := apperr.As(err); ok {
		body := map[string]any{}
		for k, v := range e.Details {
			body[k] = v
		}
		body["error"] = e.Message
		body["code"] = e.Code
		status, ok := statusByKind[e.Kind]
		if !ok {
			status = http.StatusInternalServerError
		}
		JSON(w, status, body)
		return
	}
	slog.Error("internal error", "method", r.Method, "path", r.URL.Path, "err", err)
	JSON(w, http.StatusInternalServerError, map[string]any{
		"error": "Something went wrong. Please try again.",
		"code":  "INTERNAL",
	})
}

// Decode reads a JSON body into dst, rejecting unknown fields and oversized bodies.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apperr.New(apperr.Invalid, "INVALID_JSON", "Request body is required.")
		}
		return apperr.New(apperr.Invalid, "INVALID_JSON", fmt.Sprintf("Invalid request body: %v", err))
	}
	return nil
}

// PathID parses a positive integer path parameter such as {id}.
func PathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.New(apperr.NotFound, "NOT_FOUND", "Not found.")
	}
	return id, nil
}
