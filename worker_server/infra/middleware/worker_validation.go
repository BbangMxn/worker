package middleware

import (
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidateUUID validates that a parameter is a valid UUID
func ValidateUUID(paramName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		value := c.Params(paramName)
		if value == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "missing required parameter",
				"code":  "MISSING_PARAM",
				"field": paramName,
			})
		}

		if _, err := uuid.Parse(value); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid UUID format",
				"code":  "INVALID_UUID",
				"field": paramName,
			})
		}

		return c.Next()
	}
}

// ValidateEmail validates that a field contains a valid email address
func ValidateEmail(fieldName string) fiber.Handler {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	return func(c *fiber.Ctx) error {
		// Try to get from query, then body
		email := c.Query(fieldName)
		if email == "" {
			// Check body for JSON
			var body map[string]any
			if err := c.BodyParser(&body); err == nil {
				if e, ok := body[fieldName].(string); ok {
					email = e
				}
			}
		}

		if email != "" && !emailRegex.MatchString(email) {
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid email format",
				"code":  "INVALID_EMAIL",
				"field": fieldName,
			})
		}

		return c.Next()
	}
}

// ValidateRequired ensures required fields are present in the request body
func ValidateRequired(fields ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid request body",
				"code":  "INVALID_BODY",
			})
		}

		var missing []string
		for _, field := range fields {
			if val, exists := body[field]; !exists || val == nil || val == "" {
				missing = append(missing, field)
			}
		}

		if len(missing) > 0 {
			return c.Status(400).JSON(fiber.Map{
				"error":  "missing required fields",
				"code":   "MISSING_FIELDS",
				"fields": missing,
			})
		}

		return c.Next()
	}
}

// ValidateStringLength validates string length bounds
func ValidateStringLength(fieldName string, minLen, maxLen int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Try query first
		value := c.Query(fieldName)

		// Then try body
		if value == "" {
			var body map[string]any
			if err := c.BodyParser(&body); err == nil {
				if v, ok := body[fieldName].(string); ok {
					value = v
				}
			}
		}

		if value != "" {
			length := len(value)
			if length < minLen || length > maxLen {
				return c.Status(400).JSON(fiber.Map{
					"error":  "string length out of bounds",
					"code":   "INVALID_LENGTH",
					"field":  fieldName,
					"min":    minLen,
					"max":    maxLen,
					"actual": length,
				})
			}
		}

		return c.Next()
	}
}

// ValidateEnum validates that a value is one of allowed values
func ValidateEnum(fieldName string, allowedValues []string) fiber.Handler {
	allowed := make(map[string]bool)
	for _, v := range allowedValues {
		allowed[strings.ToLower(v)] = true
	}

	return func(c *fiber.Ctx) error {
		// Try query first
		value := c.Query(fieldName)

		// Then try params
		if value == "" {
			value = c.Params(fieldName)
		}

		// Then try body
		if value == "" {
			var body map[string]any
			if err := c.BodyParser(&body); err == nil {
				if v, ok := body[fieldName].(string); ok {
					value = v
				}
			}
		}

		if value != "" && !allowed[strings.ToLower(value)] {
			return c.Status(400).JSON(fiber.Map{
				"error":   "invalid enum value",
				"code":    "INVALID_ENUM",
				"field":   fieldName,
				"value":   value,
				"allowed": allowedValues,
			})
		}

		return c.Next()
	}
}

// ValidateIntRange validates integer parameters are within range
func ValidateIntRange(paramName string, min, max int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		value, err := c.ParamsInt(paramName)
		if err != nil {
			// Try query
			value = c.QueryInt(paramName, -1)
			if value == -1 {
				return c.Next() // Optional parameter not provided
			}
		}

		if value < min || value > max {
			return c.Status(400).JSON(fiber.Map{
				"error": "value out of range",
				"code":  "OUT_OF_RANGE",
				"field": paramName,
				"min":   min,
				"max":   max,
				"value": value,
			})
		}

		return c.Next()
	}
}

// SanitizeInput removes potentially dangerous characters from input
func SanitizeInput() fiber.Handler {
	// Characters that could be used for injection
	dangerousChars := regexp.MustCompile(`[<>'";&|$\x00-\x1f]`)

	return func(c *fiber.Ctx) error {
		// Sanitize query parameters
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			sanitized := dangerousChars.ReplaceAll(value, []byte(""))
			c.Request().URI().QueryArgs().Set(string(key), string(sanitized))
		})

		return c.Next()
	}
}

// PreventPathTraversal blocks path traversal attempts
func PreventPathTraversal() fiber.Handler {
	traversalPatterns := []string{
		"..",
		"..%2f",
		"..%5c",
		"%2e%2e",
		"..\\",
	}

	return func(c *fiber.Ctx) error {
		path := strings.ToLower(c.Path())

		for _, pattern := range traversalPatterns {
			if strings.Contains(path, pattern) {
				return c.Status(400).JSON(fiber.Map{
					"error": "invalid path",
					"code":  "PATH_TRAVERSAL_BLOCKED",
				})
			}
		}

		return c.Next()
	}
}
