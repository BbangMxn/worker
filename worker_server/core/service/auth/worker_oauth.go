package auth

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"worker_server/adapter/out/persistence"
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// decodeJSON decodes JSON from reader into target struct
func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

type OAuthService struct {
	oauthRepo       out.OAuthRepository
	messageProducer out.MessageProducer
	webhookSetup    func(ctx context.Context, connectionID int64) error // Webhook 설정 함수
	googleConfig    *oauth2.Config
	msConfig        *oauth2.Config
}

func NewOAuthService(oauthRepo domain.OAuthRepository) *OAuthService {
	return &OAuthService{}
}

func NewOAuthServiceWithConfig(
	googleClientID, googleClientSecret, googleRedirectURL string,
	msClientID, msClientSecret, msRedirectURL string,
	db *sqlx.DB,
) *OAuthService {
	var repo out.OAuthRepository
	if db != nil {
		repo = persistence.NewOAuthAdapter(db)
	}

	var googleConfig *oauth2.Config
	if googleClientID != "" && googleClientSecret != "" {
		googleConfig = &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			RedirectURL:  googleRedirectURL,
			Scopes: []string{
				"https://www.googleapis.com/auth/gmail.readonly",
				"https://www.googleapis.com/auth/gmail.send",
				"https://www.googleapis.com/auth/gmail.modify",
				"https://www.googleapis.com/auth/calendar.readonly",
				"https://www.googleapis.com/auth/calendar.events",
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		}
	}

	// Microsoft config would be similar

	return &OAuthService{
		oauthRepo:    repo,
		googleConfig: googleConfig,
	}
}

// SetMessageProducer sets the message producer for triggering sync jobs
func (s *OAuthService) SetMessageProducer(producer out.MessageProducer) {
	s.messageProducer = producer
}

// SetWebhookSetup sets the webhook setup function for auto-subscribing on OAuth
func (s *OAuthService) SetWebhookSetup(setup func(ctx context.Context, connectionID int64) error) {
	s.webhookSetup = setup
}

