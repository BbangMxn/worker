// Package provider implements mail provider adapters and factory.
package provider

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/port/out"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
	"google.golang.org/api/gmail/v1"
)

// =============================================================================
// Provider Factory
// =============================================================================

// Factory creates mail providers based on provider type.
type Factory struct {
	gmailConfig   *GmailConfig
	outlookConfig *OutlookConfig
	connRepo      out.OAuthRepository
}

// OutlookConfig holds Outlook/Microsoft configuration.
type OutlookConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	TenantID     string // "common" for multi-tenant
}

// FactoryConfig holds all provider configurations.
type FactoryConfig struct {
	Gmail   *GmailConfig
	Outlook *OutlookConfig
}

// NewFactory creates a new provider factory.
func NewFactory(cfg *FactoryConfig, connRepo out.OAuthRepository) *Factory {
	return &Factory{
		gmailConfig:   cfg.Gmail,
		outlookConfig: cfg.Outlook,
		connRepo:      connRepo,
	}
}

// CreateProvider creates a provider with the given token.
func (f *Factory) CreateProvider(ctx context.Context, providerType string, token *oauth2.Token) (out.EmailProviderPort, error) {
	switch providerType {
	case "google", "gmail":
		return f.createGmailProvider(token)
	case "outlook":
		return f.createOutlookProvider(ctx, token)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// CreateProviderFromConnection creates a provider from a connection ID.
// This retrieves the token from the database and creates the appropriate provider.
func (f *Factory) CreateProviderFromConnection(ctx context.Context, connectionID int64) (out.EmailProviderPort, error) {
	// Get connection from database
	conn, err := f.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	if conn == nil {
		return nil, fmt.Errorf("connection not found: %d", connectionID)
	}

	// Create OAuth token from connection
	token := &oauth2.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		TokenType:    "Bearer",
		Expiry:       conn.ExpiresAt,
	}

	// Create provider based on type
	provider, err := f.CreateProvider(ctx, string(conn.Provider), token)
	if err != nil {
		return nil, err
	}

	// Wrap with token refresh callback
	return &tokenRefreshingProvider{
		provider:     provider,
		connectionID: connectionID,
		connRepo:     f.connRepo,
		factory:      f,
	}, nil
}

// createGmailProvider creates a Gmail provider.
func (f *Factory) createGmailProvider(token *oauth2.Token) (out.EmailProviderPort, error) {
	if f.gmailConfig == nil {
		return nil, fmt.Errorf("gmail config not set")
	}
	return NewGmailAdapter(f.gmailConfig), nil
}

// createOutlookProvider creates an Outlook provider.
func (f *Factory) createOutlookProvider(ctx context.Context, token *oauth2.Token) (out.EmailProviderPort, error) {
	if f.outlookConfig == nil {
		return nil, fmt.Errorf("outlook config not set")
	}

	config := f.getOutlookOAuthConfig()
	return NewOutlookAdapter(ctx, token, config)
}

// getGmailOAuthConfig returns the Gmail OAuth2 config.
func (f *Factory) getGmailOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     f.gmailConfig.ClientID,
		ClientSecret: f.gmailConfig.ClientSecret,
		RedirectURL:  f.gmailConfig.RedirectURL,
		Scopes: []string{
			gmail.GmailReadonlyScope,
			gmail.GmailSendScope,
			gmail.GmailModifyScope,
			gmail.GmailLabelsScope,
		},
		Endpoint: google.Endpoint,
	}
}

// getOutlookOAuthConfig returns the Outlook OAuth2 config.
func (f *Factory) getOutlookOAuthConfig() *oauth2.Config {
	tenantID := f.outlookConfig.TenantID
	if tenantID == "" {
		tenantID = "common"
	}

	return &oauth2.Config{
		ClientID:     f.outlookConfig.ClientID,
		ClientSecret: f.outlookConfig.ClientSecret,
		RedirectURL:  f.outlookConfig.RedirectURL,
		Scopes: []string{
			"https://graph.microsoft.com/Mail.ReadWrite",
			"https://graph.microsoft.com/Mail.Send",
			"https://graph.microsoft.com/User.Read",
			"offline_access",
		},
		Endpoint: microsoft.AzureADEndpoint(tenantID),
	}
}

// =============================================================================
// Token Refreshing Provider Wrapper
// =============================================================================

// tokenRefreshingProvider wraps a provider and handles token refresh.
type tokenRefreshingProvider struct {
	provider     out.EmailProviderPort
	connectionID int64
	connRepo     out.OAuthRepository
	factory      *Factory
}

