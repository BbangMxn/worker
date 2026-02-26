// Package provider implements mail provider adapters.
package provider

import (
	"worker_server/core/port/out"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/sony/gobreaker"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// =============================================================================
// Gmail Metadata Headers
// =============================================================================

// gmailMetadataHeaders contains all headers to request from Gmail API.
// Includes basic headers + RFC classification headers for Stage 0 classification.
var gmailMetadataHeaders = []string{
	// Basic headers
	"From", "To", "Cc", "Bcc", "Subject", "Date",
	"Message-ID", "In-Reply-To", "References", "Content-Type",

	// RFC Classification Headers (Stage 0)
	"List-Unsubscribe",         // RFC 2369 - Newsletter
	"List-Unsubscribe-Post",    // RFC 8058 - One-Click Unsubscribe
	"List-Id",                  // RFC 2919 - Mailing List ID
	"Precedence",               // bulk, list, junk
	"Auto-Submitted",           // RFC 3834 - Auto-generated
	"X-Auto-Response-Suppress", // Microsoft auto-reply

	// Mailer Info
	"X-Mailer",    // Sending client
	"Feedback-ID", // Gmail bulk tracking

	// ESP (Email Service Provider) Detection
	"X-MC-User",           // Mailchimp
	"X-SG-EID",            // SendGrid
	"X-SES-Outgoing",      // Amazon SES
	"X-Mailgun-Variables", // Mailgun
	"X-PM-Message-Id",     // Postmark
	"X-Campaign-ID",       // Campaign emails

	// === Developer Service Headers ===

	// GitHub Headers
	"X-GitHub-Reason",   // review_requested, author, mention, security_alert, ci_activity, etc.
	"X-GitHub-Severity", // critical, high, moderate, low (Dependabot)
	"X-GitHub-Sender",   // GitHub username

	// GitLab Headers
	"X-GitLab-Project",
	"X-GitLab-Pipeline-Id",
	"X-GitLab-NotificationReason",

	// Jira/Atlassian Headers
	"X-JIRA-FingerPrint",
	"X-Atlassian-Token",

	// Linear Headers
	"X-Linear-Team",
	"X-Linear-Project",

	// Sentry Headers
	"X-Sentry-Project",

	// Vercel Headers
	"X-Vercel-Deployment-Url",

	// AWS Headers
	"X-AWS-Service",
}

// =============================================================================
// Token Manager
// =============================================================================

// TokenManager manages OAuth tokens.
type TokenManager struct {
	config *oauth2.Config
}

// NewTokenManager creates a new token manager.
func NewTokenManager(config *oauth2.Config) *TokenManager {
	return &TokenManager{config: config}
}

// =============================================================================
// Gmail Adapter
// =============================================================================

// GmailAdapter implements out.EmailProviderPort for Gmail.
type GmailAdapter struct {
	config       *oauth2.Config
	projectID    string
	topicName    string
	tokenManager *TokenManager
	cb           *gobreaker.CircuitBreaker
}

// GmailConfig holds Gmail configuration.
type GmailConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	ProjectID    string
}

// NewGmailAdapter creates a new Gmail adapter.
func NewGmailAdapter(cfg *GmailConfig) *GmailAdapter {
	config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes: []string{
			gmail.GmailReadonlyScope,
			gmail.GmailSendScope,
			gmail.GmailModifyScope,
			gmail.GmailLabelsScope,
		},
		Endpoint: google.Endpoint,
	}

	cbSettings := gobreaker.Settings{
		Name:        "gmail-api",
		MaxRequests: 3,                // Half-open 상태에서 허용할 요청 수
		Interval:    60 * time.Second, // Closed 상태에서 카운터 리셋 간격
		Timeout:     30 * time.Second, // Open 상태 유지 시간 (이후 Half-open)
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 연속 5회 실패 또는 60% 이상 실패율 (최소 10회 요청)
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.ConsecutiveFailures > 5 ||
				(counts.Requests >= 10 && failureRatio >= 0.6)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("[CircuitBreaker] %s: state changed from %s to %s", name, from.String(), to.String())
		},
	}

	return &GmailAdapter{
		config:       config,
		projectID:    cfg.ProjectID,
		topicName:    fmt.Sprintf("projects/%s/topics/gmail-push", cfg.ProjectID),
		tokenManager: NewTokenManager(config),
		cb:           gobreaker.NewCircuitBreaker(cbSettings),
	}
}

// GetProviderType returns the provider type.
func (a *GmailAdapter) GetProviderType() string {
	return "gmail"
}

// =============================================================================
// Authentication
// =============================================================================

// GetAuthURL returns the OAuth authorization URL.
func (a *GmailAdapter) GetAuthURL(state string) string {
	return a.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// ExchangeToken exchanges authorization code for token.
func (a *GmailAdapter) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return nil, a.wrapError(err, "failed to exchange token")
	}
	return token, nil
}

// RefreshToken refreshes the access token.
func (a *GmailAdapter) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	src := a.config.TokenSource(ctx, token)
	newToken, err := src.Token()
	if err != nil {
		return nil, a.wrapError(err, "failed to refresh token")
	}
	return newToken, nil
}

// ValidateToken validates the token.
func (a *GmailAdapter) ValidateToken(ctx context.Context, token *oauth2.Token) (bool, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return false, err
	}

	_, err = svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == 401 {
			return false, nil
		}
		return false, a.wrapError(err, "failed to validate token")
	}
	return true, nil
}

// =============================================================================
// Sync
// =============================================================================

