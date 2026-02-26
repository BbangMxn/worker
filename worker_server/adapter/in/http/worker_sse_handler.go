package http

import (
	"bufio"
	"time"

	"worker_server/adapter/out/realtime"
	"worker_server/core/domain"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// =============================================================================
// SSE Handler - RealtimePort 기반
// =============================================================================

// SSEHandler handles Server-Sent Events connections.
type SSEHandler struct {
	hub *realtime.SSEHub
	log zerolog.Logger
}

// NewSSEHandler creates a new SSE handler.
func NewSSEHandler(hub *realtime.SSEHub, log zerolog.Logger) *SSEHandler {
	return &SSEHandler{
		hub: hub,
		log: log.With().Str("handler", "sse").Logger(),
	}
}

// Register registers SSE routes.
func (h *SSEHandler) Register(app fiber.Router) {
	app.Get("/events", h.Stream)
	app.Get("/events/status", h.Status)
}

// Stream handles SSE connections.
func (h *SSEHandler) Stream(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	userIDStr := userID.String()
	client := h.hub.CreateClient(userIDStr)

	h.log.Info().
		Str("user_id", userIDStr).
		Msg("SSE client connected")

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")
	c.Set("X-Accel-Buffering", "no") // Nginx buffering 비활성화

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(client.HeartbeatInterval())
		defer ticker.Stop()
		defer func() {
			client.Close()
			h.log.Info().
				Str("user_id", userIDStr).
				Msg("SSE client disconnected")
		}()

		// Send initial connection event
		w.WriteString("event: connected\n")
		w.WriteString("data: {\"status\":\"connected\"}\n\n")
		w.Flush()

		for {
			select {
			case event, ok := <-client.Events:
				if !ok {
					return
				}

				data, err := realtime.SerializeEvent(event)
				if err != nil {
					h.log.Error().Err(err).Msg("failed to serialize event")
					continue
				}

				// Write SSE format
				w.WriteString("event: ")
				w.WriteString(string(event.Type))
				w.WriteString("\n")
				w.WriteString("data: ")
				w.Write(data)
				w.WriteString("\n\n")

				if err := w.Flush(); err != nil {
					h.log.Debug().Err(err).Msg("client disconnected during write")
					return
				}

			case <-ticker.C:
				// Heartbeat
				w.WriteString(": heartbeat\n\n")
				if err := w.Flush(); err != nil {
					h.log.Debug().Err(err).Msg("client disconnected during heartbeat")
					return
				}

			case <-client.Done:
				return
			}
		}
	})

	return nil
}

// Status returns SSE connection status.
func (h *SSEHandler) Status(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	// Note: This would need access to the adapter for full status
	return c.JSON(fiber.Map{
		"user_id":   userID.String(),
		"connected": true,
	})
}

// =============================================================================
// Legacy SSE Hub (하위 호환성)
// =============================================================================

// LegacySSEHub - 기존 코드와 호환성 유지용
type LegacySSEHub struct {
	realtimeHub *realtime.SSEHub
}

// NewLegacySSEHub creates a legacy SSE hub wrapper.
func NewLegacySSEHub(hub *realtime.SSEHub) *LegacySSEHub {
	return &LegacySSEHub{realtimeHub: hub}
}

// Send sends an event to a user (legacy interface).
func (h *LegacySSEHub) Send(userID uuid.UUID, eventType string, data interface{}) {
	event := &domain.RealtimeEvent{
		Type:      domain.EventType(eventType),
		Data:      data,
		Timestamp: time.Now(),
	}
	h.realtimeHub.Send(userID.String(), event)
}

// ClientCount returns the number of connected clients.
func (h *LegacySSEHub) ClientCount() int {
	// Note: This would need access to the adapter for accurate count
	return 0
}
