// Package errors defines stable cross-transport error categories.
package errors

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrDependency   = errors.New("dependency unavailable")
	ErrDuplicate    = errors.New("duplicate message")
)