// InitialSync performs initial mail sync.
func (a *GmailAdapter) InitialSync(ctx context.Context, token *oauth2.Token, opts *out.ProviderSyncOptions) (*out.ProviderSyncResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	maxResults := int64(100)
	if opts != nil && opts.MaxResults > 0 {
		maxResults = int64(opts.MaxResults)
	}

	// 첨부파일이 있는 메시지 ID 목록을 미리 조회 (Gmail의 has:attachment 쿼리 활용)
	attachmentMsgIDs := a.fetchAttachmentMessageIDs(ctx, svc)
	log.Printf("[InitialSync] Found %d messages with attachments via has:attachment query", len(attachmentMsgIDs))

	req := svc.Users.Messages.List("me").MaxResults(maxResults)

	// 날짜 기반 필터 적용 (StartDate가 있으면 after: 쿼리 추가)
	if opts != nil && opts.StartDate != nil {
		dateQuery := fmt.Sprintf("after:%s", opts.StartDate.Format("2006/01/02"))
		req = req.Q(dateQuery)
		log.Printf("[InitialSync] Applying date filter: %s", dateQuery)
	}

	if opts != nil && len(opts.Labels) > 0 {
		req = req.LabelIds(opts.Labels...)
	}

	var allMessages []out.ProviderMailMessage
	pageToken := ""
	if opts != nil && opts.PageToken != "" {
		pageToken = opts.PageToken
	}

	for {
		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		resp, err := req.Context(ctx).Do()
		if err != nil {
			return nil, a.wrapError(err, "failed to list messages")
		}

		// 병렬 처리로 메시지 가져오기 (첨부파일 ID 목록 전달)
		messages := a.fetchMessagesParallelWithAttachmentInfo(ctx, svc, resp.Messages, attachmentMsgIDs)
		allMessages = append(allMessages, messages...)

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken

		if len(allMessages) >= int(maxResults) {
			// 중간 페이지에서도 historyID를 가져와야 함
			profile, profileErr := svc.Users.GetProfile("me").Context(ctx).Do()
			historyID := ""
			if profileErr == nil {
				historyID = fmt.Sprintf("%d", profile.HistoryId)
			}
			return &out.ProviderSyncResult{
				Messages:      allMessages,
				NextSyncState: historyID,
				NextPageToken: pageToken,
				HasMore:       true,
			}, nil
		}
	}

	// Get history ID for incremental sync
	profile, err := svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to get profile")
	}

	return &out.ProviderSyncResult{
		Messages:      allMessages,
		NextSyncState: fmt.Sprintf("%d", profile.HistoryId),
		HasMore:       false,
	}, nil
}

// fetchAttachmentMessageIDs fetches all message IDs that have attachments using Gmail's has:attachment query
func (a *GmailAdapter) fetchAttachmentMessageIDs(ctx context.Context, svc *gmail.Service) map[string]bool {
	attachmentIDs := make(map[string]bool)

	// has:attachment 쿼리로 첨부파일 있는 메시지만 조회
	req := svc.Users.Messages.List("me").Q("has:attachment").MaxResults(500)
	pageToken := ""

	for {
		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		resp, err := req.Context(ctx).Do()
		if err != nil {
			log.Printf("[fetchAttachmentMessageIDs] Error fetching attachment messages: %v", err)
			return attachmentIDs
		}

		for _, msg := range resp.Messages {
			attachmentIDs[msg.Id] = true
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken

		// 최대 2000개까지만 조회 (성능 고려)
		if len(attachmentIDs) >= 2000 {
			break
		}
	}

	return attachmentIDs
}

// GetAttachmentMessageIDs returns message IDs that have attachments (public version for resync)
func (a *GmailAdapter) GetAttachmentMessageIDs(ctx context.Context, token *oauth2.Token, limit int) ([]string, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 500
	}

	var attachmentIDs []string
	req := svc.Users.Messages.List("me").Q("has:attachment").MaxResults(int64(limit))
	pageToken := ""

	for {
		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		resp, err := req.Context(ctx).Do()
		if err != nil {
			return nil, a.wrapError(err, "failed to list attachment messages")
		}

		for _, msg := range resp.Messages {
			attachmentIDs = append(attachmentIDs, msg.Id)
		}

		if resp.NextPageToken == "" || len(attachmentIDs) >= limit {
			break
		}
		pageToken = resp.NextPageToken
	}

	return attachmentIDs, nil
}

// IncrementalSync performs incremental sync using history.
func (a *GmailAdapter) IncrementalSync(ctx context.Context, token *oauth2.Token, syncState string) (*out.ProviderSyncResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	var historyID uint64
	fmt.Sscanf(syncState, "%d", &historyID)

	resp, err := svc.Users.History.List("me").StartHistoryId(historyID).Context(ctx).Do()
	if err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == 404 {
			return nil, out.NewProviderError("gmail", out.ProviderErrSyncRequired, "Full sync required", err, false)
		}
		return nil, a.wrapError(err, "failed to get history")
	}

	var deletedIDs []string
	seenIDs := make(map[string]bool)

	// 추가된 메시지 ID 수집 (중복 제거)
	var addedMsgRefs []*gmail.Message
	for _, history := range resp.History {
		for _, added := range history.MessagesAdded {
			if !seenIDs[added.Message.Id] {
				seenIDs[added.Message.Id] = true
				addedMsgRefs = append(addedMsgRefs, added.Message)
			}
		}

		for _, deleted := range history.MessagesDeleted {
			deletedIDs = append(deletedIDs, deleted.Message.Id)
		}
	}

	// 병렬 처리로 추가된 메시지 가져오기
	messages := a.fetchMessagesParallel(ctx, svc, addedMsgRefs)

	return &out.ProviderSyncResult{
		Messages:      messages,
		DeletedIDs:    deletedIDs,
		NextSyncState: fmt.Sprintf("%d", resp.HistoryId),
		HasMore:       false,
	}, nil
}

// Watch sets up push notifications.
func (a *GmailAdapter) Watch(ctx context.Context, token *oauth2.Token) (*out.ProviderWatchResponse, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	req := &gmail.WatchRequest{
		TopicName: a.topicName,
		LabelIds:  []string{"INBOX"},
	}

	var resp *gmail.WatchResponse
	cbErr := a.executeWithCircuitBreaker(ctx, "Watch", func() error {
		var apiErr error
		resp, apiErr = svc.Users.Watch("me", req).Context(ctx).Do()
		return apiErr
	})
	if cbErr != nil {
		return nil, a.wrapError(cbErr, "failed to setup watch")
	}

	return &out.ProviderWatchResponse{
		ExternalID: fmt.Sprintf("%d", resp.HistoryId),
		Expiration: time.Unix(0, resp.Expiration*int64(time.Millisecond)),
	}, nil
}

// StopWatch stops push notifications.
func (a *GmailAdapter) StopWatch(ctx context.Context, token *oauth2.Token) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	err = svc.Users.Stop("me").Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to stop watch")
	}
	return nil
}

// =============================================================================
// Message Reading
// =============================================================================

// GetMessage retrieves a single message.
func (a *GmailAdapter) GetMessage(ctx context.Context, token *oauth2.Token, externalID string) (*out.ProviderMailMessage, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	var msg *gmail.Message
	cbErr := a.executeWithCircuitBreaker(ctx, "GetMessage", func() error {
		var apiErr error
		msg, apiErr = svc.Users.Messages.Get("me", externalID).Format("full").Context(ctx).Do()
		return apiErr
	})
	if cbErr != nil {
		return nil, a.wrapError(cbErr, "failed to get message")
	}

	result := a.convertMessage(msg)
	return &result, nil
}

