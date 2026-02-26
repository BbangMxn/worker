// Package realtime provides real-time communication adapters.
package realtime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/rs/zerolog"
)

// =============================================================================
// SSE Adapter - RealtimePort 구현
// =============================================================================

// SSEAdapter implements out.RealtimePort using Server-Sent Events.
type SSEAdapter struct {
	clients map[string]map[chan *domain.RealtimeEvent]struct{} // userID -> channels
	mu      sync.RWMutex
	log     zerolog.Logger

	// Metrics
	messagesSent    int64
	messagesDropped int64
	seqCounter      int64 // 전역 시퀀스 카운터
}

// NewSSEAdapter creates a new SSE adapter.
func NewSSEAdapter(log zerolog.Logger) *SSEAdapter {
	return &SSEAdapter{
		clients: make(map[string]map[chan *domain.RealtimeEvent]struct{}),
		log:     log.With().Str("component", "sse_adapter").Logger(),
	}
}

// Subscribe creates a new subscription channel for a user.
func (a *SSEAdapter) Subscribe(userID string) <-chan *domain.RealtimeEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	ch := make(chan *domain.RealtimeEvent, 256) // Buffer for backpressure

	if a.clients[userID] == nil {
		a.clients[userID] = make(map[chan *domain.RealtimeEvent]struct{})
	}
	a.clients[userID][ch] = struct{}{}

	a.log.Debug().
		Str("user_id", userID).
		Int("total_connections", len(a.clients[userID])).
		Msg("client subscribed")

	return ch
}

// Unsubscribe removes a subscription channel.
func (a *SSEAdapter) Unsubscribe(userID string, ch <-chan *domain.RealtimeEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if channels, ok := a.clients[userID]; ok {
		// Find and remove the channel
		for c := range channels {
			if c == ch {
				delete(channels, c)
				close(c)
				break
			}
		}

		// Clean up empty user entry
		if len(channels) == 0 {
			delete(a.clients, userID)
		}
	}

	a.log.Debug().
		Str("user_id", userID).
		Msg("client unsubscribed")
}

// Push sends an event to a specific user.
func (a *SSEAdapter) Push(ctx context.Context, userID string, event *domain.RealtimeEvent) error {
	// 시퀀스 번호 할당 (atomic - 순서 보장)
	event.Seq = atomic.AddInt64(&a.seqCounter, 1)

	a.mu.RLock()
	channels, ok := a.clients[userID]
	if !ok || len(channels) == 0 {
		a.mu.RUnlock()
		return nil // No active connections
	}

	// Copy channels to avoid holding lock during send
	chList := make([]chan *domain.RealtimeEvent, 0, len(channels))
	for ch := range channels {
		chList = append(chList, ch)
	}
	a.mu.RUnlock()

	// Send to all user's connections
	for _, ch := range chList {
		select {
		case ch <- event:
			a.messagesSent++
		default:
			// Channel full, drop message (backpressure)
			a.messagesDropped++
			a.log.Warn().
				Str("user_id", userID).
				Str("event_type", string(event.Type)).
				Int64("seq", event.Seq).
				Msg("dropped event due to full buffer")
		}
	}

	return nil
}

// Broadcast sends an event to all connected users.
func (a *SSEAdapter) Broadcast(ctx context.Context, event *domain.RealtimeEvent) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for userID, channels := range a.clients {
		for ch := range channels {
			select {
			case ch <- event:
				a.messagesSent++
			default:
				a.messagesDropped++
				a.log.Warn().
					Str("user_id", userID).
					Str("event_type", string(event.Type)).
					Msg("dropped broadcast event")
			}
		}
	}

	return nil
}

// ConnectedCount returns the number of connected users.
func (a *SSEAdapter) ConnectedCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.clients)
}

// IsConnected checks if a user has active connections.
func (a *SSEAdapter) IsConnected(userID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	channels, ok := a.clients[userID]
	return ok && len(channels) > 0
}

// GetMetrics returns adapter metrics.
func (a *SSEAdapter) GetMetrics() SSEMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	totalConnections := 0
	for _, channels := range a.clients {
		totalConnections += len(channels)
	}

	return SSEMetrics{
		ConnectedUsers:   len(a.clients),
		TotalConnections: totalConnections,
		MessagesSent:     a.messagesSent,
		MessagesDropped:  a.messagesDropped,
	}
}

// SSEMetrics holds SSE adapter metrics.
type SSEMetrics struct {
	ConnectedUsers   int   `json:"connected_users"`
	TotalConnections int   `json:"total_connections"`
	MessagesSent     int64 `json:"messages_sent"`
	MessagesDropped  int64 `json:"messages_dropped"`
}

// =============================================================================
// SSE Hub - HTTP Handler 연결용
// =============================================================================

// SSEHub manages SSE connections for HTTP handlers.
type SSEHub struct {
	adapter *SSEAdapter
	log     zerolog.Logger

	// Heartbeat
	heartbeatInterval time.Duration
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(adapter *SSEAdapter, log zerolog.Logger) *SSEHub {
	return &SSEHub{
		adapter:           adapter,
		log:               log.With().Str("component", "sse_hub").Logger(),
		heartbeatInterval: 30 * time.Second,
	}
}

// CreateClient creates a new SSE client for a user.
func (h *SSEHub) CreateClient(userID string) *SSEClient {
	eventCh := h.adapter.Subscribe(userID)

	return &SSEClient{
		UserID: userID,
		Events: eventCh,
		Done:   make(chan struct{}),
		hub:    h,
	}
}

// RemoveClient removes an SSE client.
func (h *SSEHub) RemoveClient(client *SSEClient) {
	h.adapter.Unsubscribe(client.UserID, client.Events)
}

// Send sends an event to a specific user (convenience method).
func (h *SSEHub) Send(userID string, event *domain.RealtimeEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.adapter.Push(ctx, userID, event)
}

// SSEClient represents an SSE client connection.
type SSEClient struct {
	UserID string
	Events <-chan *domain.RealtimeEvent
	Done   chan struct{}
	hub    *SSEHub
}

// Close closes the client connection.
func (c *SSEClient) Close() {
	close(c.Done)
	c.hub.RemoveClient(c)
}

// HeartbeatInterval returns the heartbeat interval.
func (c *SSEClient) HeartbeatInterval() time.Duration {
	return c.hub.heartbeatInterval
}

// =============================================================================
// Event Serialization
// =============================================================================

// SerializeEvent converts a RealtimeEvent to SSE format.
func SerializeEvent(event *domain.RealtimeEvent) ([]byte, error) {
	payload := map[string]interface{}{
		"type":      event.Type,
		"data":      event.Data,
		"timestamp": event.Timestamp.Format(time.RFC3339),
	}
	return json.Marshal(payload)
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.RealtimePort = (*SSEAdapter)(nil)
