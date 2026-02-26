// Package provider implements unified mail provider for multi-account support.
package provider

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// =============================================================================
// Unified Mail Provider - Gmail + Outlook 통합 처리
// =============================================================================

// UnifiedMailProvider provides unified access to multiple mail providers.
// 여러 계정(Gmail, Outlook)을 하나의 인터페이스로 통합 조회합니다.
type UnifiedMailProvider struct {
	providers     map[string]out.EmailProviderPort // "gmail" -> GmailAdapter, "outlook" -> OutlookAdapter
	oauthGetter   OAuthTokenGetter
	emailRepo      out.EmailRepository
	syncStateRepo out.SyncStateRepository
	mu            sync.RWMutex
}

// OAuthTokenGetter interface for getting OAuth tokens by connection ID.
type OAuthTokenGetter interface {
	GetOAuth2Token(ctx context.Context, connectionID int64) (*oauth2.Token, error)
	GetConnection(ctx context.Context, connectionID int64) (*domain.OAuthConnection, error)
	GetConnectionsByUser(ctx context.Context, userID uuid.UUID) ([]*domain.OAuthConnection, error)
}

// UnifiedListOptions represents options for unified listing.
type UnifiedListOptions struct {
	UserID uuid.UUID
	Limit  int
	Cursor *UnifiedCursor
	Folder *string
	Search *string
}

// UnifiedCursor tracks pagination state across DB and multiple providers.
type UnifiedCursor struct {
	DBOffset    int                       `json:"db_offset"`
	DBExhausted bool                      `json:"db_exhausted"`
	Providers   map[int64]*ProviderCursor `json:"providers"` // connectionID -> cursor
	LastTime    *time.Time                `json:"last_time,omitempty"`
}

// ProviderCursor tracks pagination state for a single provider.
type ProviderCursor struct {
	Type      string `json:"type"` // "gmail" or "outlook"
	PageToken string `json:"page_token"`
	Exhausted bool   `json:"exhausted"` // no more data from this provider
}

// UnifiedListResult represents unified list result.
type UnifiedListResult struct {
	Emails     []*UnifiedEmail `json:"emails"`
	Total      int             `json:"total"` // DB total (known count)
	HasMore    bool            `json:"has_more"`
	NextCursor *UnifiedCursor  `json:"next_cursor,omitempty"`
}

// UnifiedEmail represents an email from any provider.
type UnifiedEmail struct {
	ID           int64     `json:"id,omitempty"` // DB ID (0 if not in DB)
	ConnectionID int64     `json:"connection_id"`
	ProviderType string    `json:"provider_type"` // "gmail" or "outlook"
	ProviderID   string    `json:"provider_id"`   // external ID
	Subject      string    `json:"subject"`
	FromEmail    string    `json:"from_email"`
	FromName     *string   `json:"from_name,omitempty"`
	FromAvatar   *string   `json:"from_avatar,omitempty"`
	Snippet      string    `json:"snippet"`
	Folder       string    `json:"folder"`
	IsRead       bool      `json:"is_read"`
	IsStarred    bool      `json:"is_starred"`
	HasAttach    bool      `json:"has_attachments"`
	ReceivedAt   time.Time `json:"received_at"`
}

// NewUnifiedMailProvider creates a new unified mail provider.
func NewUnifiedMailProvider(
	oauthGetter OAuthTokenGetter,
	emailRepo out.EmailRepository,
	syncStateRepo out.SyncStateRepository,
) *UnifiedMailProvider {
	return &UnifiedMailProvider{
		providers:     make(map[string]out.EmailProviderPort),
		oauthGetter:   oauthGetter,
		emailRepo:      emailRepo,
		syncStateRepo: syncStateRepo,
	}
}

// RegisterProvider registers a mail provider.
func (u *UnifiedMailProvider) RegisterProvider(providerType string, provider out.EmailProviderPort) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.providers[providerType] = provider
}

// =============================================================================
// Unified List - DB 우선 + Provider 보충
// =============================================================================