// GetMessageBody retrieves message body.
func (a *GmailAdapter) GetMessageBody(ctx context.Context, token *oauth2.Token, externalID string) (*out.ProviderMessageBody, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	var msg *gmail.Message
	cbErr := a.executeWithCircuitBreaker(ctx, "GetMessageBody", func() error {
		var apiErr error
		msg, apiErr = svc.Users.Messages.Get("me", externalID).Format("full").Context(ctx).Do()
		return apiErr
	})
	if cbErr != nil {
		return nil, a.wrapError(cbErr, "failed to get message body")
	}

	body := &out.ProviderMessageBody{}
	a.extractBody(msg.Payload, body, 0) // depth 0 for debugging
	body.Attachments = a.extractAttachments(msg.Payload)

	// Debug logging
	log.Printf("[GmailAdapter] GetMessageBody: externalID=%s, hasText=%v (len=%d), hasHTML=%v (len=%d), attachments=%d",
		externalID, body.Text != "", len(body.Text), body.HTML != "", len(body.HTML), len(body.Attachments))

	return body, nil
}

// ListMessages lists messages with options.
func (a *GmailAdapter) ListMessages(ctx context.Context, token *oauth2.Token, opts *out.ProviderListOptions) (*out.ProviderListResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	maxResults := int64(50)
	if opts != nil && opts.MaxResults > 0 {
		maxResults = int64(opts.MaxResults)
	}

	req := svc.Users.Messages.List("me").MaxResults(maxResults)

	if opts != nil {
		if opts.Query != "" {
			req = req.Q(opts.Query)
		}
		if len(opts.Labels) > 0 {
			req = req.LabelIds(opts.Labels...)
		}
		if opts.PageToken != "" {
			req = req.PageToken(opts.PageToken)
		}
	}

	resp, err := req.Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to list messages")
	}

	// 병렬 처리로 메시지 가져오기
	messages := a.fetchMessagesParallel(ctx, svc, resp.Messages)

	return &out.ProviderListResult{
		Messages:      messages,
		NextPageToken: resp.NextPageToken,
		TotalCount:    int64(resp.ResultSizeEstimate),
	}, nil
}

// fetchMessagesParallel fetches multiple messages in parallel with concurrency limit
// 최적화: Format("metadata")로 본문 제외하고 메타데이터만 가져옴 (응답 크기 90% 감소)
// 안정성: 각 goroutine에 타임아웃 적용, context 취소 시 정상 종료
func (a *GmailAdapter) fetchMessagesParallel(ctx context.Context, svc *gmail.Service, msgRefs []*gmail.Message) []out.ProviderMailMessage {
	if len(msgRefs) == 0 {
		return nil
	}

	// 동시 처리 제한 (Gmail API rate limit 방지)
	const maxConcurrency = 10
	const perMessageTimeout = 15 * time.Second

	type result struct {
		index int
		msg   out.ProviderMailMessage
		err   error
	}

	results := make(chan result, len(msgRefs))
	sem := make(chan struct{}, maxConcurrency)

	// 병렬로 메시지 가져오기
	for i, msgRef := range msgRefs {
		go func(idx int, id string) {
			// 세마포어 획득 (context 취소 시 빠른 종료)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, err: ctx.Err()}
				return
			}

			// 개별 메시지에 타임아웃 적용
			msgCtx, cancel := context.WithTimeout(ctx, perMessageTimeout)
			defer cancel()

			metaMsg, err := svc.Users.Messages.Get("me", id).
				Format("metadata").
				MetadataHeaders(gmailMetadataHeaders...).
				Context(msgCtx).Do()
			if err != nil {
				results <- result{index: idx, err: err}
				return
			}
			results <- result{index: idx, msg: a.convertMessage(metaMsg)}
		}(i, msgRef.Id)
	}

	// 결과 수집 (타임아웃과 context 취소 처리)
	messages := make([]out.ProviderMailMessage, len(msgRefs))
	validCount := 0
	collected := 0

	for collected < len(msgRefs) {
		select {
		case r := <-results:
			collected++
			if r.err == nil {
				messages[r.index] = r.msg
				validCount++
			}
		case <-ctx.Done():
			// Context 취소 시 수집된 것만 반환
			break
		}
	}

	// 에러 없는 메시지만 필터링 (순서 유지)
	filtered := make([]out.ProviderMailMessage, 0, validCount)
	for _, msg := range messages {
		if msg.ExternalID != "" {
			filtered = append(filtered, msg)
		}
	}

	// 첨부파일 상세 정보는 본문 조회 시 가져옴 (no-op)
	a.fetchAttachmentDetails(ctx, svc, filtered)

	return filtered
}

// fetchMessagesParallelWithAttachmentInfo fetches messages with accurate attachment detection
// Gmail의 has:attachment 쿼리 결과를 활용하여 첨부파일 여부를 정확하게 설정
// 안정성: 각 goroutine에 타임아웃 적용, context 취소 시 정상 종료
func (a *GmailAdapter) fetchMessagesParallelWithAttachmentInfo(ctx context.Context, svc *gmail.Service, msgRefs []*gmail.Message, attachmentMsgIDs map[string]bool) []out.ProviderMailMessage {
	if len(msgRefs) == 0 {
		return nil
	}

	const maxConcurrency = 10
	const perMessageTimeout = 15 * time.Second

	type result struct {
		index int
		msg   out.ProviderMailMessage
		err   error
	}

	results := make(chan result, len(msgRefs))
	sem := make(chan struct{}, maxConcurrency)

	for i, msgRef := range msgRefs {
		go func(idx int, id string) {
			// 세마포어 획득 (context 취소 시 빠른 종료)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, err: ctx.Err()}
				return
			}

			// 개별 메시지에 타임아웃 적용
			msgCtx, cancel := context.WithTimeout(ctx, perMessageTimeout)
			defer cancel()

			metaMsg, err := svc.Users.Messages.Get("me", id).
				Format("metadata").
				MetadataHeaders(gmailMetadataHeaders...).
				Context(msgCtx).Do()
			if err != nil {
				results <- result{index: idx, err: err}
				return
			}

			msg := a.convertMessage(metaMsg)
			// has:attachment 쿼리 결과로 정확한 첨부파일 여부 설정
			if attachmentMsgIDs[id] {
				msg.HasAttachment = true
			}
			results <- result{index: idx, msg: msg}
		}(i, msgRef.Id)
	}

	// 결과 수집 (타임아웃과 context 취소 처리)
	messages := make([]out.ProviderMailMessage, len(msgRefs))
	validCount := 0
	collected := 0

	for collected < len(msgRefs) {
		select {
		case r := <-results:
			collected++
			if r.err == nil {
				messages[r.index] = r.msg
				validCount++
			}
		case <-ctx.Done():
			break
		}
	}

	filtered := make([]out.ProviderMailMessage, 0, validCount)
	for _, msg := range messages {
		if msg.ExternalID != "" {
			filtered = append(filtered, msg)
		}
	}

	// 첨부파일 상세 정보는 본문 조회 시 가져옴 (no-op)
	a.fetchAttachmentDetails(ctx, svc, filtered)

	return filtered
}

