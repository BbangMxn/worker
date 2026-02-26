package middleware

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"worker_server/pkg/apperr"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ErrorResponse is the standard error response format
type ErrorResponse struct {
	Success   bool        `json:"success"`
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
	Timestamp string      `json:"timestamp"`
}

type ErrorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ErrorHandler is a centralized error handler for Fiber
func ErrorHandler() fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		requestID, _ := c.Locals("request_id").(string)

		// Default error response
		response := ErrorResponse{
			Success:   false,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		// Handle different error types
		var status int

		switch e := err.(type) {
		case *apperr.AppError:
			status = e.Status
			response.Error = ErrorDetail{
				Code:    e.Code,
				Message: e.Message,
				Details: e.Details,
			}

			// Log application errors
			log := logger.WithField("request_id", requestID).
				WithField("error_code", e.Code).
				WithError(e.Err)

			if status >= 500 {
				log.Error("Internal error: %s", e.Message)
			} else {
				log.Warn("Client error: %s", e.Message)
			}

		case *fiber.Error:
			status = e.Code
			response.Error = ErrorDetail{
				Code:    mapHTTPStatusToCode(e.Code),
				Message: e.Message,
			}

		default:
			status = fiber.StatusInternalServerError
			response.Error = ErrorDetail{
				Code:    apperr.CodeInternalError,
				Message: "An unexpected error occurred",
			}

			// Log unexpected errors with stack trace
			logger.WithField("request_id", requestID).
				WithError(err).
				WithField("stack", string(debug.Stack())).
				Error("Unexpected error: %s", err.Error())
		}

		return c.Status(status).JSON(response)
	}
}

// RequestID middleware adds a unique request ID to each request
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Locals("request_id", requestID)
		c.Set("X-Request-ID", requestID)
		return c.Next()
	}
}

// RequestLogger logs incoming requests and their responses
func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		requestID, _ := c.Locals("request_id").(string)

		// Process request
		err := c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Get user ID if available
		userID := ""
		if uid, ok := c.Locals("user_id").(uuid.UUID); ok {
			userID = uid.String()
		}

		// Build log entry
		log := logger.WithFields(map[string]any{
			"request_id":  requestID,
			"method":      c.Method(),
			"path":        c.Path(),
			"status":      c.Response().StatusCode(),
			"duration_ms": float64(duration.Microseconds()) / 1000.0,
			"ip":          c.IP(),
			"user_agent":  c.Get("User-Agent"),
		})

		if userID != "" {
			log = log.WithField("user_id", userID)
		}

		// Log based on status code
		status := c.Response().StatusCode()
		switch {
		case status >= 500:
			log.Error("Request failed: %s %s -> %d", c.Method(), c.Path(), status)
		case status >= 400:
			log.Warn("Request error: %s %s -> %d", c.Method(), c.Path(), status)
		default:
			log.Info("Request completed: %s %s -> %d", c.Method(), c.Path(), status)
		}

		return err
	}
}

// Recover middleware recovers from panics
func Recover() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				requestID, _ := c.Locals("request_id").(string)
				stack := string(debug.Stack())

				// Log panic with full details to stderr for Railway
				fmt.Fprintf(os.Stderr, "\n=== PANIC RECOVERED ===\n")
				fmt.Fprintf(os.Stderr, "Request ID: %s\n", requestID)
				fmt.Fprintf(os.Stderr, "Path: %s %s\n", c.Method(), c.Path())
				fmt.Fprintf(os.Stderr, "Panic: %v\n", r)
				fmt.Fprintf(os.Stderr, "Stack:\n%s\n", stack)
				fmt.Fprintf(os.Stderr, "=== END PANIC ===\n\n")

				logger.WithFields(map[string]any{
					"request_id": requestID,
					"panic":      fmt.Sprintf("%v", r),
					"path":       c.Path(),
					"method":     c.Method(),
				}).Error("Panic recovered")

				c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Success:   false,
					RequestID: requestID,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Error: ErrorDetail{
						Code:    apperr.CodeInternalError,
						Message: "An unexpected error occurred",
					},
				})
			}
		}()
		return c.Next()
	}
}

func mapHTTPStatusToCode(status int) string {
	switch status {
	case 400:
		return apperr.CodeValidationFailed
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
	case 502, 503, 504:
		return "SERVICE_UNAVAILABLE"
	default:
		return "UNKNOWN_ERROR"
	}
}
