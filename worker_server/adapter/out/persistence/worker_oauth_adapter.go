// Package persistence provides database adapters.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"worker_server/core/port/out"
	"worker_server/pkg/crypto"
	"worker_server/pkg/logger"

	"github.com/jmoiron/sqlx"
)

// OAuthAdapter implements out.OAuthRepository using PostgreSQL.
type OAuthAdapter struct {
	db                *sqlx.DB
	encryptionEnabled bool
}

// NewOAuthAdapter creates a new OAuthAdapter.
func NewOAuthAdapter(db *sqlx.DB) *OAuthAdapter {
	// Try to initialize encryption
	err := crypto.Init()
	encryptionEnabled := err == nil
	if !encryptionEnabled {
		logger.Warn("Token encryption disabled: %v", err)
	} else {
		logger.Info("Token encryption enabled")
	}

	return &OAuthAdapter{
		db:                db,
		encryptionEnabled: encryptionEnabled,
	}
}

// encryptToken encrypts a token if encryption is enabled
func (a *OAuthAdapter) encryptToken(token string) string {
	if !a.encryptionEnabled || token == "" {
		return token
	}
	encrypted, err := crypto.EncryptToken(token)
	if err != nil {
		logger.Warn("Failed to encrypt token: %v", err)
		return token
	}
	return encrypted
}

// decryptToken decrypts a token if it appears to be encrypted
func (a *OAuthAdapter) decryptToken(token string) string {
	if token == "" {
		return token
	}
	// Check if token appears to be encrypted
	if !crypto.IsEncrypted(token) {
		return token
	}
	decrypted, err := crypto.DecryptToken(token)
	if err != nil {
		// Token might not be encrypted (legacy), return as-is
		return token
	}
	return decrypted
}

// decryptEntity decrypts tokens in an entity
func (a *OAuthAdapter) decryptEntity(entity *out.OAuthConnectionEntity) {
	if entity == nil {
		return
	}
	entity.AccessToken = a.decryptToken(entity.AccessToken)
	entity.RefreshToken = a.decryptToken(entity.RefreshToken)
}

// ListByUser returns all connections for a user.
func (a *OAuthAdapter) ListByUser(ctx context.Context, userID string) ([]*out.OAuthConnectionEntity, error) {
	var entities []*out.OAuthConnectionEntity
	query := `
		SELECT id, user_id, provider, email, access_token, refresh_token,
		       expires_at, is_connected, is_default, signature, last_sync_at, created_at, updated_at
		FROM oauth_connections
		WHERE user_id = $1
		ORDER BY created_at DESC`

	if err := a.db.SelectContext(ctx, &entities, query, userID); err != nil {
		return nil, err
	}

	// Decrypt tokens
	for _, entity := range entities {
		a.decryptEntity(entity)
	}
	return entities, nil
}

// ListAllActive returns all active connections (for webhook setup on startup).
func (a *OAuthAdapter) ListAllActive(ctx context.Context) ([]*out.OAuthConnectionEntity, error) {
	var entities []*out.OAuthConnectionEntity
	query := `
		SELECT id, user_id, provider, email, access_token, refresh_token,
		       expires_at, is_connected, is_default, signature, last_sync_at, created_at, updated_at
		FROM oauth_connections
		WHERE is_connected = true
		ORDER BY created_at DESC`

	if err := a.db.SelectContext(ctx, &entities, query); err != nil {
		return nil, err
	}

	// Decrypt tokens
	for _, entity := range entities {
		a.decryptEntity(entity)
	}
	return entities, nil
}