// GetProviderType returns the provider type.
func (p *tokenRefreshingProvider) GetProviderType() string {
	return p.provider.GetProviderType()
}

// GetAuthURL returns the OAuth authorization URL.
func (p *tokenRefreshingProvider) GetAuthURL(state string) string {
	return p.provider.GetAuthURL(state)
}

// ExchangeToken exchanges authorization code for token.
func (p *tokenRefreshingProvider) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.provider.ExchangeToken(ctx, code)
}

// RefreshToken refreshes the access token and saves to DB.
func (p *tokenRefreshingProvider) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	newToken, err := p.provider.RefreshToken(ctx, token)
	if err != nil {
		return nil, err
	}

	// Save refreshed token to database
	conn, err := p.connRepo.GetByID(ctx, p.connectionID)
	if err == nil && conn != nil {
		conn.AccessToken = newToken.AccessToken
		conn.RefreshToken = newToken.RefreshToken
		conn.ExpiresAt = newToken.Expiry
		conn.UpdatedAt = time.Now()
		_ = p.connRepo.Update(ctx, conn)
	}

	return newToken, nil
}

// ValidateToken validates the token.
func (p *tokenRefreshingProvider) ValidateToken(ctx context.Context, token *oauth2.Token) (bool, error) {
	return p.provider.ValidateToken(ctx, token)
}

// InitialSync performs initial mail sync.
func (p *tokenRefreshingProvider) InitialSync(ctx context.Context, token *oauth2.Token, opts *out.ProviderSyncOptions) (*out.ProviderSyncResult, error) {
	return p.provider.InitialSync(ctx, token, opts)
}

// IncrementalSync performs incremental sync.
func (p *tokenRefreshingProvider) IncrementalSync(ctx context.Context, token *oauth2.Token, syncState string) (*out.ProviderSyncResult, error) {
	return p.provider.IncrementalSync(ctx, token, syncState)
}

// Watch sets up push notifications.
func (p *tokenRefreshingProvider) Watch(ctx context.Context, token *oauth2.Token) (*out.ProviderWatchResponse, error) {
	return p.provider.Watch(ctx, token)
}

// StopWatch stops push notifications.
func (p *tokenRefreshingProvider) StopWatch(ctx context.Context, token *oauth2.Token) error {
	return p.provider.StopWatch(ctx, token)
}

// GetMessage retrieves a single message.
func (p *tokenRefreshingProvider) GetMessage(ctx context.Context, token *oauth2.Token, externalID string) (*out.ProviderMailMessage, error) {
	return p.provider.GetMessage(ctx, token, externalID)
}

// GetMessageBody retrieves message body.
func (p *tokenRefreshingProvider) GetMessageBody(ctx context.Context, token *oauth2.Token, externalID string) (*out.ProviderMessageBody, error) {
	return p.provider.GetMessageBody(ctx, token, externalID)
}

// ListMessages lists messages.
func (p *tokenRefreshingProvider) ListMessages(ctx context.Context, token *oauth2.Token, opts *out.ProviderListOptions) (*out.ProviderListResult, error) {
	return p.provider.ListMessages(ctx, token, opts)
}

// Send sends a message.
func (p *tokenRefreshingProvider) Send(ctx context.Context, token *oauth2.Token, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	return p.provider.Send(ctx, token, msg)
}

// Reply sends a reply.
func (p *tokenRefreshingProvider) Reply(ctx context.Context, token *oauth2.Token, replyToID string, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	return p.provider.Reply(ctx, token, replyToID, msg)
}

// Forward forwards a message.
func (p *tokenRefreshingProvider) Forward(ctx context.Context, token *oauth2.Token, forwardID string, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	return p.provider.Forward(ctx, token, forwardID, msg)
}

// CreateDraft creates a draft.
func (p *tokenRefreshingProvider) CreateDraft(ctx context.Context, token *oauth2.Token, msg *out.ProviderOutgoingMessage) (*out.ProviderDraftResult, error) {
	return p.provider.CreateDraft(ctx, token, msg)
}

// UpdateDraft updates a draft.
func (p *tokenRefreshingProvider) UpdateDraft(ctx context.Context, token *oauth2.Token, draftID string, msg *out.ProviderOutgoingMessage) (*out.ProviderDraftResult, error) {
	return p.provider.UpdateDraft(ctx, token, draftID, msg)
}

// DeleteDraft deletes a draft.
func (p *tokenRefreshingProvider) DeleteDraft(ctx context.Context, token *oauth2.Token, draftID string) error {
	return p.provider.DeleteDraft(ctx, token, draftID)
}

