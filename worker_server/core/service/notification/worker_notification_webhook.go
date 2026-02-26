package notification

import (
	"context"
	"fmt"
	"log"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// EmailProvider interface for webhook operations.
type EmailProvider interface {
	Watch(ctx context.Context, token *oauth2.Token) (*out.ProviderWatchResponse, error)
	StopWatch(ctx context.Context, token *oauth2.Token) error
}

// WebhookService manages webhook subscriptions.
type WebhookService struct {
	webhookRepo   domain.WebhookRepository
	oauthService  *auth.OAuthService
	gmailProvider EmailProvider
}

// NewWebhookService creates a new webhook service.
func NewWebhookService(
	webhookRepo domain.WebhookRepository,
	oauthService *auth.OAuthService,
	gmailProvider EmailProvider,
) *WebhookService {
	return &WebhookService{
		webhookRepo:   webhookRepo,
		oauthService:  oauthService,
		gmailProvider: gmailProvider,
	}
}

// SetupWatch sets up a webhook for a connection.
func (s *WebhookService) SetupWatch(ctx context.Context, connectionID int64) (*domain.WebhookConfig, error) {
	if s.webhookRepo == nil || s.oauthService == nil {
		return nil, fmt.Errorf("service not initialized")
	}

	// Get connection
	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Check if webhook already exists
	existing, _ := s.webhookRepo.GetByConnectionID(connectionID)
	if existing != nil && existing.Status == domain.WebhookStatusActive && !existing.IsExpired() {
		return existing, nil
	}

	// Setup watch based on provider
	var watchResp *out.ProviderWatchResponse

	switch conn.Provider {
	case "google", "gmail":
		if s.gmailProvider == nil {
			return nil, fmt.Errorf("gmail provider not initialized")
		}
		watchResp, err = s.gmailProvider.Watch(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("failed to setup gmail watch: %w", err)
		}
	case "outlook":
		// TODO: Implement outlook watch
		return nil, fmt.Errorf("outlook watch not implemented")
	default:
		return nil, fmt.Errorf("unsupported provider: %s", conn.Provider)
	}

	// Create or update webhook config
	webhook := &domain.WebhookConfig{
		ConnectionID:   connectionID,
		UserID:         conn.UserID,
		Provider:       string(conn.Provider),
		SubscriptionID: watchResp.ExternalID,
		ResourceID:     watchResp.ExternalID,
		Status:         domain.WebhookStatusActive,
		ExpiresAt:      watchResp.Expiration,
	}

	if existing != nil {
		webhook.ID = existing.ID
		now := time.Now()
		webhook.LastRenewedAt = &now
		if err := s.webhookRepo.Update(webhook); err != nil {
			return nil, fmt.Errorf("failed to update webhook: %w", err)
		}
	} else {
		if err := s.webhookRepo.Create(webhook); err != nil {
			return nil, fmt.Errorf("failed to create webhook: %w", err)
		}
	}

	log.Printf("Webhook setup for connection %d, expires at %v", connectionID, webhook.ExpiresAt)
	return webhook, nil
}

// StopWatch stops a webhook for a connection.
func (s *WebhookService) StopWatch(ctx context.Context, connectionID int64) error {
	if s.webhookRepo == nil || s.oauthService == nil {
		return fmt.Errorf("service not initialized")
	}

	// Get webhook
	webhook, err := s.webhookRepo.GetByConnectionID(connectionID)
	if err != nil {
		return nil // No webhook to stop
	}

	// Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		// Just delete the webhook record
		return s.webhookRepo.Delete(webhook.ID)
	}

	// Stop watch at provider
	switch webhook.Provider {
	case "google", "gmail":
		if s.gmailProvider != nil {
			if err := s.gmailProvider.StopWatch(ctx, token); err != nil {
				log.Printf("Warning: failed to stop gmail watch: %v", err)
			}
		}
	}

	// Delete webhook record
	return s.webhookRepo.Delete(webhook.ID)
}