// fetchMessagesParallelFull fetches multiple messages with full content (본문 필요 시)
// 안정성: 각 goroutine에 타임아웃 적용, context 취소 시 정상 종료
func (a *GmailAdapter) fetchMessagesParallelFull(ctx context.Context, svc *gmail.Service, msgRefs []*gmail.Message) []out.ProviderMailMessage {
	if len(msgRefs) == 0 {
		return nil
	}

	const maxConcurrency = 10
	const perMessageTimeout = 30 * time.Second // full format은 더 오래 걸릴 수 있음

	type result struct {
		index int
		msg   out.ProviderMailMessage
		err   error
	}

	results := make(chan result, len(msgRefs))
	sem := make(chan struct{}, maxConcurrency)

	for i, msgRef := range msgRefs {
		go func(idx int, id string) {
			// 세마포어 획득 (context 취소 시 빠른 종료)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, err: ctx.Err()}
				return
			}

			// 개별 메시지에 타임아웃 적용
			msgCtx, cancel := context.WithTimeout(ctx, perMessageTimeout)
			defer cancel()

			fullMsg, err := svc.Users.Messages.Get("me", id).Format("full").Context(msgCtx).Do()
			if err != nil {
				results <- result{index: idx, err: err}
				return
			}
			results <- result{index: idx, msg: a.convertMessage(fullMsg)}
		}(i, msgRef.Id)
	}

	// 결과 수집 (타임아웃과 context 취소 처리)
	messages := make([]out.ProviderMailMessage, len(msgRefs))
	validCount := 0
	collected := 0

	for collected < len(msgRefs) {
		select {
		case r := <-results:
			collected++
			if r.err == nil {
				messages[r.index] = r.msg
				validCount++
			}
		case <-ctx.Done():
			break
		}
	}

	filtered := make([]out.ProviderMailMessage, 0, validCount)
	for _, msg := range messages {
		if msg.ExternalID != "" {
			filtered = append(filtered, msg)
		}
	}

	return filtered
}

// fetchAttachmentDetails is now a no-op
// 첨부파일 메타데이터는 동기화 시 DB에 저장하지 않음 (URL 기반 방식)
// HasAttachment 플래그는 has:attachment 쿼리 결과로 이미 설정됨
// 실제 첨부파일 목록은 GetMessageBody 호출 시 Provider에서 직접 가져옴
func (a *GmailAdapter) fetchAttachmentDetails(ctx context.Context, svc *gmail.Service, messages []out.ProviderMailMessage) {
	// No-op: 첨부파일 상세 정보는 본문 조회 시 가져옴
}

// =============================================================================
// Message Sending
// =============================================================================

// Send sends a new message.
func (a *GmailAdapter) Send(ctx context.Context, token *oauth2.Token, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	raw := a.buildRawMessage(msg)
	gmailMsg := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(raw)),
	}

	var sent *gmail.Message
	cbErr := a.executeWithCircuitBreaker(ctx, "Send", func() error {
		var apiErr error
		sent, apiErr = svc.Users.Messages.Send("me", gmailMsg).Context(ctx).Do()
		return apiErr
	})
	if cbErr != nil {
		return nil, a.wrapError(cbErr, "failed to send message")
	}

	return &out.ProviderSendResult{
		ExternalID:       sent.Id,
		ExternalThreadID: sent.ThreadId,
		SentAt:           time.Now(),
	}, nil
}

// Reply sends a reply.
func (a *GmailAdapter) Reply(ctx context.Context, token *oauth2.Token, replyToID string, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	// Get original message for thread info
	original, err := svc.Users.Messages.Get("me", replyToID).Format("metadata").Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to get original message")
	}

	msg.ThreadID = original.ThreadId
	msg.InReplyTo = a.getHeader(original.Payload.Headers, "Message-ID")
	msg.References = msg.InReplyTo

	raw := a.buildRawMessage(msg)
	gmailMsg := &gmail.Message{
		Raw:      base64.URLEncoding.EncodeToString([]byte(raw)),
		ThreadId: original.ThreadId,
	}

	sent, err := svc.Users.Messages.Send("me", gmailMsg).Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to send reply")
	}

	return &out.ProviderSendResult{
		ExternalID:       sent.Id,
		ExternalThreadID: sent.ThreadId,
		SentAt:           time.Now(),
	}, nil
}

// Forward forwards a message.
func (a *GmailAdapter) Forward(ctx context.Context, token *oauth2.Token, forwardID string, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	return a.Send(ctx, token, msg)
}

// CreateDraft creates a draft.
func (a *GmailAdapter) CreateDraft(ctx context.Context, token *oauth2.Token, msg *out.ProviderOutgoingMessage) (*out.ProviderDraftResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	raw := a.buildRawMessage(msg)
	draft := &gmail.Draft{
		Message: &gmail.Message{
			Raw: base64.URLEncoding.EncodeToString([]byte(raw)),
		},
	}

	created, err := svc.Users.Drafts.Create("me", draft).Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to create draft")
	}

	return &out.ProviderDraftResult{
		ExternalID: created.Id,
	}, nil
}

// UpdateDraft updates a draft.
func (a *GmailAdapter) UpdateDraft(ctx context.Context, token *oauth2.Token, draftID string, msg *out.ProviderOutgoingMessage) (*out.ProviderDraftResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	raw := a.buildRawMessage(msg)
	draft := &gmail.Draft{
		Message: &gmail.Message{
			Raw: base64.URLEncoding.EncodeToString([]byte(raw)),
		},
	}

	updated, err := svc.Users.Drafts.Update("me", draftID, draft).Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to update draft")
	}

	return &out.ProviderDraftResult{
		ExternalID: updated.Id,
	}, nil
}

// DeleteDraft deletes a draft.
func (a *GmailAdapter) DeleteDraft(ctx context.Context, token *oauth2.Token, draftID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	err = svc.Users.Drafts.Delete("me", draftID).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to delete draft")
	}
	return nil
}

