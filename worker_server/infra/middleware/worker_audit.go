package middleware

import (
	"context"
	"time"

	"github.com/goccy/go-json"

	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// AuditEvent represents a security audit event
type AuditEvent struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	UserID      string    `json:"user_id,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	Action      string    `json:"action"`
	Resource    string    `json:"resource"`
	ResourceID  string    `json:"resource_id,omitempty"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	StatusCode  int       `json:"status_code"`
	Duration    int64     `json:"duration_ms"`
	RequestID   string    `json:"request_id"`
	Success     bool      `json:"success"`
	ErrorDetail string    `json:"error_detail,omitempty"`
	Metadata    any       `json:"metadata,omitempty"`
}

// AuditLogger handles audit logging
type AuditLogger struct {
	redis   *redis.Client
	stream  string
	enabled bool
}

var auditLogger *AuditLogger

// InitAuditLogger initializes the audit logger
func InitAuditLogger(redisClient *redis.Client) {
	if redisClient == nil {
		logger.Warn("Redis client not provided, audit logging disabled")
		auditLogger = &AuditLogger{enabled: false}
		return
	}
	auditLogger = &AuditLogger{
		redis:   redisClient,
		stream:  "audit:events",
		enabled: true,
	}
	logger.Info("Audit logger initialized")
}

// LogAuditEvent logs an audit event to Redis stream
func LogAuditEvent(ctx context.Context, event *AuditEvent) error {
	if auditLogger == nil || !auditLogger.enabled {
		return nil
	}

	event.ID = uuid.NewString()
	event.Timestamp = time.Now()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return auditLogger.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: auditLogger.stream,
		Values: map[string]interface{}{
			"event": string(data),
		},
		MaxLen: 100000, // Keep last 100k events
		Approx: true,
	}).Err()
}

// SensitiveActions defines actions that require audit logging
var SensitiveActions = map[string]string{
	"POST:/api/v1/oauth/connect":   "oauth_connect",
	"DELETE:/api/v1/oauth":         "oauth_disconnect",
	"POST:/api/v1/email/send":       "email_send",
	"POST:/api/v1/email/reply":      "email_reply",
	"DELETE:/api/v1/email":          "email_delete",
	"POST:/api/v1/calendar/events": "calendar_create",
	"PUT:/api/v1/calendar/events":  "calendar_update",
	"DELETE:/api/v1/calendar":      "calendar_delete",
	"POST:/api/v1/contacts":        "contact_create",
	"PUT:/api/v1/contacts":         "contact_update",
	"DELETE:/api/v1/contacts":      "contact_delete",
	"PUT:/api/v1/settings":         "settings_update",
	"POST:/api/v1/ai/chat":         "ai_chat",
	"POST:/api/v1/ai/proposal":     "ai_proposal_confirm",
}

// AuditMiddleware logs sensitive actions for security audit
func AuditMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		method := c.Method()
		path := c.Path()

		// Check if this is a sensitive action
		actionKey := method + ":" + path
		action := ""

		// Match with prefix for parameterized routes
		for key, act := range SensitiveActions {
			if matchesAuditPattern(actionKey, key) {
				action = act
				break
			}
		}

		// Process request
		err := c.Next()

		// Only log sensitive actions
		if action != "" {
			event := &AuditEvent{
				Action:     action,
				Resource:   extractResource(path),
				ResourceID: extractResourceID(c),
				Method:     method,
				Path:       path,
				IP:         c.IP(),
				UserAgent:  c.Get("User-Agent"),
				StatusCode: c.Response().StatusCode(),
				Duration:   time.Since(start).Milliseconds(),
				RequestID:  c.GetRespHeader("X-Request-ID"),
				Success:    c.Response().StatusCode() < 400,
			}

			// Add user info if authenticated
			if userID, ok := c.Locals("user_id").(uuid.UUID); ok {
				event.UserID = userID.String()
			}
			if sessionID, ok := c.Locals("session_id").(string); ok {
				event.SessionID = sessionID
			}

			// Log error detail for failed requests
			if !event.Success && err != nil {
				event.ErrorDetail = err.Error()
			}

			// Async logging to not block response
			go func() {
				if logErr := LogAuditEvent(context.Background(), event); logErr != nil {
					logger.WithError(logErr).Warn("Failed to log audit event")
				}
			}()
		}

		return err
	}
}

// matchesAuditPattern checks if a path matches an audit pattern
func matchesAuditPattern(path, pattern string) bool {
	// Simple prefix matching for now
	if len(path) < len(pattern) {
		return false
	}
	return path[:len(pattern)] == pattern
}

// extractResource extracts the resource type from path
func extractResource(path string) string {
	parts := splitPath(path)
	if len(parts) >= 4 {
		return parts[3] // /api/v1/{resource}
	}
	return ""
}

// extractResourceID extracts the resource ID from path or body
func extractResourceID(c *fiber.Ctx) string {
	// Try path param
	if id := c.Params("id"); id != "" {
		return id
	}
	if id := c.Params("eventId"); id != "" {
		return id
	}
	if id := c.Params("contactId"); id != "" {
		return id
	}
	return ""
}

// splitPath splits a path into segments
func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, ch := range path {
		if ch == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// SecurityEventLogger logs security-related events
func LogSecurityEvent(ctx context.Context, eventType string, userID string, details map[string]any) {
	if auditLogger == nil || !auditLogger.enabled {
		return
	}

	event := &AuditEvent{
		ID:        uuid.NewString(),
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    eventType,
		Resource:  "security",
		Metadata:  details,
		Success:   true,
	}

	if logErr := LogAuditEvent(ctx, event); logErr != nil {
		logger.WithError(logErr).Warn("Failed to log security event")
	}
}
