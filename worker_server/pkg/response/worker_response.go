// Package response provides optimized API response utilities.
package response

import (
	"reflect"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// =============================================================================
// Standard API Response
// =============================================================================

// Response is the standard API response structure.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// ErrorInfo contains error details.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Meta contains pagination and other metadata.
type Meta struct {
	Total    int    `json:"total,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	HasMore  bool   `json:"has_more,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// =============================================================================
// Response Builders
// =============================================================================

// OK returns a successful response.
func OK(c *fiber.Ctx, data interface{}) error {
	return c.JSON(Response{
		Success: true,
		Data:    data,
	})
}

// OKWithMeta returns a successful response with metadata.
func OKWithMeta(c *fiber.Ctx, data interface{}, meta *Meta) error {
	return c.JSON(Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Created returns a 201 created response.
func Created(c *fiber.Ctx, data interface{}) error {
	return c.Status(201).JSON(Response{
		Success: true,
		Data:    data,
	})
}

// NoContent returns a 204 no content response.
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(204)
}

// Error returns an error response.
func Error(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	})
}

// BadRequest returns a 400 bad request response.
func BadRequest(c *fiber.Ctx, message string) error {
	return Error(c, 400, "BAD_REQUEST", message)
}

// Unauthorized returns a 401 unauthorized response.
func Unauthorized(c *fiber.Ctx, message string) error {
	return Error(c, 401, "UNAUTHORIZED", message)
}

// NotFound returns a 404 not found response.
func NotFound(c *fiber.Ctx, message string) error {
	return Error(c, 404, "NOT_FOUND", message)
}

// InternalError returns a 500 internal server error response.
func InternalError(c *fiber.Ctx, message string) error {
	return Error(c, 500, "INTERNAL_ERROR", message)
}

// =============================================================================
// Field Selection (Sparse Fieldsets)
// =============================================================================

// SelectFields filters struct fields based on query parameter.
// Usage: GET /api/emails?fields=id,subject,from_email
func SelectFields(c *fiber.Ctx, data interface{}) interface{} {
	fieldsParam := c.Query("fields")
	if fieldsParam == "" {
		return data
	}

	fields := strings.Split(fieldsParam, ",")
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[strings.TrimSpace(strings.ToLower(f))] = true
	}

	return filterFields(data, fieldSet)
}

func filterFields(data interface{}, fields map[string]bool) interface{} {
	if data == nil {
		return nil
	}

	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Slice:
		result := make([]map[string]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			result[i] = filterStructFields(v.Index(i), fields)
		}
		return result

	case reflect.Struct:
		return filterStructFields(v, fields)

	default:
		return data
	}
}

func filterStructFields(v reflect.Value, fields map[string]bool) map[string]interface{} {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	t := v.Type()
	result := make(map[string]interface{})

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)

		// Get JSON tag name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		jsonName := strings.Split(jsonTag, ",")[0]

		// Check if field is requested
		if fields[strings.ToLower(jsonName)] {
			result[jsonName] = v.Field(i).Interface()
		}
	}

	return result
}

// =============================================================================
// Pagination Helper
// =============================================================================

// PaginationParams extracts pagination parameters from request.
type PaginationParams struct {
	Page     int
	PageSize int
	Offset   int
	Limit    int
	Cursor   string
}

// GetPagination extracts pagination params from request.
func GetPagination(c *fiber.Ctx, defaultPageSize, maxPageSize int) *PaginationParams {
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}

	pageSize := c.QueryInt("page_size", defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	// Also support limit/offset style
	limit := c.QueryInt("limit", pageSize)
	if limit > maxPageSize {
		limit = maxPageSize
	}

	offset := c.QueryInt("offset", (page-1)*pageSize)

	return &PaginationParams{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
		Limit:    limit,
		Cursor:   c.Query("cursor"),
	}
}