// SendDraft sends a draft.
func (a *GmailAdapter) SendDraft(ctx context.Context, token *oauth2.Token, draftID string) (*out.ProviderSendResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	sent, err := svc.Users.Drafts.Send("me", &gmail.Draft{Id: draftID}).Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to send draft")
	}

	return &out.ProviderSendResult{
		ExternalID:       sent.Id,
		ExternalThreadID: sent.ThreadId,
		SentAt:           time.Now(),
	}, nil
}

// =============================================================================
// Message Modification
// =============================================================================

// MarkAsRead marks message as read.
func (a *GmailAdapter) MarkAsRead(ctx context.Context, token *oauth2.Token, externalID string) error {
	return a.modifyLabels(ctx, token, externalID, nil, []string{"UNREAD"})
}

// MarkAsUnread marks message as unread.
func (a *GmailAdapter) MarkAsUnread(ctx context.Context, token *oauth2.Token, externalID string) error {
	return a.modifyLabels(ctx, token, externalID, []string{"UNREAD"}, nil)
}

// Star stars a message.
func (a *GmailAdapter) Star(ctx context.Context, token *oauth2.Token, externalID string) error {
	return a.modifyLabels(ctx, token, externalID, []string{"STARRED"}, nil)
}

// Unstar unstars a message.
func (a *GmailAdapter) Unstar(ctx context.Context, token *oauth2.Token, externalID string) error {
	return a.modifyLabels(ctx, token, externalID, nil, []string{"STARRED"})
}

// Archive archives a message.
func (a *GmailAdapter) Archive(ctx context.Context, token *oauth2.Token, externalID string) error {
	return a.modifyLabels(ctx, token, externalID, nil, []string{"INBOX"})
}

// Trash moves message to trash.
func (a *GmailAdapter) Trash(ctx context.Context, token *oauth2.Token, externalID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	_, err = svc.Users.Messages.Trash("me", externalID).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to trash message")
	}
	return nil
}

// Restore restores message from trash.
func (a *GmailAdapter) Restore(ctx context.Context, token *oauth2.Token, externalID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	_, err = svc.Users.Messages.Untrash("me", externalID).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to restore message")
	}
	return nil
}

// Delete permanently deletes a message.
func (a *GmailAdapter) Delete(ctx context.Context, token *oauth2.Token, externalID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	err = svc.Users.Messages.Delete("me", externalID).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to delete message")
	}
	return nil
}

// BatchModify modifies multiple messages.
func (a *GmailAdapter) BatchModify(ctx context.Context, token *oauth2.Token, req *out.ProviderBatchModifyRequest) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	batchReq := &gmail.BatchModifyMessagesRequest{
		Ids:            req.IDs,
		AddLabelIds:    req.AddLabels,
		RemoveLabelIds: req.RemoveLabels,
	}

	err = svc.Users.Messages.BatchModify("me", batchReq).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to batch modify")
	}
	return nil
}

// =============================================================================
// Labels
// =============================================================================

// ListLabels lists all labels.
func (a *GmailAdapter) ListLabels(ctx context.Context, token *oauth2.Token) ([]out.ProviderMailLabel, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	resp, err := svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to list labels")
	}

	labels := make([]out.ProviderMailLabel, len(resp.Labels))
	for i, l := range resp.Labels {
		labels[i] = out.ProviderMailLabel{
			ExternalID:     l.Id,
			Name:           l.Name,
			Type:           l.Type,
			MessagesTotal:  l.MessagesTotal,
			MessagesUnread: l.MessagesUnread,
		}
	}
	return labels, nil
}

// CreateLabel creates a new label.
func (a *GmailAdapter) CreateLabel(ctx context.Context, token *oauth2.Token, name string, color *string) (*out.ProviderMailLabel, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	label := &gmail.Label{
		Name:                  name,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}

	created, err := svc.Users.Labels.Create("me", label).Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to create label")
	}

	return &out.ProviderMailLabel{
		ExternalID: created.Id,
		Name:       created.Name,
		Type:       created.Type,
	}, nil
}

// DeleteLabel deletes a label.
func (a *GmailAdapter) DeleteLabel(ctx context.Context, token *oauth2.Token, labelID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	err = svc.Users.Labels.Delete("me", labelID).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to delete label")
	}
	return nil
}

// AddLabel adds a label to a message.
func (a *GmailAdapter) AddLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error {
	return a.modifyLabels(ctx, token, messageID, []string{labelID}, nil)
}

// RemoveLabel removes a label from a message.
func (a *GmailAdapter) RemoveLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error {
	return a.modifyLabels(ctx, token, messageID, nil, []string{labelID})
}

// =============================================================================
// Attachments
// =============================================================================

// GetAttachment retrieves an attachment.
func (a *GmailAdapter) GetAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) ([]byte, string, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, "", err
	}

	var att *gmail.MessagePartBody
	cbErr := a.executeWithCircuitBreaker(ctx, "GetAttachment", func() error {
		var apiErr error
		att, apiErr = svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
		return apiErr
	})
	if cbErr != nil {
		return nil, "", a.wrapError(cbErr, "failed to get attachment")
	}

	data, err := base64.URLEncoding.DecodeString(att.Data)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode attachment: %w", err)
	}

	return data, "", nil
}

// StreamAttachment streams an attachment.
func (a *GmailAdapter) StreamAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) (*out.ProviderAttachmentStream, error) {
	data, mimeType, err := a.GetAttachment(ctx, token, messageID, attachmentID)
	if err != nil {
		return nil, err
	}

	return &out.ProviderAttachmentStream{
		Reader:   strings.NewReader(string(data)),
		Size:     int64(len(data)),
		MimeType: mimeType,
	}, nil
}

// =============================================================================
// Upload Session (Resumable Upload for large attachments > 5MB)
// =============================================================================

const (
	gmailUploadBaseURL    = "https://www.googleapis.com/upload/gmail/v1/users/me/messages/send"
	gmailRecommendedChunk = 8 * 1024 * 1024  // 8MB recommended
	gmailMinChunkSize     = 256 * 1024       // 256KB minimum (must be multiple)
	gmailMaxChunkSize     = 50 * 1024 * 1024 // 50MB max per request
)