// SendDraft sends a draft.
func (p *tokenRefreshingProvider) SendDraft(ctx context.Context, token *oauth2.Token, draftID string) (*out.ProviderSendResult, error) {
	return p.provider.SendDraft(ctx, token, draftID)
}

// MarkAsRead marks message as read.
func (p *tokenRefreshingProvider) MarkAsRead(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.MarkAsRead(ctx, token, externalID)
}

// MarkAsUnread marks message as unread.
func (p *tokenRefreshingProvider) MarkAsUnread(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.MarkAsUnread(ctx, token, externalID)
}

// Star stars a message.
func (p *tokenRefreshingProvider) Star(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.Star(ctx, token, externalID)
}

// Unstar unstars a message.
func (p *tokenRefreshingProvider) Unstar(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.Unstar(ctx, token, externalID)
}

// Archive archives a message.
func (p *tokenRefreshingProvider) Archive(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.Archive(ctx, token, externalID)
}

// Trash moves message to trash.
func (p *tokenRefreshingProvider) Trash(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.Trash(ctx, token, externalID)
}

// Restore restores message from trash.
func (p *tokenRefreshingProvider) Restore(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.Restore(ctx, token, externalID)
}

// Delete permanently deletes a message.
func (p *tokenRefreshingProvider) Delete(ctx context.Context, token *oauth2.Token, externalID string) error {
	return p.provider.Delete(ctx, token, externalID)
}

// BatchModify modifies multiple messages.
func (p *tokenRefreshingProvider) BatchModify(ctx context.Context, token *oauth2.Token, req *out.ProviderBatchModifyRequest) error {
	return p.provider.BatchModify(ctx, token, req)
}

// ListLabels lists all labels.
func (p *tokenRefreshingProvider) ListLabels(ctx context.Context, token *oauth2.Token) ([]out.ProviderMailLabel, error) {
	return p.provider.ListLabels(ctx, token)
}

// CreateLabel creates a new label.
func (p *tokenRefreshingProvider) CreateLabel(ctx context.Context, token *oauth2.Token, name string, color *string) (*out.ProviderMailLabel, error) {
	return p.provider.CreateLabel(ctx, token, name, color)
}

// DeleteLabel deletes a label.
func (p *tokenRefreshingProvider) DeleteLabel(ctx context.Context, token *oauth2.Token, labelID string) error {
	return p.provider.DeleteLabel(ctx, token, labelID)
}

// AddLabel adds a label to a message.
func (p *tokenRefreshingProvider) AddLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error {
	return p.provider.AddLabel(ctx, token, messageID, labelID)
}

// RemoveLabel removes a label from a message.
func (p *tokenRefreshingProvider) RemoveLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error {
	return p.provider.RemoveLabel(ctx, token, messageID, labelID)
}

// GetAttachment retrieves an attachment.
func (p *tokenRefreshingProvider) GetAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) ([]byte, string, error) {
	return p.provider.GetAttachment(ctx, token, messageID, attachmentID)
}

// StreamAttachment streams an attachment.
func (p *tokenRefreshingProvider) StreamAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) (*out.ProviderAttachmentStream, error) {
	return p.provider.StreamAttachment(ctx, token, messageID, attachmentID)
}

// GetProfile retrieves user profile.
func (p *tokenRefreshingProvider) GetProfile(ctx context.Context, token *oauth2.Token) (*out.ProviderProfile, error) {
	return p.provider.GetProfile(ctx, token)
}

// CreateUploadSession creates an upload session for large attachments.
func (p *tokenRefreshingProvider) CreateUploadSession(ctx context.Context, token *oauth2.Token, messageID string, req *out.UploadSessionRequest) (*out.UploadSessionResponse, error) {
	return p.provider.CreateUploadSession(ctx, token, messageID, req)
}

// GetUploadSessionStatus gets the status of an upload session.
func (p *tokenRefreshingProvider) GetUploadSessionStatus(ctx context.Context, token *oauth2.Token, sessionID string) (*out.UploadSessionStatus, error) {
	return p.provider.GetUploadSessionStatus(ctx, token, sessionID)
}

// CancelUploadSession cancels an upload session.
func (p *tokenRefreshingProvider) CancelUploadSession(ctx context.Context, token *oauth2.Token, sessionID string) error {
	return p.provider.CancelUploadSession(ctx, token, sessionID)
}

var _ out.EmailProviderPort = (*tokenRefreshingProvider)(nil)
var _ out.MailProviderFactory = (*Factory)(nil)