func (s *OAuthService) GetAuthURL(ctx context.Context, provider domain.OAuthProvider, state string) (string, error) {
	switch provider {
	case domain.ProviderGoogle, "gmail":
		if s.googleConfig == nil {
			return "", fmt.Errorf("google oauth not configured")
		}
		return s.googleConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
	case domain.ProviderOutlook:
		return "", fmt.Errorf("microsoft oauth not yet implemented")
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

func (s *OAuthService) HandleCallback(ctx context.Context, provider domain.OAuthProvider, code string, userID uuid.UUID) (*domain.OAuthConnection, error) {
	logger.Info("[OAuthService.HandleCallback] Starting for provider: %s, userID: %s", provider, userID)

	var token *oauth2.Token
	var email string
	var err error

	switch provider {
	case domain.ProviderGoogle, "gmail":
		if s.googleConfig == nil {
			return nil, fmt.Errorf("google oauth not configured")
		}
		token, err = s.googleConfig.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange token: %w", err)
		}
		// Get user email from Google
		email, err = s.getGoogleEmail(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("failed to get user email: %w", err)
		}
		logger.Info("[OAuthService.HandleCallback] Got email: %s", email)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	// Create or update connection
	conn := &domain.OAuthConnection{
		UserID:       userID,
		Provider:     provider,
		Email:        email,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
		IsConnected:  true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if s.oauthRepo != nil {
		// Try to find existing connection by email
		existing, _ := s.oauthRepo.GetByEmail(ctx, userID.String(), string(provider), email)
		if existing != nil {
			conn.ID = existing.ID
			if err := s.oauthRepo.Update(ctx, toOAuthEntity(conn)); err != nil {
				return nil, fmt.Errorf("failed to update connection: %w", err)
			}
			logger.Info("[OAuthService.HandleCallback] Connection updated ID: %d", conn.ID)
		} else {
			entity := toOAuthEntity(conn)
			if err := s.oauthRepo.Create(ctx, entity); err != nil {
				return nil, fmt.Errorf("failed to create connection: %w", err)
			}
			conn.ID = entity.ID
			logger.Info("[OAuthService.HandleCallback] Connection created ID: %d", conn.ID)
		}
	}

	// Trigger initial mail sync job
	if s.messageProducer != nil && conn.ID > 0 {
		syncJob := &out.MailSyncJob{
			UserID:       userID.String(),
			ConnectionID: conn.ID,
			Provider:     string(provider),
			FullSync:     true,
		}
		if err := s.messageProducer.PublishMailSync(ctx, syncJob); err != nil {
			// Log error but don't fail the callback
			logger.Warn("Warning: failed to publish mail sync job: %v", err)
		} else {
			logger.Info("Published mail sync job for connection %d", conn.ID)
		}

		// Also trigger calendar sync
		calSyncJob := &out.CalendarSyncJob{
			UserID:       userID.String(),
			ConnectionID: conn.ID,
			FullSync:     true,
		}
		if err := s.messageProducer.PublishCalendarSync(ctx, calSyncJob); err != nil {
			logger.Warn("Warning: failed to publish calendar sync job: %v", err)
		}
	}

	// Setup Gmail Push Notification (Webhook) for real-time updates
	if s.webhookSetup != nil && conn.ID > 0 {
		go func() {
			// 비동기로 webhook 설정 (연결 콜백을 빨리 반환하기 위해)
			if err := s.webhookSetup(context.Background(), conn.ID); err != nil {
				logger.Warn("[OAuthService.HandleCallback] Failed to setup webhook: %v", err)
			} else {
				logger.Info("[OAuthService.HandleCallback] Webhook setup for connection %d", conn.ID)
			}
		}()
	}

	return conn, nil
}

func (s *OAuthService) getGoogleEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	client := s.googleConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(resp.Body, &userInfo); err != nil {
		return "", err
	}
	return userInfo.Email, nil
}

func (s *OAuthService) GetConnection(ctx context.Context, connectionID int64) (*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return nil, fmt.Errorf("oauth repository not initialized")
	}
	entity, err := s.oauthRepo.GetByID(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	return toDomainOAuth(entity), nil
}

func (s *OAuthService) GetConnectionsByUser(ctx context.Context, userID uuid.UUID) ([]*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return []*domain.OAuthConnection{}, nil
	}
	entities, err := s.oauthRepo.ListByUser(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	result := make([]*domain.OAuthConnection, len(entities))
	for i, e := range entities {
		result[i] = toDomainOAuth(e)
	}
	return result, nil
}

func (s *OAuthService) Disconnect(ctx context.Context, connectionID int64) error {
	if s.oauthRepo == nil {
		return fmt.Errorf("oauth repository not initialized")
	}
	return s.oauthRepo.Disconnect(ctx, connectionID)
}

// ErrTokenExpired indicates that the OAuth token has expired and requires re-authentication.
var ErrTokenExpired = fmt.Errorf("oauth token expired, re-authentication required")

// isTokenExpiredError checks if the error indicates a permanent token failure.
func isTokenExpiredError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Google OAuth errors indicating token is permanently invalid
	return strings.Contains(errStr, "invalid_client") ||
		strings.Contains(errStr, "invalid_grant") ||
		strings.Contains(errStr, "Token has been expired or revoked") ||
		strings.Contains(errStr, "Token has been revoked")
}

func (s *OAuthService) RefreshToken(ctx context.Context, connectionID int64) error {
	if s.oauthRepo == nil {
		return fmt.Errorf("oauth repository not initialized")
	}

	entity, err := s.oauthRepo.GetByID(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	// Create token from stored values
	token := &oauth2.Token{
		AccessToken:  entity.AccessToken,
		RefreshToken: entity.RefreshToken,
		Expiry:       entity.ExpiresAt,
	}

	// Get config based on provider
	var config *oauth2.Config
	switch entity.Provider {
	case "google", "gmail":
		config = s.googleConfig
	default:
		return fmt.Errorf("unsupported provider: %s", entity.Provider)
	}

	if config == nil {
		return fmt.Errorf("oauth config not initialized for provider: %s", entity.Provider)
	}

	// Refresh token
	tokenSource := config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		// Check if token is permanently expired (requires re-authentication)
		if isTokenExpiredError(err) {
			logger.Warn("[OAuthService.RefreshToken] Token expired for connection %d, marking as disconnected: %v",
				connectionID, err)
			// Mark connection as disconnected
			entity.IsConnected = false
			entity.UpdatedAt = time.Now()
			if updateErr := s.oauthRepo.Update(ctx, entity); updateErr != nil {
				logger.Error("[OAuthService.RefreshToken] Failed to update connection status: %v", updateErr)
			}
			return ErrTokenExpired
		}
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	// Update stored token
	entity.AccessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		entity.RefreshToken = newToken.RefreshToken
	}
	entity.ExpiresAt = newToken.Expiry
	entity.UpdatedAt = time.Now()

	if err := s.oauthRepo.Update(ctx, entity); err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}

	logger.Debug("[OAuthService.RefreshToken] Token refreshed successfully for connection %d", connectionID)
	return nil
}