// CreateUploadSession creates a resumable upload session for large attachments.
// Gmail uses resumable upload protocol for files > 5MB.
// Returns uploadUrl that frontend can use to directly upload chunks to Gmail.
func (a *GmailAdapter) CreateUploadSession(ctx context.Context, token *oauth2.Token, messageID string, req *out.UploadSessionRequest) (*out.UploadSessionResponse, error) {
	// Gmail's resumable upload requires initiating with POST to upload endpoint
	// with X-Upload-Content-Type and X-Upload-Content-Length headers

	client := a.config.Client(ctx, token)

	// Build the initial resumable upload request
	// For Gmail, we need to create a draft first, then upload attachment
	httpReq, err := http.NewRequestWithContext(ctx, "POST", gmailUploadBaseURL+"?uploadType=resumable", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("X-Upload-Content-Type", req.MimeType)
	httpReq.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", req.Size))
	httpReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	httpReq.Header.Set("Content-Length", "0")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate upload session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create upload session: status %d, body: %s", resp.StatusCode, string(body))
	}

	// The upload URL is returned in the Location header
	uploadURL := resp.Header.Get("Location")
	if uploadURL == "" {
		return nil, fmt.Errorf("no upload URL returned from Gmail")
	}

	// Generate session ID for tracking
	sessionID := fmt.Sprintf("gmail_%s_%d", messageID, time.Now().UnixNano())

	return &out.UploadSessionResponse{
		SessionID:    sessionID,
		UploadURL:    uploadURL,
		ExpiresAt:    time.Now().Add(24 * time.Hour), // Gmail sessions expire in ~1 day
		ChunkSize:    gmailRecommendedChunk,
		MaxChunkSize: gmailMaxChunkSize,
		Provider:     "gmail",
	}, nil
}

// GetUploadSessionStatus checks the status of a resumable upload.
// For Gmail, we send an empty PUT with Content-Range: bytes */total to query status.
func (a *GmailAdapter) GetUploadSessionStatus(ctx context.Context, token *oauth2.Token, uploadURL string) (*out.UploadSessionStatus, error) {
	client := a.config.Client(ctx, token)

	// Query upload status with empty PUT request
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	httpReq.Header.Set("Content-Length", "0")
	httpReq.Header.Set("Content-Range", "bytes */*")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get upload status: %w", err)
	}
	defer resp.Body.Close()

	status := &out.UploadSessionStatus{
		SessionID: uploadURL,
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		// Upload complete
		status.IsComplete = true
		// Parse response for attachment ID if available
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if id, ok := result["id"].(string); ok {
				status.AttachmentID = id
			}
		}
	case 308: // Resume Incomplete
		// Parse Range header to get bytes received
		rangeHeader := resp.Header.Get("Range")
		if rangeHeader != "" {
			// Format: bytes=0-12345
			var end int64
			fmt.Sscanf(rangeHeader, "bytes=0-%d", &end)
			status.BytesUploaded = end + 1
			status.NextRangeStart = end + 1
		}
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return status, nil
}

// CancelUploadSession cancels a resumable upload session.
// For Gmail, we send DELETE to the upload URL.
func (a *GmailAdapter) CancelUploadSession(ctx context.Context, token *oauth2.Token, uploadURL string) error {
	client := a.config.Client(ctx, token)

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", uploadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create cancel request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to cancel upload: %w", err)
	}
	defer resp.Body.Close()

	// Gmail may return 404 if session already expired, which is fine
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to cancel upload: status %d", resp.StatusCode)
	}

	return nil
}

// =============================================================================
// Profile
// =============================================================================

// GetProfile retrieves user profile.
func (a *GmailAdapter) GetProfile(ctx context.Context, token *oauth2.Token) (*out.ProviderProfile, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, err
	}

	profile, err := svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return nil, a.wrapError(err, "failed to get profile")
	}

	return &out.ProviderProfile{
		Email:     profile.EmailAddress,
		HistoryID: profile.HistoryId,
	}, nil
}

// =============================================================================
// Internal Helpers
// =============================================================================

func (a *GmailAdapter) getService(ctx context.Context, token *oauth2.Token) (*gmail.Service, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	return gmail.NewService(ctx, option.WithTokenSource(
		a.config.TokenSource(ctx, token),
	))
}

// executeWithCircuitBreaker wraps an API call with circuit breaker protection.
// This prevents cascading failures when Gmail API is experiencing issues.
func (a *GmailAdapter) executeWithCircuitBreaker(ctx context.Context, operation string, fn func() error) error {
	_, err := a.cb.Execute(func() (interface{}, error) {
		if err := fn(); err != nil {
			// Check if this is a retriable error (server-side issues)
			if apiErr, ok := err.(*googleapi.Error); ok {
				switch apiErr.Code {
				case 500, 502, 503, 429:
					// These errors should trip the circuit breaker
					return nil, err
				case 400, 401, 403, 404:
					// Client errors should NOT trip the circuit breaker
					// 400: Invalid request (e.g., expired attachment token)
					// 401: Auth error
					// 403: Permission denied
					// 404: Not found
					// Wrap them to prevent circuit from opening
					return nil, &nonCircuitError{err: err}
				}
			}
			return nil, err
		}
		return nil, nil
	})

	// Unwrap non-circuit errors
	if nce, ok := err.(*nonCircuitError); ok {
		return nce.err
	}

	if err != nil {
		log.Printf("[GmailAdapter] Circuit breaker error for %s: state=%s, err=%v",
			operation, a.cb.State().String(), err)
	}

	return err
}

// nonCircuitError wraps errors that should not trip the circuit breaker.
type nonCircuitError struct {
	err error
}

func (e *nonCircuitError) Error() string {
	return e.err.Error()
}

// GetCircuitBreakerState returns the current state of the circuit breaker.
// Useful for monitoring and debugging.
func (a *GmailAdapter) GetCircuitBreakerState() string {
	return a.cb.State().String()
}

// IsCircuitOpen returns true if the circuit breaker is open (API calls will fail fast).
func (a *GmailAdapter) IsCircuitOpen() bool {
	return a.cb.State() == gobreaker.StateOpen
}

func (a *GmailAdapter) modifyLabels(ctx context.Context, token *oauth2.Token, messageID string, addLabels, removeLabels []string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return err
	}

	req := &gmail.ModifyMessageRequest{
		AddLabelIds:    addLabels,
		RemoveLabelIds: removeLabels,
	}

	_, err = svc.Users.Messages.Modify("me", messageID, req).Context(ctx).Do()
	if err != nil {
		return a.wrapError(err, "failed to modify labels")
	}
	return nil
}

