package in

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

type OAuthService interface {
	// Get OAuth URL for authorization
	GetAuthURL(ctx context.Context, provider domain.OAuthProvider, state string) (string, error)

	// Handle OAuth callback
	HandleCallback(ctx context.Context, provider domain.OAuthProvider, code string, userID uuid.UUID) (*domain.OAuthConnection, error)

	// Get connections
	GetConnection(ctx context.Context, connectionID int64) (*domain.OAuthConnection, error)
	GetConnectionsByUser(ctx context.Context, userID uuid.UUID) ([]*domain.OAuthConnection, error)
	GetDefaultConnection(ctx context.Context, userID uuid.UUID) (*domain.OAuthConnection, error)

	// Set default connection for sending emails
	SetDefaultConnection(ctx context.Context, userID uuid.UUID, connectionID int64) error

	// Disconnect
	Disconnect(ctx context.Context, connectionID int64) error

	// Token management
	RefreshToken(ctx context.Context, connectionID int64) error
	GetValidToken(ctx context.Context, connectionID int64) (string, error)
}