func (s *OAuthService) GetValidToken(ctx context.Context, connectionID int64) (string, error) {
	conn, err := s.GetConnection(ctx, connectionID)
	if err != nil {
		return "", err
	}

	// Check if token is expired or will expire soon (within 5 minutes)
	if time.Until(conn.ExpiresAt) < 5*time.Minute {
		if err := s.RefreshToken(ctx, connectionID); err != nil {
			return "", fmt.Errorf("failed to refresh expired token: %w", err)
		}
		// Re-fetch connection with new token
		conn, err = s.GetConnection(ctx, connectionID)
		if err != nil {
			return "", err
		}
	}

	return conn.AccessToken, nil
}

// GetOAuth2Token returns an oauth2.Token for use with providers
func (s *OAuthService) GetOAuth2Token(ctx context.Context, connectionID int64) (*oauth2.Token, error) {
	conn, err := s.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	// Check if token needs refresh
	if time.Until(conn.ExpiresAt) < 5*time.Minute {
		if err := s.RefreshToken(ctx, connectionID); err != nil {
			return nil, fmt.Errorf("failed to refresh expired token: %w", err)
		}
		conn, err = s.GetConnection(ctx, connectionID)
		if err != nil {
			return nil, err
		}
	}

	return &oauth2.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		Expiry:       conn.ExpiresAt,
		TokenType:    "Bearer",
	}, nil
}

// Helper functions
func toOAuthEntity(conn *domain.OAuthConnection) *out.OAuthConnectionEntity {
	return &out.OAuthConnectionEntity{
		ID:           conn.ID,
		UserID:       conn.UserID.String(),
		Provider:     string(conn.Provider),
		Email:        conn.Email,
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		ExpiresAt:    conn.ExpiresAt,
		IsConnected:  conn.IsConnected,
		IsDefault:    conn.IsDefault,
		Signature:    conn.Signature,
		CreatedAt:    conn.CreatedAt,
		UpdatedAt:    conn.UpdatedAt,
	}
}

func toDomainOAuth(entity *out.OAuthConnectionEntity) *domain.OAuthConnection {
	userID, _ := uuid.Parse(entity.UserID)
	return &domain.OAuthConnection{
		ID:           entity.ID,
		UserID:       userID,
		Provider:     domain.OAuthProvider(entity.Provider),
		Email:        entity.Email,
		AccessToken:  entity.AccessToken,
		RefreshToken: entity.RefreshToken,
		ExpiresAt:    entity.ExpiresAt,
		IsConnected:  entity.IsConnected,
		IsDefault:    entity.IsDefault,
		Signature:    entity.Signature,
		CreatedAt:    entity.CreatedAt,
		UpdatedAt:    entity.UpdatedAt,
	}
}

