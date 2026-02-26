package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes
const (
	// Auth errors
	CodeUnauthorized = "UNAUTHORIZED"
	CodeInvalidToken = "INVALID_TOKEN"
	CodeTokenExpired = "TOKEN_EXPIRED"
	CodeForbidden    = "FORBIDDEN"

	// Validation errors
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeBadRequest       = "BAD_REQUEST"
	CodeInvalidInput     = "INVALID_INPUT"
	CodeMissingField     = "MISSING_FIELD"

	// Resource errors
	CodeNotFound      = "NOT_FOUND"
	CodeAlreadyExists = "ALREADY_EXISTS"
	CodeConflict      = "CONFLICT"

	// External errors
	CodeOAuthFailed   = "OAUTH_FAILED"
	CodeDatabaseError = "DATABASE_ERROR"
	CodeExternalError = "EXTERNAL_ERROR"

	// Internal errors
	CodeInternalError = "INTERNAL_ERROR"
	CodeConfigError   = "CONFIG_ERROR"
	CodeTimeout       = "TIMEOUT"
)

// AppError represents a structured application error
type AppError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Status  int            `json:"-"`
	Details map[string]any `json:"details,omitempty"`
	Err     error          `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func (e *AppError) WithDetail(key string, value any) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

func (e *AppError) WithError(err error) *AppError {
	e.Err = err
	return e
}

// HTTPStatus returns the HTTP status code
func (e *AppError) HTTPStatus() int {
	return e.Status
}

// Constructor functions
func New(code, message string, status int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Status:  status,
	}
}

func Wrap(err error, code, message string, status int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Status:  status,
		Err:     err,
	}
}

// Auth errors
func Unauthorized(message string) *AppError {
	if message == "" {
		message = "unauthorized"
	}
	return &AppError{
		Code:    CodeUnauthorized,
		Message: message,
		Status:  http.StatusUnauthorized,
	}
}

func InvalidToken(message string) *AppError {
	return &AppError{
		Code:    CodeInvalidToken,
		Message: message,
		Status:  http.StatusUnauthorized,
	}
}

func Forbidden(message string) *AppError {
	if message == "" {
		message = "forbidden"
	}
	return &AppError{
		Code:    CodeForbidden,
		Message: message,
		Status:  http.StatusForbidden,
	}
}

// Validation errors
func BadRequest(message string) *AppError {
	return &AppError{
		Code:    CodeBadRequest,
		Message: message,
		Status:  http.StatusBadRequest,
	}
}

func ValidationFailed(message string) *AppError {
	return &AppError{
		Code:    CodeValidationFailed,
		Message: message,
		Status:  http.StatusBadRequest,
	}
}

func InvalidInput(field, reason string) *AppError {
	return &AppError{
		Code:    CodeInvalidInput,
		Message: fmt.Sprintf("invalid input for '%s': %s", field, reason),
		Status:  http.StatusBadRequest,
		Details: map[string]any{"field": field},
	}
}

func MissingField(field string) *AppError {
	return &AppError{
		Code:    CodeMissingField,
		Message: fmt.Sprintf("missing required field: %s", field),
		Status:  http.StatusBadRequest,
		Details: map[string]any{"field": field},
	}
}

// Resource errors
func NotFound(resource string) *AppError {
	return &AppError{
		Code:    CodeNotFound,
		Message: fmt.Sprintf("%s not found", resource),
		Status:  http.StatusNotFound,
	}
}

func AlreadyExists(resource string) *AppError {
	return &AppError{
		Code:    CodeAlreadyExists,
		Message: fmt.Sprintf("%s already exists", resource),
		Status:  http.StatusConflict,
	}
}

func Conflict(message string) *AppError {
	return &AppError{
		Code:    CodeConflict,
		Message: message,
		Status:  http.StatusConflict,
	}
}

// External errors
func OAuthFailed(provider string, err error) *AppError {
	return &AppError{
		Code:    CodeOAuthFailed,
		Message: fmt.Sprintf("OAuth failed for %s", provider),
		Status:  http.StatusBadGateway,
		Details: map[string]any{"provider": provider},
		Err:     err,
	}
}

func DatabaseError(operation string, err error) *AppError {
	return &AppError{
		Code:    CodeDatabaseError,
		Message: fmt.Sprintf("database error: %s", operation),
		Status:  http.StatusInternalServerError,
		Err:     err,
	}
}

func ExternalError(service string, err error) *AppError {
	return &AppError{
		Code:    CodeExternalError,
		Message: fmt.Sprintf("external service error: %s", service),
		Status:  http.StatusBadGateway,
		Details: map[string]any{"service": service},
		Err:     err,
	}
}

// Internal errors
func Internal(message string) *AppError {
	if message == "" {
		message = "internal server error"
	}
	return &AppError{
		Code:    CodeInternalError,
		Message: message,
		Status:  http.StatusInternalServerError,
	}
}

func InternalWithError(err error) *AppError {
	return &AppError{
		Code:    CodeInternalError,
		Message: "internal server error",
		Status:  http.StatusInternalServerError,
		Err:     err,
	}
}

func ConfigError(message string) *AppError {
	return &AppError{
		Code:    CodeConfigError,
		Message: message,
		Status:  http.StatusInternalServerError,
	}
}

func Timeout(operation string) *AppError {
	return &AppError{
		Code:    CodeTimeout,
		Message: fmt.Sprintf("operation timed out: %s", operation),
		Status:  http.StatusGatewayTimeout,
	}
}

// Common error instances
var (
	ErrNotFound     = NotFound("resource")
	ErrUnauthorized = Unauthorized("")
	ErrForbidden    = Forbidden("")
	ErrBadRequest   = BadRequest("bad request")
	ErrInternal     = Internal("")
	ErrConflict     = Conflict("resource conflict")
	ErrRateLimited  = New("RATE_LIMITED", "too many requests", http.StatusTooManyRequests)
)

// Helper functions
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

func AsAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return InternalWithError(err)
}

func GetHTTPStatus(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Status
	}
	return http.StatusInternalServerError
}
