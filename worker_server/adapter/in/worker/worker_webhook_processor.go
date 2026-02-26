package worker

import (
	"context"
	"fmt"

	"worker_server/core/service/notification"
	"worker_server/pkg/logger"
)

// WebhookProcessor processes webhook-related jobs.
type WebhookProcessor struct {
	webhookService *notification.WebhookService
}

// NewWebhookProcessor creates a new webhook processor.
func NewWebhookProcessor(webhookService *notification.WebhookService) *WebhookProcessor {
	return &WebhookProcessor{
		webhookService: webhookService,
	}
}

// ProcessRenew handles webhook renewal jobs.
func (p *WebhookProcessor) ProcessRenew(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[WebhookRenewPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	if p.webhookService == nil {
		return fmt.Errorf("webhook service not initialized")
	}

	// Renew all expiring webhooks
	if payload.RenewAll {
		renewed, err := p.webhookService.RenewExpiring(ctx)
		if err != nil {
			return fmt.Errorf("failed to renew expiring webhooks: %w", err)
		}
		logger.Info("Renewed %d webhooks", renewed)
		return nil
	}

	// Renew specific webhook by connection ID
	if payload.ConnectionID > 0 {
		_, err := p.webhookService.SetupWatch(ctx, payload.ConnectionID)
		if err != nil {
			return fmt.Errorf("failed to renew webhook for connection %d: %w", payload.ConnectionID, err)
		}
		logger.Info("Renewed webhook for connection %d", payload.ConnectionID)
		return nil
	}

	// Renew specific webhook by ID
	if payload.WebhookID > 0 {
		_, err := p.webhookService.EnableWebhook(ctx, payload.WebhookID)
		if err != nil {
			return fmt.Errorf("failed to renew webhook %d: %w", payload.WebhookID, err)
		}
		logger.Info("Renewed webhook %d", payload.WebhookID)
		return nil
	}

	return fmt.Errorf("no webhook specified for renewal")
}
