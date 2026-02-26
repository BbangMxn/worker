package out

import (
	"context"
	"time"
)

// OAuthRepository defines the outbound port for OAuth persistence.
type OAuthRepository interface {
	// ListByUser returns all connections for a user.
	ListByUser(ctx context.Context, userID string) ([]*OAuthConnectionEntity, error)

	// ListAllActive returns all active connections (for webhook setup on startup).
	ListAllActive(ctx context.Context) ([]*OAuthConnectionEntity, error)

	// GetByID returns a connection by ID.
	GetByID(ctx context.Context, id int64) (*OAuthConnectionEntity, error)

	// GetByEmail returns a connection by user, provider and email.
	GetByEmail(ctx context.Context, userID, provider, email string) (*OAuthConnectionEntity, error)

	// GetByEmailOnly returns a connection by email and provider (without user ID).
	GetByEmailOnly(ctx context.Context, email, provider string) (*OAuthConnectionEntity, error)

	// GetByWebhookID returns a connection by webhook subscription ID.
	GetByWebhookID(ctx context.Context, subscriptionID, provider string) (*OAuthConnectionEntity, error)

	// Create creates a new connection.
	Create(ctx context.Context, entity *OAuthConnectionEntity) error

	// Update updates an existing connection.
	Update(ctx context.Context, entity *OAuthConnectionEntity) error

	// Disconnect marks a connection as disconnected.
	Disconnect(ctx context.Context, id int64) error

	// Delete removes a connection.
	Delete(ctx context.Context, id int64) error

	// SetDefault sets or clears the default flag on a connection.
	SetDefault(ctx context.Context, id int64, isDefault bool) error
}

// OAuthConnectionEntity represents an OAuth connection in persistence.
type OAuthConnectionEntity struct {
	ID           int64      `db:"id"`
	UserID       string     `db:"user_id"`
	Provider     string     `db:"provider"`
	Email        string     `db:"email"`
	AccessToken  string     `db:"access_token"`
	RefreshToken string     `db:"refresh_token"`
	ExpiresAt    time.Time  `db:"expires_at"`
	IsConnected  bool       `db:"is_connected"`
	IsDefault    bool       `db:"is_default"`
	Signature    *string    `db:"signature"`
	LastSyncAt   *time.Time `db:"last_sync_at"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
}