func (a *GmailAdapter) convertMessage(msg *gmail.Message) out.ProviderMailMessage {
	result := out.ProviderMailMessage{
		ExternalID:       msg.Id,
		ExternalThreadID: msg.ThreadId,
		Labels:           msg.LabelIds,
		Size:             msg.SizeEstimate,
	}

	// Parse headers (including RFC classification headers)
	var classHeaders *out.ProviderClassificationHeaders
	if msg.Payload != nil {
		classHeaders = &out.ProviderClassificationHeaders{}
		hasClassificationHeaders := false

		for _, h := range msg.Payload.Headers {
			switch h.Name {
			// Basic headers
			case "Subject":
				result.Subject = h.Value
			case "From":
				result.From = a.parseEmailAddress(h.Value)
			case "To":
				result.To = a.parseEmailAddresses(h.Value)
			case "Cc":
				result.CC = a.parseEmailAddresses(h.Value)
			case "Bcc":
				result.BCC = a.parseEmailAddresses(h.Value)
			case "Date":
				if t, err := mail.ParseDate(h.Value); err == nil {
					result.Date = t
				}
			case "Message-ID":
				result.MessageID = h.Value
			case "In-Reply-To":
				result.InReplyTo = h.Value
			case "References":
				result.References = h.Value

			// RFC Classification Headers (Stage 0)
			case "List-Unsubscribe":
				classHeaders.ListUnsubscribe = h.Value
				hasClassificationHeaders = true
			case "List-Unsubscribe-Post":
				classHeaders.ListUnsubscribePost = h.Value
				hasClassificationHeaders = true
			case "List-Id":
				classHeaders.ListID = h.Value
				hasClassificationHeaders = true
			case "Precedence":
				classHeaders.Precedence = h.Value
				hasClassificationHeaders = true
			case "Auto-Submitted":
				classHeaders.AutoSubmitted = h.Value
				hasClassificationHeaders = true
			case "X-Auto-Response-Suppress":
				classHeaders.AutoResponseSuppress = h.Value
				hasClassificationHeaders = true
			case "X-Mailer":
				classHeaders.XMailer = h.Value
				hasClassificationHeaders = true
			case "Feedback-ID":
				classHeaders.FeedbackID = h.Value
				hasClassificationHeaders = true

			// ESP Detection Headers
			case "X-MC-User":
				classHeaders.IsMailchimp = true
				hasClassificationHeaders = true
			case "X-SG-EID":
				classHeaders.IsSendGrid = true
				hasClassificationHeaders = true
			case "X-SES-Outgoing":
				classHeaders.IsAmazonSES = true
				hasClassificationHeaders = true
			case "X-Mailgun-Variables":
				classHeaders.IsMailgun = true
				hasClassificationHeaders = true
			case "X-PM-Message-Id":
				classHeaders.IsPostmark = true
				hasClassificationHeaders = true
			case "X-Campaign-ID":
				classHeaders.IsCampaign = true
				hasClassificationHeaders = true

			// === Developer Service Headers ===

			// GitHub Headers
			case "X-GitHub-Reason":
				classHeaders.XGitHubReason = h.Value
				hasClassificationHeaders = true
			case "X-GitHub-Severity":
				classHeaders.XGitHubSeverity = h.Value
				hasClassificationHeaders = true
			case "X-GitHub-Sender":
				classHeaders.XGitHubSender = h.Value
				hasClassificationHeaders = true

			// GitLab Headers
			case "X-GitLab-Project":
				classHeaders.XGitLabProject = h.Value
				hasClassificationHeaders = true
			case "X-GitLab-Pipeline-Id":
				classHeaders.XGitLabPipelineID = h.Value
				hasClassificationHeaders = true
			case "X-GitLab-NotificationReason":
				classHeaders.XGitLabNotificationReason = h.Value
				hasClassificationHeaders = true

			// Jira/Atlassian Headers
			case "X-JIRA-FingerPrint":
				classHeaders.XJIRAFingerprint = h.Value
				hasClassificationHeaders = true

			// Linear Headers
			case "X-Linear-Team":
				classHeaders.XLinearTeam = h.Value
				hasClassificationHeaders = true

			// Sentry Headers
			case "X-Sentry-Project":
				classHeaders.XSentryProject = h.Value
				hasClassificationHeaders = true

			// Vercel Headers
			case "X-Vercel-Deployment-Url":
				classHeaders.XVercelDeploymentURL = h.Value
				hasClassificationHeaders = true

			// AWS Headers
			case "X-AWS-Service":
				classHeaders.XAWSService = h.Value
				hasClassificationHeaders = true
			}
		}

		// Extract CC addresses for notification type detection
		if len(result.CC) > 0 {
			if classHeaders == nil {
				classHeaders = &out.ProviderClassificationHeaders{}
			}
			classHeaders.CCAddresses = make([]string, len(result.CC))
			for i, cc := range result.CC {
				classHeaders.CCAddresses[i] = cc.Email
			}
			hasClassificationHeaders = true
		}

		// Only set if we found any classification headers
		if hasClassificationHeaders {
			result.ClassificationHeaders = classHeaders
		}
	}

	result.Snippet = msg.Snippet
	result.ReceivedAt = time.Unix(0, msg.InternalDate*int64(time.Millisecond))

	// Parse status from labels
	for _, label := range msg.LabelIds {
		switch label {
		case "UNREAD":
			result.IsRead = false
		case "STARRED":
			result.IsStarred = true
		case "INBOX":
			result.Folder = "inbox"
		case "SENT":
			result.Folder = "sent"
		case "DRAFT":
			result.Folder = "drafts"
		case "TRASH":
			result.Folder = "trash"
		case "SPAM":
			result.Folder = "spam"
		}
	}

	if result.Folder == "" {
		result.Folder = "inbox"
	}

	// Default to read if UNREAD not in labels
	if !contains(msg.LabelIds, "UNREAD") {
		result.IsRead = true
	}

	// Parse attachments
	if msg.Payload != nil {
		result.Attachments = a.extractAttachments(msg.Payload)
		result.HasAttachment = len(result.Attachments) > 0
	}

	// metadata format에서는 Payload.Parts가 없어서 extractAttachments가 빈 배열 반환
	// Content-Type 헤더 기반 감지는 false positive가 많아서 비활성화
	// 대신 Gmail의 has:attachment 쿼리 결과를 사용 (fetchMessagesParallelWithAttachmentInfo에서 설정)
	//
	// 참고: full format으로 가져온 경우에는 위의 extractAttachments에서 이미 정확하게 설정됨

	return result
}

