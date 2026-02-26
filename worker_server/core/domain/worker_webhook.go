package domain

import (
	"time"

	"github.com/google/uuid"
)

// WebhookStatus represents the status of a webhook subscription.
type WebhookStatus string

const (
	WebhookStatusActive   WebhookStatus = "active"
	WebhookStatusExpired  WebhookStatus = "expired"
	WebhookStatusFailed   WebhookStatus = "failed"
	WebhookStatusPending  WebhookStatus = "pending"
	WebhookStatusDisabled WebhookStatus = "disabled"
)

// WebhookConfig represents a webhook subscription configuration.
type WebhookConfig struct {
	ID           int64     `json:"id"`
	ConnectionID int64     `json:"connection_id"`
	UserID       uuid.UUID `json:"user_id"`
	Provider     string    `json:"provider"` // gmail, outlook

	// Subscription details
	SubscriptionID string `json:"subscription_id,omitempty"` // External ID from provider
	ResourceID     string `json:"resource_id,omitempty"`     // Gmail: historyId, Outlook: subscriptionId
	ChannelID      string `json:"channel_id,omitempty"`      // Gmail push channel ID

	// Status
	Status       WebhookStatus `json:"status"`
	LastError    string        `json:"last_error,omitempty"`
	FailureCount int           `json:"failure_count"`

	// Timing
	ExpiresAt       time.Time  `json:"expires_at"`
	LastRenewedAt   *time.Time `json:"last_renewed_at,omitempty"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsExpired checks if the webhook is expired.
func (w *WebhookConfig) IsExpired() bool {
	return time.Now().After(w.ExpiresAt)
}

// NeedsRenewal checks if the webhook needs renewal (within 1 day of expiry).
func (w *WebhookConfig) NeedsRenewal() bool {
	return time.Now().Add(24 * time.Hour).After(w.ExpiresAt)
}

// WebhookRepository defines webhook persistence operations.
type WebhookRepository interface {
	// CRUD
	Create(webhook *WebhookConfig) error
	GetByID(id int64) (*WebhookConfig, error)
	GetByConnectionID(connectionID int64) (*WebhookConfig, error)
	GetBySubscriptionID(subscriptionID string) (*WebhookConfig, error)
	Update(webhook *WebhookConfig) error
	Delete(id int64) error
	DeleteByConnectionID(connectionID int64) error

	// Queries
	ListByUserID(userID uuid.UUID) ([]*WebhookConfig, error)
	ListExpiring(before time.Time) ([]*WebhookConfig, error)
	ListByStatus(status WebhookStatus) ([]*WebhookConfig, error)
	ListActive() ([]*WebhookConfig, error)

	// Status updates
	UpdateStatus(id int64, status WebhookStatus, lastError string) error
	UpdateExpiration(id int64, expiresAt time.Time) error
	IncrementFailureCount(id int64) error
	ResetFailureCount(id int64) error
	UpdateLastTriggered(id int64) error
}