// GetByID returns a connection by ID.
func (a *OAuthAdapter) GetByID(ctx context.Context, id int64) (*out.OAuthConnectionEntity, error) {
	var entity out.OAuthConnectionEntity
	query := `
		SELECT id, user_id, provider, email, access_token, refresh_token,
		       expires_at, is_connected, is_default, signature, last_sync_at, created_at, updated_at
		FROM oauth_connections
		WHERE id = $1`

	if err := a.db.GetContext(ctx, &entity, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Decrypt tokens
	a.decryptEntity(&entity)
	return &entity, nil
}

// GetByEmail returns a connection by user, provider and email.
func (a *OAuthAdapter) GetByEmail(ctx context.Context, userID, provider, email string) (*out.OAuthConnectionEntity, error) {
	var entity out.OAuthConnectionEntity
	query := `
		SELECT id, user_id, provider, email, access_token, refresh_token,
		       expires_at, is_connected, is_default, signature, last_sync_at, created_at, updated_at
		FROM oauth_connections
		WHERE user_id = $1 AND provider = $2 AND email = $3`

	if err := a.db.GetContext(ctx, &entity, query, userID, provider, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Decrypt tokens
	a.decryptEntity(&entity)
	return &entity, nil
}

// GetByEmailOnly returns a connection by email and provider (without user ID).
func (a *OAuthAdapter) GetByEmailOnly(ctx context.Context, email, provider string) (*out.OAuthConnectionEntity, error) {
	var entity out.OAuthConnectionEntity
	query := `
		SELECT id, user_id, provider, email, access_token, refresh_token,
		       expires_at, is_connected, is_default, signature, last_sync_at, created_at, updated_at
		FROM oauth_connections
		WHERE email = $1 AND provider = $2 AND is_connected = true
		LIMIT 1`

	if err := a.db.GetContext(ctx, &entity, query, email, provider); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// Decrypt tokens
	a.decryptEntity(&entity)
	return &entity, nil
}

// GetByWebhookID returns a connection by webhook subscription ID.
// Note: This requires a webhook_subscription_id column in oauth_connections table
// For now, we'll join with webhook_configs table
func (a *OAuthAdapter) GetByWebhookID(ctx context.Context, subscriptionID, provider string) (*out.OAuthConnectionEntity, error) {
	var entity out.OAuthConnectionEntity
	query := `
		SELECT oc.id, oc.user_id, oc.provider, oc.email, oc.access_token, oc.refresh_token,
		       oc.expires_at, oc.is_connected, oc.is_default, oc.signature, oc.last_sync_at, oc.created_at, oc.updated_at
		FROM oauth_connections oc
		INNER JOIN webhook_configs wc ON oc.id = wc.connection_id
		WHERE wc.subscription_id = $1 AND oc.provider = $2 AND oc.is_connected = true
		LIMIT 1`

	if err := a.db.GetContext(ctx, &entity, query, subscriptionID, provider); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// Decrypt tokens
	a.decryptEntity(&entity)
	return &entity, nil
}

// Create creates a new connection.
func (a *OAuthAdapter) Create(ctx context.Context, entity *out.OAuthConnectionEntity) error {
	// Encrypt tokens before storing
	encryptedAccessToken := a.encryptToken(entity.AccessToken)
	encryptedRefreshToken := a.encryptToken(entity.RefreshToken)

	query := `
		INSERT INTO oauth_connections (user_id, provider, email, access_token, refresh_token,
		                               expires_at, is_connected, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	return a.db.QueryRowContext(ctx, query,
		entity.UserID,
		entity.Provider,
		entity.Email,
		encryptedAccessToken,
		encryptedRefreshToken,
		entity.ExpiresAt,
		entity.IsConnected,
		entity.CreatedAt,
		entity.UpdatedAt,
	).Scan(&entity.ID)
}

// Update updates an existing connection.
func (a *OAuthAdapter) Update(ctx context.Context, entity *out.OAuthConnectionEntity) error {
	// Encrypt tokens before storing
	encryptedAccessToken := a.encryptToken(entity.AccessToken)
	encryptedRefreshToken := a.encryptToken(entity.RefreshToken)

	query := `
		UPDATE oauth_connections
		SET access_token = $1, refresh_token = $2, expires_at = $3,
		    is_connected = $4, updated_at = $5
		WHERE id = $6`

	_, err := a.db.ExecContext(ctx, query,
		encryptedAccessToken,
		encryptedRefreshToken,
		entity.ExpiresAt,
		entity.IsConnected,
		time.Now(),
		entity.ID,
	)
	return err
}

// Disconnect marks a connection as disconnected.
func (a *OAuthAdapter) Disconnect(ctx context.Context, id int64) error {
	query := `
		UPDATE oauth_connections
		SET is_connected = false, updated_at = $1
		WHERE id = $2`

	_, err := a.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

// Delete removes a connection.
func (a *OAuthAdapter) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM oauth_connections WHERE id = $1`
	_, err := a.db.ExecContext(ctx, query, id)
	return err
}

// SetDefault sets or clears the default flag on a connection.
func (a *OAuthAdapter) SetDefault(ctx context.Context, id int64, isDefault bool) error {
	query := `
		UPDATE oauth_connections
		SET is_default = $1, updated_at = $2
		WHERE id = $3`

	_, err := a.db.ExecContext(ctx, query, isDefault, time.Now(), id)
	return err
}

// Ensure OAuthAdapter implements out.OAuthRepository
var _ out.OAuthRepository = (*OAuthAdapter)(nil)