// ListAll lists emails from all connections with unified pagination.
// 철학: DB 먼저 조회, 부족하면 Provider에서 보충, 시간순 정렬
func (u *UnifiedMailProvider) ListAll(ctx context.Context, opts *UnifiedListOptions) (*UnifiedListResult, error) {
	if opts == nil {
		return nil, nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 30
	}

	// Initialize cursor
	cursor := opts.Cursor
	if cursor == nil {
		cursor = &UnifiedCursor{
			DBOffset:  0,
			Providers: make(map[int64]*ProviderCursor),
		}
	}

	// 1. Get user's connections
	connections, err := u.oauthGetter.GetConnectionsByUser(ctx, opts.UserID)
	if err != nil {
		return nil, err
	}

	// 2. Query DB first (all connections, sorted by received_at DESC)
	var dbEmails []*UnifiedEmail
	var dbTotal int

	if !cursor.DBExhausted {
		dbEmails, dbTotal, err = u.queryDB(ctx, opts, cursor.DBOffset)
		if err != nil {
			logger.Warn("[UnifiedProvider] DB query failed: %v", err)
		}
	}

	// 3. Calculate how many more we need
	needed := opts.Limit - len(dbEmails)

	// Mark DB as exhausted if we got fewer than requested
	if len(dbEmails) < opts.Limit && !cursor.DBExhausted {
		cursor.DBExhausted = true
	}

	// 4. If DB is exhausted or insufficient, fetch from providers
	var providerEmails []*UnifiedEmail
	if needed > 0 && len(connections) > 0 {
		providerEmails, err = u.fetchFromProviders(ctx, connections, needed, cursor)
		if err != nil {
			logger.Warn("[UnifiedProvider] Provider fetch failed: %v", err)
		}
	}

	// 5. Merge and deduplicate (by ProviderID)
	allEmails := u.mergeAndDeduplicate(dbEmails, providerEmails)

	// 6. Sort by received_at DESC
	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].ReceivedAt.After(allEmails[j].ReceivedAt)
	})

	// 7. Trim to limit
	if len(allEmails) > opts.Limit {
		allEmails = allEmails[:opts.Limit]
	}

	// 8. Update cursor for next page
	nextCursor := &UnifiedCursor{
		DBOffset:    cursor.DBOffset + len(dbEmails),
		DBExhausted: cursor.DBExhausted,
		Providers:   cursor.Providers,
	}
	if len(allEmails) > 0 {
		lastTime := allEmails[len(allEmails)-1].ReceivedAt
		nextCursor.LastTime = &lastTime
	}

	// 9. Determine if there's more
	hasMore := u.hasMoreData(cursor, dbTotal, len(dbEmails))

	return &UnifiedListResult{
		Emails:     allEmails,
		Total:      dbTotal,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// =============================================================================
// Internal Methods
// =============================================================================

// queryDB queries emails from PostgreSQL.
func (u *UnifiedMailProvider) queryDB(ctx context.Context, opts *UnifiedListOptions, offset int) ([]*UnifiedEmail, int, error) {
	if u.emailRepo == nil {
		return nil, 0, nil
	}

	// Build filter
	query := &out.MailListQuery{
		Limit:   opts.Limit,
		Offset:  offset,
		OrderBy: "received_at",
		Order:   "DESC",
	}
	if opts.Folder != nil {
		query.Folder = *opts.Folder
	}

	// Query
	emails, total, err := u.emailRepo.List(ctx, opts.UserID, query)
	if err != nil {
		return nil, 0, err
	}

	// Convert to UnifiedEmail
	result := make([]*UnifiedEmail, len(emails))
	for i, e := range emails {
		var fromName *string
		if e.FromName != "" {
			fromName = &e.FromName
		}
		result[i] = &UnifiedEmail{
			ID:           e.ID,
			ConnectionID: e.ConnectionID,
			ProviderType: e.Provider,
			ProviderID:   e.ExternalID,
			Subject:      e.Subject,
			FromEmail:    e.FromEmail,
			FromName:     fromName,
			Snippet:      e.Snippet,
			Folder:       e.Folder,
			IsRead:       e.IsRead,
			HasAttach:    e.HasAttachment,
			ReceivedAt:   e.ReceivedAt,
		}
	}

	return result, total, nil
}

// fetchFromProviders fetches emails from all providers in parallel.
func (u *UnifiedMailProvider) fetchFromProviders(
	ctx context.Context,
	connections []*domain.OAuthConnection,
	needed int,
	cursor *UnifiedCursor,
) ([]*UnifiedEmail, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	type fetchResult struct {
		emails    []*UnifiedEmail
		connID    int64
		pageToken string
		exhausted bool
		err       error
	}

	results := make(chan fetchResult, len(connections))
	var wg sync.WaitGroup

	// Limit concurrent provider queries to prevent resource exhaustion
	const maxConcurrentProviders = 5
	sem := make(chan struct{}, maxConcurrentProviders)

	// Fetch from each connection in parallel (bounded)
	for _, conn := range connections {
		// Skip exhausted connections
		if pc, ok := cursor.Providers[conn.ID]; ok && pc.Exhausted {
			continue
		}

		providerType := string(conn.Provider)
		provider, ok := u.providers[providerType]
		if !ok {
			continue
		}

		wg.Add(1)
		go func(conn *domain.OAuthConnection, provider out.EmailProviderPort, provType string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Get token
			token, err := u.oauthGetter.GetOAuth2Token(ctx, conn.ID)
			if err != nil {
				results <- fetchResult{connID: conn.ID, err: err}
				return
			}

			// Get page token from cursor
			var pageToken string
			if pc, ok := cursor.Providers[conn.ID]; ok {
				pageToken = pc.PageToken
			}

			// Fetch from provider
			listResult, err := provider.ListMessages(ctx, token, &out.ProviderListOptions{
				MaxResults: needed,
				PageToken:  pageToken,
			})
			if err != nil {
				results <- fetchResult{connID: conn.ID, err: err}
				return
			}

			// Convert to UnifiedEmail
			emails := make([]*UnifiedEmail, len(listResult.Messages))
			for i, msg := range listResult.Messages {
				var fromName *string
				if msg.From.Name != "" {
					fromName = &msg.From.Name
				}
				emails[i] = &UnifiedEmail{
					ConnectionID: conn.ID,
					ProviderType: provType,
					ProviderID:   msg.ExternalID,
					Subject:      msg.Subject,
					FromEmail:    msg.From.Email,
					FromName:     fromName,
					Snippet:      msg.Snippet,
					Folder:       msg.Folder,
					IsRead:       msg.IsRead,
					IsStarred:    msg.IsStarred,
					HasAttach:    msg.HasAttachment,
					ReceivedAt:   msg.ReceivedAt,
				}
			}

			exhausted := listResult.NextPageToken == ""
			results <- fetchResult{
				emails:    emails,
				connID:    conn.ID,
				pageToken: listResult.NextPageToken,
				exhausted: exhausted,
			}
		}(conn, provider, providerType)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allEmails []*UnifiedEmail
	for result := range results {
		if result.err != nil {
			logger.Warn("[UnifiedProvider] Failed to fetch from connection %d: %v", result.connID, result.err)
			continue
		}

		allEmails = append(allEmails, result.emails...)

		// Update cursor
		if cursor.Providers == nil {
			cursor.Providers = make(map[int64]*ProviderCursor)
		}
		cursor.Providers[result.connID] = &ProviderCursor{
			PageToken: result.pageToken,
			Exhausted: result.exhausted,
		}
	}

	return allEmails, nil
}

// mergeAndDeduplicate merges DB and provider emails, removing duplicates.
func (u *UnifiedMailProvider) mergeAndDeduplicate(dbEmails, providerEmails []*UnifiedEmail) []*UnifiedEmail {
	// Build set of existing ProviderIDs from DB
	seen := make(map[string]bool, len(dbEmails))
	for _, e := range dbEmails {
		seen[e.ProviderID] = true
	}

	// Start with DB emails
	result := make([]*UnifiedEmail, 0, len(dbEmails)+len(providerEmails))
	result = append(result, dbEmails...)

	// Add provider emails that aren't in DB
	for _, e := range providerEmails {
		if !seen[e.ProviderID] {
			result = append(result, e)
			seen[e.ProviderID] = true
		}
	}

	return result
}

// hasMoreData checks if there's more data available.
func (u *UnifiedMailProvider) hasMoreData(cursor *UnifiedCursor, dbTotal, dbFetched int) bool {
	// More in DB?
	if !cursor.DBExhausted && cursor.DBOffset+dbFetched < dbTotal {
		return true
	}

	// More in any provider?
	for _, pc := range cursor.Providers {
		if !pc.Exhausted {
			return true
		}
	}

	return false
}

// =============================================================================
// Cursor Encoding/Decoding
// =============================================================================

// EncodeCursor encodes cursor to base64 string.
func EncodeCursor(cursor *UnifiedCursor) string {
	if cursor == nil {
		return ""
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor decodes cursor from base64 string.
func DecodeCursor(encoded string) *UnifiedCursor {
	if encoded == "" {
		return nil
	}
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	var cursor UnifiedCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil
	}
	return &cursor
}