func (a *GmailAdapter) extractBody(part *gmail.MessagePart, body *out.ProviderMessageBody, depth int) {
	if part == nil {
		return
	}

	// Debug logging for email structure
	hasData := part.Body != nil && part.Body.Data != ""
	log.Printf("[GmailAdapter] extractBody: depth=%d, mimeType=%s, hasData=%v, partsCount=%d",
		depth, part.MimeType, hasData, len(part.Parts))

	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		if data, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
			body.Text = string(data)
			log.Printf("[GmailAdapter] extractBody: extracted text/plain, len=%d", len(body.Text))
		} else {
			log.Printf("[GmailAdapter] extractBody: failed to decode text/plain: %v", err)
		}
	}

	if part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
		if data, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
			body.HTML = string(data)
			log.Printf("[GmailAdapter] extractBody: extracted text/html, len=%d", len(body.HTML))
		} else {
			log.Printf("[GmailAdapter] extractBody: failed to decode text/html: %v", err)
		}
	}

	for _, p := range part.Parts {
		a.extractBody(p, body, depth+1)
	}
}

func (a *GmailAdapter) extractAttachments(part *gmail.MessagePart) []out.ProviderMailAttachment {
	var attachments []out.ProviderMailAttachment

	// 첨부파일 감지 조건:
	// 1. full format: Filename + AttachmentId 둘 다 있음
	// 2. metadata format: Filename만 있고 AttachmentId는 비어있음
	// 따라서 Filename이 있으면 첨부파일로 인식 (AttachmentId는 선택적)
	if part.Filename != "" {
		att := out.ProviderMailAttachment{
			Filename: part.Filename,
			MimeType: part.MimeType,
		}

		// AttachmentId와 Size는 full format에서만 사용 가능
		if part.Body != nil {
			if part.Body.AttachmentId != "" {
				att.ID = part.Body.AttachmentId
			}
			att.Size = part.Body.Size
		}

		// Check for inline attachment (Content-ID header)
		for _, header := range part.Headers {
			if header.Name == "Content-ID" {
				// Remove angle brackets from Content-ID (e.g., "<image001>" -> "image001")
				cid := header.Value
				if len(cid) > 2 && cid[0] == '<' && cid[len(cid)-1] == '>' {
					cid = cid[1 : len(cid)-1]
				}
				att.ContentID = cid
				att.IsInline = true
				break
			}
		}

		// Also check Content-Disposition header for inline
		if !att.IsInline {
			for _, header := range part.Headers {
				if header.Name == "Content-Disposition" && strings.HasPrefix(header.Value, "inline") {
					att.IsInline = true
					break
				}
			}
		}

		attachments = append(attachments, att)
	}

	for _, p := range part.Parts {
		attachments = append(attachments, a.extractAttachments(p)...)
	}

	return attachments
}

func (a *GmailAdapter) parseEmailAddress(s string) out.ProviderEmailAddress {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return out.ProviderEmailAddress{Email: s}
	}
	return out.ProviderEmailAddress{
		Name:  addr.Name,
		Email: addr.Address,
	}
}

func (a *GmailAdapter) parseEmailAddresses(s string) []out.ProviderEmailAddress {
	list, err := mail.ParseAddressList(s)
	if err != nil {
		if s != "" {
			return []out.ProviderEmailAddress{{Email: s}}
		}
		return nil
	}

	result := make([]out.ProviderEmailAddress, len(list))
	for i, addr := range list {
		result[i] = out.ProviderEmailAddress{
			Name:  addr.Name,
			Email: addr.Address,
		}
	}
	return result
}

func (a *GmailAdapter) getHeader(headers []*gmail.MessagePartHeader, name string) string {
	for _, h := range headers {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}

func (a *GmailAdapter) buildRawMessage(msg *out.ProviderOutgoingMessage) string {
	var buf strings.Builder

	// Headers
	if len(msg.To) > 0 {
		buf.WriteString(fmt.Sprintf("To: %s\r\n", formatAddresses(msg.To)))
	}
	if len(msg.CC) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", formatAddresses(msg.CC)))
	}
	if len(msg.BCC) > 0 {
		buf.WriteString(fmt.Sprintf("Bcc: %s\r\n", formatAddresses(msg.BCC)))
	}
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))

	if msg.InReplyTo != "" {
		buf.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", msg.InReplyTo))
	}
	if msg.References != "" {
		buf.WriteString(fmt.Sprintf("References: %s\r\n", msg.References))
	}

	// Check if we have attachments
	if len(msg.Attachments) > 0 {
		// Use multipart/mixed for message with attachments
		boundary := fmt.Sprintf("boundary_%d", time.Now().UnixNano())
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
		buf.WriteString("\r\n")

		// Body part
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		contentType := "text/plain"
		if msg.IsHTML {
			contentType = "text/html"
		}
		buf.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
		buf.WriteString("\r\n")
		buf.WriteString(msg.Body)
		buf.WriteString("\r\n")

		// Attachment parts
		for _, att := range msg.Attachments {
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			buf.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.MimeType, att.Filename))
			buf.WriteString("Content-Transfer-Encoding: base64\r\n")
			buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", att.Filename))
			buf.WriteString("\r\n")
			buf.WriteString(base64.StdEncoding.EncodeToString(att.Data))
			buf.WriteString("\r\n")
		}

		// End boundary
		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		// Simple message without attachments
		contentType := "text/plain"
		if msg.IsHTML {
			contentType = "text/html"
		}
		buf.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
		buf.WriteString("\r\n")
		buf.WriteString(msg.Body)
	}

	return buf.String()
}

func (a *GmailAdapter) wrapError(err error, defaultMsg string) error {
	if err == nil {
		return nil
	}

	if apiErr, ok := err.(*googleapi.Error); ok {
		switch apiErr.Code {
		case 401:
			return out.NewProviderError("gmail", out.ProviderErrTokenExpired, "Token expired", err, false)
		case 403:
			if strings.Contains(apiErr.Message, "Rate Limit") {
				return out.NewProviderError("gmail", out.ProviderErrRateLimit, "Rate limit exceeded", err, true)
			}
			return out.NewProviderError("gmail", out.ProviderErrAuth, "Access denied", err, false)
		case 404:
			return out.NewProviderError("gmail", out.ProviderErrNotFound, "Not found", err, false)
		case 429:
			return out.NewProviderError("gmail", out.ProviderErrRateLimit, "Too many requests", err, true)
		case 500, 502, 503:
			return out.NewProviderError("gmail", out.ProviderErrServer, "Server error", err, true)
		}
	}

	return out.NewProviderError("gmail", out.ProviderErrServer, defaultMsg, err, true)
}

func formatAddresses(addrs []out.ProviderEmailAddress) string {
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		if a.Name != "" {
			parts[i] = fmt.Sprintf("%s <%s>", a.Name, a.Email)
		} else {
			parts[i] = a.Email
		}
	}
	return strings.Join(parts, ", ")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.EmailProviderPort = (*GmailAdapter)(nil)
