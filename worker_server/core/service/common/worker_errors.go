package common

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrBadRequest       = errors.New("bad request")
	ErrConflict         = errors.New("conflict")
	ErrInternalError    = errors.New("internal error")
	ErrTokenExpired     = errors.New("token expired")
	ErrInvalidToken     = errors.New("invalid token")
	ErrProviderError    = errors.New("provider error")
	ErrRateLimited      = errors.New("rate limited")
	ErrConnectionClosed = errors.New("connection closed")
)