// RenewExpiring renews webhooks that are about to expire.
func (s *WebhookService) RenewExpiring(ctx context.Context) (int, error) {
	if s.webhookRepo == nil {
		return 0, nil
	}

	// Get webhooks expiring in the next 24 hours
	expiringBefore := time.Now().Add(24 * time.Hour)
	webhooks, err := s.webhookRepo.ListExpiring(expiringBefore)
	if err != nil {
		return 0, fmt.Errorf("failed to list expiring webhooks: %w", err)
	}

	renewed := 0
	for _, webhook := range webhooks {
		_, err := s.SetupWatch(ctx, webhook.ConnectionID)
		if err != nil {
			log.Printf("Failed to renew webhook %d: %v", webhook.ID, err)
			s.webhookRepo.UpdateStatus(webhook.ID, domain.WebhookStatusFailed, err.Error())
			s.webhookRepo.IncrementFailureCount(webhook.ID)
			continue
		}
		renewed++
	}

	log.Printf("Renewed %d/%d expiring webhooks", renewed, len(webhooks))
	return renewed, nil
}

// GetWebhook returns a webhook by ID.
func (s *WebhookService) GetWebhook(ctx context.Context, webhookID int64) (*domain.WebhookConfig, error) {
	if s.webhookRepo == nil {
		return nil, fmt.Errorf("service not initialized")
	}
	return s.webhookRepo.GetByID(webhookID)
}

// GetWebhookByConnection returns a webhook by connection ID.
func (s *WebhookService) GetWebhookByConnection(ctx context.Context, connectionID int64) (*domain.WebhookConfig, error) {
	if s.webhookRepo == nil {
		return nil, fmt.Errorf("service not initialized")
	}
	return s.webhookRepo.GetByConnectionID(connectionID)
}

// ListUserWebhooks returns all webhooks for a user.
func (s *WebhookService) ListUserWebhooks(ctx context.Context, userID uuid.UUID) ([]*domain.WebhookConfig, error) {
	if s.webhookRepo == nil {
		return []*domain.WebhookConfig{}, nil
	}
	return s.webhookRepo.ListByUserID(userID)
}

// HandleWebhookTriggered updates webhook stats when triggered.
func (s *WebhookService) HandleWebhookTriggered(ctx context.Context, webhookID int64) error {
	if s.webhookRepo == nil {
		return nil
	}
	s.webhookRepo.ResetFailureCount(webhookID)
	return s.webhookRepo.UpdateLastTriggered(webhookID)
}

// DisableWebhook disables a webhook.
func (s *WebhookService) DisableWebhook(ctx context.Context, webhookID int64) error {
	if s.webhookRepo == nil {
		return nil
	}
	return s.webhookRepo.UpdateStatus(webhookID, domain.WebhookStatusDisabled, "")
}

// EnableWebhook re-enables a webhook by setting up a new watch.
func (s *WebhookService) EnableWebhook(ctx context.Context, webhookID int64) (*domain.WebhookConfig, error) {
	webhook, err := s.webhookRepo.GetByID(webhookID)
	if err != nil {
		return nil, err
	}
	return s.SetupWatch(ctx, webhook.ConnectionID)
}

// SetupAllConnections sets up webhooks for all active connections that don't have one.
// This should be called on server startup to ensure all existing connections have webhooks.
func (s *WebhookService) SetupAllConnections(ctx context.Context) (int, int, error) {
	if s.oauthService == nil {
		return 0, 0, fmt.Errorf("oauth service not initialized")
	}

	// Get all active connections
	connections, err := s.oauthService.ListAllActiveConnections(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list connections: %w", err)
	}

	success := 0
	failed := 0

	for _, conn := range connections {
		// Skip non-Gmail providers for now
		if conn.Provider != "google" {
			continue
		}

		// Check if webhook already exists and is active
		if s.webhookRepo != nil {
			existing, _ := s.webhookRepo.GetByConnectionID(conn.ID)
			if existing != nil && existing.Status == domain.WebhookStatusActive && !existing.IsExpired() {
				log.Printf("[WebhookService] Connection %d already has active webhook, skipping", conn.ID)
				continue
			}
		}

		// Setup webhook
		_, err := s.SetupWatch(ctx, conn.ID)
		if err != nil {
			log.Printf("[WebhookService] Failed to setup webhook for connection %d: %v", conn.ID, err)
			failed++
			continue
		}

		log.Printf("[WebhookService] Webhook setup for connection %d (%s)", conn.ID, conn.Email)
		success++
	}

	log.Printf("[WebhookService] SetupAllConnections completed: %d success, %d failed, %d total",
		success, failed, len(connections))
	return success, failed, nil
}
