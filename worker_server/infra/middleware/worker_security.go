package middleware

import (
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// SecurityHeaders adds security headers to all responses
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Prevent MIME type sniffing
		c.Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		c.Set("X-Frame-Options", "DENY")

		// Enable XSS filter
		c.Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		c.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

		// Permissions Policy (disable unnecessary browser features)
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Strict Transport Security (enable HTTPS enforcement)
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Remove server header
		c.Set("Server", "")

		return c.Next()
	}
}

// InputSanitizer sanitizes common injection patterns
func InputSanitizer() fiber.Handler {
	// Enhanced SQL injection patterns
	sqlInjectionPatterns := regexp.MustCompile(`(?i)(` +
		`union\s+(all\s+)?select|` +
		`insert\s+into|` +
		`drop\s+(table|database|index)|` +
		`delete\s+from|` +
		`update\s+\w+\s+set|` +
		`truncate\s+table|` +
		`alter\s+table|` +
		`create\s+(table|database|index)|` +
		`exec(\s+|\()|` +
		`execute(\s+|\()|` +
		`xp_|sp_|` +
		`;\s*--|` +
		`'\s*(or|and)\s*'|` +
		`"\s*(or|and)\s*"|` +
		`'\s*(or|and)\s+\d|` +
		`\d\s*(or|and)\s*'|` +
		`--\s*$|` +
		`/\*.*\*/|` +
		`benchmark\s*\(|` +
		`sleep\s*\(|` +
		`waitfor\s+delay|` +
		`load_file\s*\(|` +
		`into\s+(out|dump)file)`)

	// Enhanced XSS patterns
	// Note: on\w+= pattern is too broad and matches legitimate params like "connection_id="
	// Use specific event handler patterns instead
	xssPatterns := regexp.MustCompile(`(?i)(` +
		`<script|` +
		`javascript\s*:|` +
		`vbscript\s*:|` +
		`\bon(click|load|error|mouse\w+|key\w+|focus|blur|change|submit|reset|select|abort|unload)\s*=|` +
		`<iframe|` +
		`<object|` +
		`<embed|` +
		`<svg\s|` +
		`<img[^>]+onerror|` +
		`<body[^>]+onload|` +
		`expression\s*\(|` +
		`url\s*\(\s*['"]?\s*data:|` +
		`<link[^>]+href\s*=|` +
		`<meta[^>]+http-equiv)`)

	// Command injection patterns
	cmdInjectionPatterns := regexp.MustCompile(`(?i)(` +
		`;\s*\w+|` +
		`\|\s*\w+|` +
		`\$\(|` +
		"\\x60|" + // backtick
		`>\s*/|` +
		`<\s*/|` +
		`&&\s*\w+|` +
		`\|\|\s*\w+)`)

	return func(c *fiber.Ctx) error {
		// Check query parameters
		queryString := string(c.Request().URI().QueryString())
		if sqlInjectionPatterns.MatchString(queryString) {
			logSuspiciousRequest(c, "sql_injection", queryString)
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid request parameters",
				"code":  "SQL_INJECTION_BLOCKED",
			})
		}
		if xssPatterns.MatchString(queryString) {
			logSuspiciousRequest(c, "xss", queryString)
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid request parameters",
				"code":  "XSS_BLOCKED",
			})
		}

		// Check path parameters
		path := c.Path()
		if xssPatterns.MatchString(path) || cmdInjectionPatterns.MatchString(path) {
			logSuspiciousRequest(c, "path_injection", path)
			return c.Status(400).JSON(fiber.Map{
				"error": "invalid request path",
				"code":  "INVALID_INPUT",
			})
		}

		// Check request body for POST/PUT/PATCH
		if c.Method() == "POST" || c.Method() == "PUT" || c.Method() == "PATCH" {
			body := string(c.Body())
			if len(body) > 0 && len(body) < 100000 { // Only check reasonable sized bodies
				if sqlInjectionPatterns.MatchString(body) {
					logSuspiciousRequest(c, "sql_injection_body", body[:min(500, len(body))])
					return c.Status(400).JSON(fiber.Map{
						"error": "invalid request body",
						"code":  "SQL_INJECTION_BLOCKED",
					})
				}
			}
		}

		return c.Next()
	}
}

// logSuspiciousRequest logs potential attack attempts
func logSuspiciousRequest(c *fiber.Ctx, attackType, payload string) {
	// Truncate payload for logging
	if len(payload) > 200 {
		payload = payload[:200] + "..."
	}

	// Log to standard logger (could also send to security monitoring)
	// Using fmt to avoid import cycle with logger package
	// In production, this should use proper structured logging
	_ = attackType
	_ = payload
	// logger.Warn("Suspicious request blocked: type=%s ip=%s path=%s payload=%s",
	//     attackType, c.IP(), c.Path(), payload)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ValidateContentType ensures requests have appropriate content types
func ValidateContentType() fiber.Handler {
	return func(c *fiber.Ctx) error {
		method := c.Method()

		// Only validate POST, PUT, PATCH requests with body
		if method == "POST" || method == "PUT" || method == "PATCH" {
			contentType := c.Get("Content-Type")
			bodyLen := len(c.Body())

			// If there's a body, content type should be set
			if bodyLen > 0 {
				if contentType == "" {
					return c.Status(400).JSON(fiber.Map{
						"error": "content-type header required",
						"code":  "MISSING_CONTENT_TYPE",
					})
				}

				// Allow only specific content types
				allowedTypes := []string{
					"application/json",
					"application/x-www-form-urlencoded",
					"multipart/form-data",
				}

				valid := false
				for _, t := range allowedTypes {
					if strings.HasPrefix(contentType, t) {
						valid = true
						break
					}
				}

				if !valid {
					return c.Status(415).JSON(fiber.Map{
						"error": "unsupported content type",
						"code":  "UNSUPPORTED_MEDIA_TYPE",
					})
				}
			}
		}

		return c.Next()
	}
}

// MaxBodySize limits request body size for specific endpoints
func MaxBodySize(maxBytes int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if len(c.Body()) > maxBytes {
			return c.Status(413).JSON(fiber.Map{
				"error":    "request body too large",
				"code":     "PAYLOAD_TOO_LARGE",
				"max_size": maxBytes,
			})
		}
		return c.Next()
	}
}

// IPWhitelist allows only specific IPs
func IPWhitelist(allowedIPs []string) fiber.Handler {
	ipSet := make(map[string]bool)
	for _, ip := range allowedIPs {
		ipSet[ip] = true
	}

	return func(c *fiber.Ctx) error {
		clientIP := c.IP()
		if !ipSet[clientIP] {
			return c.Status(403).JSON(fiber.Map{
				"error": "access denied",
				"code":  "IP_NOT_ALLOWED",
			})
		}
		return c.Next()
	}
}
