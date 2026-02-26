package persistence

import (
	"database/sql"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// WebhookAdapter implements domain.WebhookRepository using PostgreSQL.
type WebhookAdapter struct {
	db *sqlx.DB
}

// NewWebhookAdapter creates a new webhook adapter.
func NewWebhookAdapter(db *sqlx.DB) *WebhookAdapter {
	return &WebhookAdapter{db: db}
}

// webhookRow represents the database row.
type webhookRow struct {
	ID              int64          `db:"id"`
	ConnectionID    int64          `db:"connection_id"`
	UserID          uuid.UUID      `db:"user_id"`
	Provider        string         `db:"provider"`
	SubscriptionID  sql.NullString `db:"subscription_id"`
	ResourceID      sql.NullString `db:"resource_id"`
	ChannelID       sql.NullString `db:"channel_id"`
	Status          string         `db:"status"`
	LastError       sql.NullString `db:"last_error"`
	FailureCount    int            `db:"failure_count"`
	ExpiresAt       time.Time      `db:"expires_at"`
	LastRenewedAt   sql.NullTime   `db:"last_renewed_at"`
	LastTriggeredAt sql.NullTime   `db:"last_triggered_at"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

func (r *webhookRow) toDomain() *domain.WebhookConfig {
	w := &domain.WebhookConfig{
		ID:           r.ID,
		ConnectionID: r.ConnectionID,
		UserID:       r.UserID,
		Provider:     r.Provider,
		Status:       domain.WebhookStatus(r.Status),
		FailureCount: r.FailureCount,
		ExpiresAt:    r.ExpiresAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}

	if r.SubscriptionID.Valid {
		w.SubscriptionID = r.SubscriptionID.String
	}
	if r.ResourceID.Valid {
		w.ResourceID = r.ResourceID.String
	}
	if r.ChannelID.Valid {
		w.ChannelID = r.ChannelID.String
	}
	if r.LastError.Valid {
		w.LastError = r.LastError.String
	}
	if r.LastRenewedAt.Valid {
		w.LastRenewedAt = &r.LastRenewedAt.Time
	}
	if r.LastTriggeredAt.Valid {
		w.LastTriggeredAt = &r.LastTriggeredAt.Time
	}

	return w
}

// Create creates a new webhook config (upsert - updates if exists).
func (a *WebhookAdapter) Create(webhook *domain.WebhookConfig) error {
	query := `
		INSERT INTO webhook_configs (
			connection_id, user_id, provider, subscription_id, resource_id, channel_id,
			status, last_error, failure_count, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (connection_id, resource_type) DO UPDATE SET
			subscription_id = EXCLUDED.subscription_id,
			resource_id = EXCLUDED.resource_id,
			channel_id = EXCLUDED.channel_id,
			status = EXCLUDED.status,
			last_error = EXCLUDED.last_error,
			failure_count = EXCLUDED.failure_count,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	var subscriptionID, resourceID, channelID, lastError sql.NullString
	if webhook.SubscriptionID != "" {
		subscriptionID = sql.NullString{String: webhook.SubscriptionID, Valid: true}
	}
	if webhook.ResourceID != "" {
		resourceID = sql.NullString{String: webhook.ResourceID, Valid: true}
	}
	if webhook.ChannelID != "" {
		channelID = sql.NullString{String: webhook.ChannelID, Valid: true}
	}
	if webhook.LastError != "" {
		lastError = sql.NullString{String: webhook.LastError, Valid: true}
	}

	return a.db.QueryRow(
		query,
		webhook.ConnectionID,
		webhook.UserID,
		webhook.Provider,
		subscriptionID,
		resourceID,
		channelID,
		string(webhook.Status),
		lastError,
		webhook.FailureCount,
		webhook.ExpiresAt,
	).Scan(&webhook.ID, &webhook.CreatedAt, &webhook.UpdatedAt)
}

// GetByID retrieves a webhook by ID.
func (a *WebhookAdapter) GetByID(id int64) (*domain.WebhookConfig, error) {
	var row webhookRow
	err := a.db.Get(&row, `SELECT * FROM webhook_configs WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

// GetByConnectionID retrieves a webhook by connection ID.
func (a *WebhookAdapter) GetByConnectionID(connectionID int64) (*domain.WebhookConfig, error) {
	var row webhookRow
	err := a.db.Get(&row, `SELECT * FROM webhook_configs WHERE connection_id = $1`, connectionID)
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

// GetBySubscriptionID retrieves a webhook by subscription ID.
func (a *WebhookAdapter) GetBySubscriptionID(subscriptionID string) (*domain.WebhookConfig, error) {
	var row webhookRow
	err := a.db.Get(&row, `SELECT * FROM webhook_configs WHERE subscription_id = $1`, subscriptionID)
	if err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

// Update updates a webhook config.
func (a *WebhookAdapter) Update(webhook *domain.WebhookConfig) error {
	query := `
		UPDATE webhook_configs SET
			subscription_id = $1, resource_id = $2, channel_id = $3,
			status = $4, last_error = $5, failure_count = $6,
			expires_at = $7, last_renewed_at = $8, updated_at = NOW()
		WHERE id = $9
	`

	var subscriptionID, resourceID, channelID, lastError sql.NullString
	var lastRenewedAt sql.NullTime

	if webhook.SubscriptionID != "" {
		subscriptionID = sql.NullString{String: webhook.SubscriptionID, Valid: true}
	}
	if webhook.ResourceID != "" {
		resourceID = sql.NullString{String: webhook.ResourceID, Valid: true}
	}
	if webhook.ChannelID != "" {
		channelID = sql.NullString{String: webhook.ChannelID, Valid: true}
	}
	if webhook.LastError != "" {
		lastError = sql.NullString{String: webhook.LastError, Valid: true}
	}
	if webhook.LastRenewedAt != nil {
		lastRenewedAt = sql.NullTime{Time: *webhook.LastRenewedAt, Valid: true}
	}

	_, err := a.db.Exec(query,
		subscriptionID,
		resourceID,
		channelID,
		string(webhook.Status),
		lastError,
		webhook.FailureCount,
		webhook.ExpiresAt,
		lastRenewedAt,
		webhook.ID,
	)
	return err
}

// Delete deletes a webhook config.
func (a *WebhookAdapter) Delete(id int64) error {
	_, err := a.db.Exec(`DELETE FROM webhook_configs WHERE id = $1`, id)
	return err
}

// DeleteByConnectionID deletes webhook config by connection ID.
func (a *WebhookAdapter) DeleteByConnectionID(connectionID int64) error {
	_, err := a.db.Exec(`DELETE FROM webhook_configs WHERE connection_id = $1`, connectionID)
	return err
}

// ListByUserID lists webhooks for a user.
func (a *WebhookAdapter) ListByUserID(userID uuid.UUID) ([]*domain.WebhookConfig, error) {
	var rows []webhookRow
	err := a.db.Select(&rows, `SELECT * FROM webhook_configs WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}

	webhooks := make([]*domain.WebhookConfig, len(rows))
	for i, row := range rows {
		webhooks[i] = row.toDomain()
	}
	return webhooks, nil
}

// ListExpiring lists webhooks expiring before the given time.
func (a *WebhookAdapter) ListExpiring(before time.Time) ([]*domain.WebhookConfig, error) {
	var rows []webhookRow
	err := a.db.Select(&rows, `
		SELECT * FROM webhook_configs
		WHERE status = 'active' AND expires_at < $1
		ORDER BY expires_at ASC
	`, before)
	if err != nil {
		return nil, err
	}

	webhooks := make([]*domain.WebhookConfig, len(rows))
	for i, row := range rows {
		webhooks[i] = row.toDomain()
	}
	return webhooks, nil
}

// ListByStatus lists webhooks by status.
func (a *WebhookAdapter) ListByStatus(status domain.WebhookStatus) ([]*domain.WebhookConfig, error) {
	var rows []webhookRow
	err := a.db.Select(&rows, `SELECT * FROM webhook_configs WHERE status = $1 ORDER BY created_at DESC`, string(status))
	if err != nil {
		return nil, err
	}

	webhooks := make([]*domain.WebhookConfig, len(rows))
	for i, row := range rows {
		webhooks[i] = row.toDomain()
	}
	return webhooks, nil
}

// ListActive lists all active webhooks.
func (a *WebhookAdapter) ListActive() ([]*domain.WebhookConfig, error) {
	return a.ListByStatus(domain.WebhookStatusActive)
}

// UpdateStatus updates webhook status.
func (a *WebhookAdapter) UpdateStatus(id int64, status domain.WebhookStatus, lastError string) error {
	var lastErrorNull sql.NullString
	if lastError != "" {
		lastErrorNull = sql.NullString{String: lastError, Valid: true}
	}

	_, err := a.db.Exec(`
		UPDATE webhook_configs SET status = $1, last_error = $2, updated_at = NOW()
		WHERE id = $3
	`, string(status), lastErrorNull, id)
	return err
}

// UpdateExpiration updates webhook expiration.
func (a *WebhookAdapter) UpdateExpiration(id int64, expiresAt time.Time) error {
	_, err := a.db.Exec(`
		UPDATE webhook_configs SET expires_at = $1, last_renewed_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`, expiresAt, id)
	return err
}

// IncrementFailureCount increments the failure count.
func (a *WebhookAdapter) IncrementFailureCount(id int64) error {
	_, err := a.db.Exec(`
		UPDATE webhook_configs SET failure_count = failure_count + 1, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

// ResetFailureCount resets the failure count.
func (a *WebhookAdapter) ResetFailureCount(id int64) error {
	_, err := a.db.Exec(`
		UPDATE webhook_configs SET failure_count = 0, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

// UpdateLastTriggered updates the last triggered time.
func (a *WebhookAdapter) UpdateLastTriggered(id int64) error {
	_, err := a.db.Exec(`
		UPDATE webhook_configs SET last_triggered_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

// Ensure interface compliance
var _ domain.WebhookRepository = (*WebhookAdapter)(nil)
