package http

import (
	"errors"
	"time"

	"worker_server/pkg/apperr"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var ErrUnauthorized = errors.New("unauthorized")

// GetUserID safely extracts user_id from fiber context
// Returns error if not authenticated
func GetUserID(c *fiber.Ctx) (uuid.UUID, error) {
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return uuid.Nil, ErrUnauthorized
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrUnauthorized
	}
	return userID, nil
}

// MustGetUserID extracts user_id and returns fiber error if not found
func MustGetUserID(c *fiber.Ctx) (uuid.UUID, error) {
	userID, err := GetUserID(c)
	if err != nil {
		return uuid.Nil, c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}
	return userID, nil
}

// GetConnectionID extracts connection_id from query params (for multi-account support)
func GetConnectionID(c *fiber.Ctx) *int64 {
	connID := c.QueryInt("connection_id", 0)
	if connID > 0 {
		id := int64(connID)
		return &id
	}
	return nil
}

// =============================================================================
// Standardized Error Response Helpers
// =============================================================================

// APIResponse represents a standard API response
type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// APIError represents a standard API error
type APIError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ErrorResponse sends a standardized JSON error response
func ErrorResponse(c *fiber.Ctx, status int, message string) error {
	requestID, _ := c.Locals("request_id").(string)
	return c.Status(status).JSON(APIResponse{
		Success:   false,
		Error:     &APIError{Code: mapStatusToCode(status), Message: message},
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorResponseWithCode sends a standardized error response with custom code
func ErrorResponseWithCode(c *fiber.Ctx, status int, code, message string) error {
	requestID, _ := c.Locals("request_id").(string)
	return c.Status(status).JSON(APIResponse{
		Success:   false,
		Error:     &APIError{Code: code, Message: message},
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorResponseWithDetails sends a standardized error with details
func ErrorResponseWithDetails(c *fiber.Ctx, status int, code, message string, details map[string]interface{}) error {
	requestID, _ := c.Locals("request_id").(string)
	return c.Status(status).JSON(APIResponse{
		Success:   false,
		Error:     &APIError{Code: code, Message: message, Details: details},
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// AppErrorResponse handles apperr.AppError and returns appropriate response
func AppErrorResponse(c *fiber.Ctx, err error) error {
	appErr := apperr.AsAppError(err)
	requestID, _ := c.Locals("request_id").(string)
	return c.Status(appErr.Status).JSON(APIResponse{
		Success:   false,
		Error:     &APIError{Code: appErr.Code, Message: appErr.Message, Details: appErr.Details},
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// InternalErrorResponse returns a safe 500 error without exposing internal details.
// Use this instead of ErrorResponse(c, 500, err.Error()) to prevent information leakage.
// The error is logged with context but only a generic message is returned to the client.
func InternalErrorResponse(c *fiber.Ctx, err error, operation string) error {
	// Log the actual error for debugging
	logger.WithError(err).WithField("operation", operation).Error("internal error")
	return ErrorResponseWithCode(c, 500, "INTERNAL_ERROR", operation+" failed")
}

// SuccessResponse sends a standardized JSON success response
func SuccessResponse(c *fiber.Ctx, data any) error {
	requestID, _ := c.Locals("request_id").(string)
	return c.JSON(APIResponse{
		Success:   true,
		Data:      data,
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// SuccessResponseSimple sends a simple JSON response (for backward compatibility)
func SuccessResponseSimple(c *fiber.Ctx, data any) error {
	return c.JSON(data)
}

// mapStatusToCode maps HTTP status to error code
func mapStatusToCode(status int) string {
	switch status {
	case 400:
		return apperr.CodeBadRequest
	case 401:
		return apperr.CodeUnauthorized
	case 403:
		return apperr.CodeForbidden
	case 404:
		return apperr.CodeNotFound
	case 409:
		return apperr.CodeConflict
	case 429:
		return "RATE_LIMITED"
	case 500:
		return apperr.CodeInternalError
	default:
		return "UNKNOWN_ERROR"
	}
}

// =============================================================================
// Pagination Helpers
// =============================================================================

// PaginationParams holds common pagination parameters
type PaginationParams struct {
	Limit     int
	Offset    int
	Page      int
	PageToken string
}

// GetPaginationParams extracts pagination params from query
func GetPaginationParams(c *fiber.Ctx, defaultLimit int) PaginationParams {
	limit := c.QueryInt("limit", defaultLimit)
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > 100 {
		limit = 100
	}

	offset := c.QueryInt("offset", 0)
	page := c.QueryInt("page", 0)
	pageToken := c.Query("page_token", "")

	return PaginationParams{
		Limit:     limit,
		Offset:    offset,
		Page:      page,
		PageToken: pageToken,
	}
}

// =============================================================================
// Response Helpers
// =============================================================================

// ListResponse represents a paginated list response
type ListResponse struct {
	Data     interface{} `json:"data,omitempty"`
	Emails   interface{} `json:"emails,omitempty"` // for backwards compatibility
	Total    int         `json:"total"`
	HasMore  bool        `json:"has_more"`
	Page     int         `json:"page,omitempty"`
	PageSize int         `json:"page_size,omitempty"`
}

// NewListResponse creates a list response with has_more calculation
func NewListResponse(data interface{}, total, offset, limit int) ListResponse {
	hasMore := offset+limit < total
	return ListResponse{
		Emails:   data,
		Total:    total,
		HasMore:  hasMore,
		PageSize: limit,
	}
}

// =============================================================================
// Query Parameter Helpers
// =============================================================================

// QueryBool parses a boolean query parameter (returns nil if not present)
func QueryBool(c *fiber.Ctx, key string) *bool {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	b := val == "true" || val == "1"
	return &b
}

// QueryString returns pointer to string query param (nil if empty)
func QueryString(c *fiber.Ctx, key string) *string {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	return &val
}

// QueryInt64 returns pointer to int64 query param (nil if 0 or not present)
func QueryInt64(c *fiber.Ctx, key string) *int64 {
	val := c.QueryInt(key, 0)
	if val == 0 {
		return nil
	}
	v := int64(val)
	return &v
}
