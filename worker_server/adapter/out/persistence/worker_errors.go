package persistence

import "errors"

// Common persistence errors
var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicate    = errors.New("duplicate entry")
	ErrInvalidInput = errors.New("invalid input")
)
