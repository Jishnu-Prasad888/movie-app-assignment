// Package apperr defines the business errors returned by stores and services.
// The HTTP layer (package httpx) maps each Kind to a status code, so the domain code never
// needs to know about HTTP, and unexpected errors are never leaked to clients.
package apperr

import "errors"

type Kind int

const (
	Invalid       Kind = iota + 1 // 400
	Unauthorized                  // 401
	PaymentFailed                 // 402
	Forbidden                     // 403
	NotFound                      // 404
	Conflict                      // 409
	Gone                          // 410 (e.g. hold expired)
)

type Error struct {
	Kind    Kind
	Code    string // stable machine-readable code, e.g. "SEATS_UNAVAILABLE"
	Message string // human readable, safe to show to users
	Details map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// With attaches extra JSON fields to the error response (e.g. conflicting seat ids).
func (e *Error) With(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
	return e
}

// As returns the *Error inside err, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

func IsKind(err error, kind Kind) bool {
	e, ok := As(err)
	return ok && e.Kind == kind
}