// GetConnectionByEmail finds a connection by email and provider.
func (s *OAuthService) GetConnectionByEmail(ctx context.Context, email string, provider string) (*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return nil, fmt.Errorf("oauth repository not initialized")
	}

	entity, err := s.oauthRepo.GetByEmailOnly(ctx, email, provider)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}

	return toDomainOAuth(entity), nil
}

// GetConnectionByUserID finds a connection by user ID and provider.
// This is used by the Orchestrator for proposal execution.
func (s *OAuthService) GetConnectionByUserID(ctx context.Context, userID uuid.UUID, provider string) (*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return nil, fmt.Errorf("oauth repository not initialized")
	}

	// Get all connections for user and filter by provider
	connections, err := s.GetConnectionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Find matching provider (default to google if not specified)
	if provider == "" {
		provider = "google"
	}

	// Normalize provider name (gmail -> google)
	if provider == "gmail" {
		provider = "google"
	}

	for _, conn := range connections {
		if string(conn.Provider) == provider && conn.IsConnected {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("no active connection found for provider: %s", provider)
}

// GetConnectionByWebhookID finds a connection by webhook subscription ID.
func (s *OAuthService) GetConnectionByWebhookID(ctx context.Context, subscriptionID string, provider string) (*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return nil, fmt.Errorf("oauth repository not initialized")
	}

	entity, err := s.oauthRepo.GetByWebhookID(ctx, subscriptionID, provider)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}

	return toDomainOAuth(entity), nil
}

// ListAllActiveConnections returns all active OAuth connections (for webhook setup on startup).
func (s *OAuthService) ListAllActiveConnections(ctx context.Context) ([]*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return nil, fmt.Errorf("oauth repository not initialized")
	}

	entities, err := s.oauthRepo.ListAllActive(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.OAuthConnection, len(entities))
	for i, e := range entities {
		result[i] = toDomainOAuth(e)
	}
	return result, nil
}

// GetDefaultConnection returns the default connection for a user.
// If no default is set, returns the first active connection.
func (s *OAuthService) GetDefaultConnection(ctx context.Context, userID uuid.UUID) (*domain.OAuthConnection, error) {
	if s.oauthRepo == nil {
		return nil, fmt.Errorf("oauth repository not initialized")
	}

	connections, err := s.GetConnectionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(connections) == 0 {
		return nil, fmt.Errorf("no connections found for user")
	}

	// Find the default connection
	for _, conn := range connections {
		if conn.IsDefault && conn.IsConnected {
			return conn, nil
		}
	}

	// If no default set, return first active connection
	for _, conn := range connections {
		if conn.IsConnected {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("no active connection found")
}

// SetDefaultConnection sets a connection as the default for sending emails.
// Only one connection can be default per user; this clears other defaults.
func (s *OAuthService) SetDefaultConnection(ctx context.Context, userID uuid.UUID, connectionID int64) error {
	if s.oauthRepo == nil {
		return fmt.Errorf("oauth repository not initialized")
	}

	// Verify the connection belongs to the user
	conn, err := s.GetConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("connection not found: %w", err)
	}
	if conn.UserID != userID {
		return fmt.Errorf("connection does not belong to user")
	}
	if !conn.IsConnected {
		return fmt.Errorf("connection is not active")
	}

	// Clear all defaults for this user first
	connections, err := s.GetConnectionsByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get connections: %w", err)
	}

	for _, c := range connections {
		if c.IsDefault {
			if err := s.oauthRepo.SetDefault(ctx, c.ID, false); err != nil {
				return fmt.Errorf("failed to clear default: %w", err)
			}
		}
	}

	// Set the new default
	if err := s.oauthRepo.SetDefault(ctx, connectionID, true); err != nil {
		return fmt.Errorf("failed to set default: %w", err)
	}

	return nil
}
