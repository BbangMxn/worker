// Package provider implements mail provider adapters.
package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/port/out"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

const graphBaseURL = "https://graph.microsoft.com/v1.0"

// =============================================================================
// Outlook Adapter
// =============================================================================

// OutlookAdapter implements out.EmailProviderPort for Microsoft Outlook/Graph API.
type OutlookAdapter struct {
	config *oauth2.Config
}

// NewOutlookAdapter creates a new Outlook adapter.
func NewOutlookAdapter(ctx context.Context, token *oauth2.Token, config *oauth2.Config) (*OutlookAdapter, error) {
	return &OutlookAdapter{
		config: config,
	}, nil
}

// NewOutlookAdapterWithConfig creates a new Outlook adapter with config.
func NewOutlookAdapterWithConfig(cfg *OutlookConfig) *OutlookAdapter {
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "common"
	}

	config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes: []string{
			"https://graph.microsoft.com/Mail.ReadWrite",
			"https://graph.microsoft.com/Mail.Send",
			"https://graph.microsoft.com/User.Read",
			"offline_access",
		},
		Endpoint: microsoft.AzureADEndpoint(tenantID),
	}

	return &OutlookAdapter{
		config: config,
	}
}

// GetProviderType returns the provider type.
func (a *OutlookAdapter) GetProviderType() string {
	return "outlook"
}

// =============================================================================
// Authentication
// =============================================================================

// GetAuthURL returns the OAuth authorization URL.
func (a *OutlookAdapter) GetAuthURL(state string) string {
	return a.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeToken exchanges authorization code for token.
func (a *OutlookAdapter) ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return nil, a.wrapError(err, "failed to exchange token")
	}
	return token, nil
}

// RefreshToken refreshes the access token.
func (a *OutlookAdapter) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	src := a.config.TokenSource(ctx, token)
	newToken, err := src.Token()
	if err != nil {
		return nil, a.wrapError(err, "failed to refresh token")
	}
	return newToken, nil
}

// ValidateToken validates the token.
func (a *OutlookAdapter) ValidateToken(ctx context.Context, token *oauth2.Token) (bool, error) {
	client := a.config.Client(ctx, token)
	resp, err := client.Get(graphBaseURL + "/me")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return false, nil
	}
	return resp.StatusCode == 200, nil
}

// =============================================================================
// Sync
// =============================================================================

// InitialSync performs initial mail sync.
func (a *OutlookAdapter) InitialSync(ctx context.Context, token *oauth2.Token, opts *out.ProviderSyncOptions) (*out.ProviderSyncResult, error) {
	client := a.config.Client(ctx, token)

	maxResults := 100
	if opts != nil && opts.MaxResults > 0 {
		maxResults = opts.MaxResults
	}

	params := url.Values{}
	params.Set("$top", fmt.Sprintf("%d", maxResults))
	params.Set("$orderby", "receivedDateTime desc")
	params.Set("$select", "id,conversationId,subject,bodyPreview,from,toRecipients,ccRecipients,isRead,flag,categories,hasAttachments,receivedDateTime,body")

	// 날짜 기반 필터 적용 (StartDate가 있으면 receivedDateTime 필터 추가)
	if opts != nil && opts.StartDate != nil {
		dateFilter := fmt.Sprintf("receivedDateTime ge %s", opts.StartDate.Format(time.RFC3339))
		params.Set("$filter", dateFilter)
	}

	var messages []out.ProviderMailMessage
	nextLink := graphBaseURL + "/me/messages?" + params.Encode()

	for nextLink != "" {
		var resp struct {
			Value    []graphMessage `json:"value"`
			NextLink string         `json:"@odata.nextLink"`
		}

		if err := a.doGet(client, nextLink, &resp); err != nil {
			return nil, err
		}

		for _, msg := range resp.Value {
			messages = append(messages, a.convertMessage(&msg))
		}

		if len(messages) >= maxResults {
			return &out.ProviderSyncResult{
				Messages:      messages[:maxResults],
				NextSyncState: resp.NextLink,
				HasMore:       true,
			}, nil
		}

		nextLink = resp.NextLink
	}

	// Get delta link for incremental sync
	deltaResp, err := a.getDeltaLink(client)
	if err != nil {
		return &out.ProviderSyncResult{
			Messages: messages,
			HasMore:  false,
		}, nil
	}

	return &out.ProviderSyncResult{
		Messages:      messages,
		NextSyncState: deltaResp,
		HasMore:       false,
	}, nil
}

// IncrementalSync performs incremental sync using delta.
func (a *OutlookAdapter) IncrementalSync(ctx context.Context, token *oauth2.Token, syncState string) (*out.ProviderSyncResult, error) {
	client := a.config.Client(ctx, token)

	var messages []out.ProviderMailMessage
	var deletedIDs []string

	nextLink := syncState
	if !strings.HasPrefix(nextLink, "http") {
		nextLink = graphBaseURL + "/me/messages/delta"
	}

	for nextLink != "" {
		var resp struct {
			Value     []graphMessage `json:"value"`
			NextLink  string         `json:"@odata.nextLink"`
			DeltaLink string         `json:"@odata.deltaLink"`
		}

		if err := a.doGet(client, nextLink, &resp); err != nil {
			// If delta token expired, need full sync
			if strings.Contains(err.Error(), "resyncRequired") || strings.Contains(err.Error(), "410") {
				return nil, out.NewProviderError("outlook", out.ProviderErrSyncRequired, "Full sync required", err, false)
			}
			return nil, err
		}

		for _, msg := range resp.Value {
			// Check if deleted (has @removed annotation)
			if msg.Removed != nil {
				deletedIDs = append(deletedIDs, msg.ID)
			} else {
				messages = append(messages, a.convertMessage(&msg))
			}
		}

		if resp.DeltaLink != "" {
			return &out.ProviderSyncResult{
				Messages:      messages,
				DeletedIDs:    deletedIDs,
				NextSyncState: resp.DeltaLink,
				HasMore:       false,
			}, nil
		}

		nextLink = resp.NextLink
	}

	return &out.ProviderSyncResult{
		Messages:   messages,
		DeletedIDs: deletedIDs,
		HasMore:    false,
	}, nil
}

// Watch sets up push notifications.
func (a *OutlookAdapter) Watch(ctx context.Context, token *oauth2.Token) (*out.ProviderWatchResponse, error) {
	// Outlook uses subscriptions, which are typically set up via webhook endpoint
	// This would be handled by a separate subscription management
	return nil, fmt.Errorf("watch not implemented - use subscription API")
}

// StopWatch stops push notifications.
func (a *OutlookAdapter) StopWatch(ctx context.Context, token *oauth2.Token) error {
	return nil
}

// =============================================================================
// Message Reading
// =============================================================================

// GetMessage retrieves a single message.
func (a *OutlookAdapter) GetMessage(ctx context.Context, token *oauth2.Token, externalID string) (*out.ProviderMailMessage, error) {
	client := a.config.Client(ctx, token)

	var msg graphMessage
	if err := a.doGet(client, graphBaseURL+"/me/messages/"+externalID, &msg); err != nil {
		return nil, err
	}

	result := a.convertMessage(&msg)
	return &result, nil
}

// GetMessageBody retrieves message body.
func (a *OutlookAdapter) GetMessageBody(ctx context.Context, token *oauth2.Token, externalID string) (*out.ProviderMessageBody, error) {
	client := a.config.Client(ctx, token)

	var msg struct {
		Body graphBody `json:"body"`
	}
	if err := a.doGet(client, graphBaseURL+"/me/messages/"+externalID+"?$select=body", &msg); err != nil {
		return nil, err
	}

	body := &out.ProviderMessageBody{}
	if msg.Body.ContentType == "html" {
		body.HTML = msg.Body.Content
	} else {
		body.Text = msg.Body.Content
	}

	// Fetch attachments
	attachments, err := a.listAttachments(ctx, client, externalID)
	if err == nil {
		body.Attachments = attachments
	}

	return body, nil
}

// listAttachments retrieves all attachments for a message.
func (a *OutlookAdapter) listAttachments(ctx context.Context, client *http.Client, messageID string) ([]out.ProviderMailAttachment, error) {
	var resp struct {
		Value []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			ContentType string `json:"contentType"`
			Size        int64  `json:"size"`
			ContentID   string `json:"contentId"`
			IsInline    bool   `json:"isInline"`
			ODataType   string `json:"@odata.type"`
		} `json:"value"`
	}

	if err := a.doGet(client, graphBaseURL+"/me/messages/"+messageID+"/attachments?$select=id,name,contentType,size,contentId,isInline", &resp); err != nil {
		return nil, err
	}

	attachments := make([]out.ProviderMailAttachment, 0, len(resp.Value))
	for _, att := range resp.Value {
		attachments = append(attachments, out.ProviderMailAttachment{
			ID:        att.ID,
			Filename:  att.Name,
			MimeType:  att.ContentType,
			Size:      att.Size,
			ContentID: att.ContentID,
			IsInline:  att.IsInline,
		})
	}

	return attachments, nil
}

// ListMessages lists messages with options.
func (a *OutlookAdapter) ListMessages(ctx context.Context, token *oauth2.Token, opts *out.ProviderListOptions) (*out.ProviderListResult, error) {
	client := a.config.Client(ctx, token)

	maxResults := 50
	if opts != nil && opts.MaxResults > 0 {
		maxResults = opts.MaxResults
	}

	params := url.Values{}
	params.Set("$top", fmt.Sprintf("%d", maxResults))
	params.Set("$orderby", "receivedDateTime desc")

	if opts != nil {
		if opts.Query != "" {
			params.Set("$search", fmt.Sprintf("\"%s\"", opts.Query))
		}
		if opts.PageToken != "" {
			params.Set("$skip", opts.PageToken)
		}
	}

	var resp struct {
		Value    []graphMessage `json:"value"`
		NextLink string         `json:"@odata.nextLink"`
		Count    int64          `json:"@odata.count"`
	}

	if err := a.doGet(client, graphBaseURL+"/me/messages?"+params.Encode(), &resp); err != nil {
		return nil, err
	}

	messages := make([]out.ProviderMailMessage, len(resp.Value))
	for i, msg := range resp.Value {
		messages[i] = a.convertMessage(&msg)
	}

	return &out.ProviderListResult{
		Messages:      messages,
		NextPageToken: extractSkipTokenFromURL(resp.NextLink),
		TotalCount:    resp.Count,
	}, nil
}

// =============================================================================
// Message Sending
// =============================================================================

// Send sends a new message.
func (a *OutlookAdapter) Send(ctx context.Context, token *oauth2.Token, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	client := a.config.Client(ctx, token)

	body := struct {
		Message         interface{} `json:"message"`
		SaveToSentItems bool        `json:"saveToSentItems"`
	}{
		Message:         a.buildGraphMessage(msg),
		SaveToSentItems: true,
	}

	if err := a.doPost(client, graphBaseURL+"/me/sendMail", body, nil); err != nil {
		return nil, err
	}

	return &out.ProviderSendResult{
		SentAt: time.Now(),
	}, nil
}

// Reply sends a reply.
func (a *OutlookAdapter) Reply(ctx context.Context, token *oauth2.Token, replyToID string, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	client := a.config.Client(ctx, token)

	body := struct {
		Message interface{} `json:"message"`
		Comment string      `json:"comment"`
	}{
		Message: a.buildGraphMessage(msg),
		Comment: msg.Body,
	}

	if err := a.doPost(client, graphBaseURL+"/me/messages/"+replyToID+"/reply", body, nil); err != nil {
		return nil, err
	}

	return &out.ProviderSendResult{
		SentAt: time.Now(),
	}, nil
}

// Forward forwards a message.
func (a *OutlookAdapter) Forward(ctx context.Context, token *oauth2.Token, forwardID string, msg *out.ProviderOutgoingMessage) (*out.ProviderSendResult, error) {
	return a.Send(ctx, token, msg)
}

// CreateDraft creates a draft.
func (a *OutlookAdapter) CreateDraft(ctx context.Context, token *oauth2.Token, msg *out.ProviderOutgoingMessage) (*out.ProviderDraftResult, error) {
	client := a.config.Client(ctx, token)

	graphMsg := a.buildGraphMessage(msg)

	var resp struct {
		ID string `json:"id"`
	}

	if err := a.doPost(client, graphBaseURL+"/me/messages", graphMsg, &resp); err != nil {
		return nil, err
	}

	return &out.ProviderDraftResult{
		ExternalID: resp.ID,
	}, nil
}

// UpdateDraft updates a draft.
func (a *OutlookAdapter) UpdateDraft(ctx context.Context, token *oauth2.Token, draftID string, msg *out.ProviderOutgoingMessage) (*out.ProviderDraftResult, error) {
	client := a.config.Client(ctx, token)

	graphMsg := a.buildGraphMessage(msg)

	if err := a.doPatch(client, graphBaseURL+"/me/messages/"+draftID, graphMsg); err != nil {
		return nil, err
	}

	return &out.ProviderDraftResult{
		ExternalID: draftID,
	}, nil
}

// DeleteDraft deletes a draft.
func (a *OutlookAdapter) DeleteDraft(ctx context.Context, token *oauth2.Token, draftID string) error {
	return a.Delete(ctx, token, draftID)
}

// SendDraft sends a draft.
func (a *OutlookAdapter) SendDraft(ctx context.Context, token *oauth2.Token, draftID string) (*out.ProviderSendResult, error) {
	client := a.config.Client(ctx, token)

	if err := a.doPost(client, graphBaseURL+"/me/messages/"+draftID+"/send", nil, nil); err != nil {
		return nil, err
	}

	return &out.ProviderSendResult{
		ExternalID: draftID,
		SentAt:     time.Now(),
	}, nil
}

// =============================================================================
// Message Modification
// =============================================================================

// MarkAsRead marks message as read.
func (a *OutlookAdapter) MarkAsRead(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPatch(client, graphBaseURL+"/me/messages/"+externalID, map[string]bool{"isRead": true})
}

// MarkAsUnread marks message as unread.
func (a *OutlookAdapter) MarkAsUnread(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPatch(client, graphBaseURL+"/me/messages/"+externalID, map[string]bool{"isRead": false})
}

// Star flags a message.
func (a *OutlookAdapter) Star(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPatch(client, graphBaseURL+"/me/messages/"+externalID, map[string]interface{}{
		"flag": map[string]string{"flagStatus": "flagged"},
	})
}

// Unstar removes flag from a message.
func (a *OutlookAdapter) Unstar(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPatch(client, graphBaseURL+"/me/messages/"+externalID, map[string]interface{}{
		"flag": map[string]string{"flagStatus": "notFlagged"},
	})
}

// Archive archives a message.
func (a *OutlookAdapter) Archive(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPost(client, graphBaseURL+"/me/messages/"+externalID+"/move", map[string]string{
		"destinationId": "archive",
	}, nil)
}

// Trash moves message to trash.
func (a *OutlookAdapter) Trash(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPost(client, graphBaseURL+"/me/messages/"+externalID+"/move", map[string]string{
		"destinationId": "deleteditems",
	}, nil)
}

// Restore restores message from trash.
func (a *OutlookAdapter) Restore(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doPost(client, graphBaseURL+"/me/messages/"+externalID+"/move", map[string]string{
		"destinationId": "inbox",
	}, nil)
}

// Delete permanently deletes a message.
func (a *OutlookAdapter) Delete(ctx context.Context, token *oauth2.Token, externalID string) error {
	client := a.config.Client(ctx, token)
	return a.doDelete(client, graphBaseURL+"/me/messages/"+externalID)
}

// BatchModify modifies multiple messages.
func (a *OutlookAdapter) BatchModify(ctx context.Context, token *oauth2.Token, req *out.ProviderBatchModifyRequest) error {
	// Outlook doesn't have a native batch modify, so we do it sequentially
	for _, id := range req.IDs {
		if len(req.AddLabels) > 0 {
			for _, label := range req.AddLabels {
				_ = a.AddLabel(ctx, token, id, label)
			}
		}
		if len(req.RemoveLabels) > 0 {
			for _, label := range req.RemoveLabels {
				_ = a.RemoveLabel(ctx, token, id, label)
			}
		}
	}
	return nil
}

// =============================================================================
// Labels (Categories in Outlook)
// =============================================================================

// ListLabels lists all categories.
func (a *OutlookAdapter) ListLabels(ctx context.Context, token *oauth2.Token) ([]out.ProviderMailLabel, error) {
	client := a.config.Client(ctx, token)

	var resp struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Color       string `json:"color"`
		} `json:"value"`
	}

	if err := a.doGet(client, graphBaseURL+"/me/outlook/masterCategories", &resp); err != nil {
		return nil, err
	}

	labels := make([]out.ProviderMailLabel, len(resp.Value))
	for i, c := range resp.Value {
		labels[i] = out.ProviderMailLabel{
			ExternalID: c.ID,
			Name:       c.DisplayName,
			Type:       "user",
		}
	}

	return labels, nil
}

// CreateLabel creates a new category.
func (a *OutlookAdapter) CreateLabel(ctx context.Context, token *oauth2.Token, name string, color *string) (*out.ProviderMailLabel, error) {
	client := a.config.Client(ctx, token)

	body := map[string]string{
		"displayName": name,
	}
	if color != nil {
		body["color"] = *color
	}

	var resp struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
	}

	if err := a.doPost(client, graphBaseURL+"/me/outlook/masterCategories", body, &resp); err != nil {
		return nil, err
	}

	return &out.ProviderMailLabel{
		ExternalID: resp.ID,
		Name:       resp.DisplayName,
		Type:       "user",
	}, nil
}

// DeleteLabel deletes a category.
func (a *OutlookAdapter) DeleteLabel(ctx context.Context, token *oauth2.Token, labelID string) error {
	client := a.config.Client(ctx, token)
	return a.doDelete(client, graphBaseURL+"/me/outlook/masterCategories/"+labelID)
}

// AddLabel adds a category to a message.
func (a *OutlookAdapter) AddLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error {
	client := a.config.Client(ctx, token)

	// Get current categories
	var msg struct {
		Categories []string `json:"categories"`
	}
	if err := a.doGet(client, graphBaseURL+"/me/messages/"+messageID+"?$select=categories", &msg); err != nil {
		return err
	}

	// Check if already has this category
	for _, c := range msg.Categories {
		if c == labelID {
			return nil
		}
	}

	msg.Categories = append(msg.Categories, labelID)
	return a.doPatch(client, graphBaseURL+"/me/messages/"+messageID, msg)
}

// RemoveLabel removes a category from a message.
func (a *OutlookAdapter) RemoveLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error {
	client := a.config.Client(ctx, token)

	var msg struct {
		Categories []string `json:"categories"`
	}
	if err := a.doGet(client, graphBaseURL+"/me/messages/"+messageID+"?$select=categories", &msg); err != nil {
		return err
	}

	newCategories := make([]string, 0, len(msg.Categories))
	for _, c := range msg.Categories {
		if c != labelID {
			newCategories = append(newCategories, c)
		}
	}
	msg.Categories = newCategories

	return a.doPatch(client, graphBaseURL+"/me/messages/"+messageID, msg)
}

// =============================================================================
// Attachments
// =============================================================================

// GetAttachment retrieves an attachment.
func (a *OutlookAdapter) GetAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) ([]byte, string, error) {
	client := a.config.Client(ctx, token)

	var resp struct {
		ContentBytes string `json:"contentBytes"`
		ContentType  string `json:"contentType"`
	}

	if err := a.doGet(client, graphBaseURL+"/me/messages/"+messageID+"/attachments/"+attachmentID, &resp); err != nil {
		return nil, "", err
	}

	// Decode base64 content
	data := []byte(resp.ContentBytes) // Graph API returns raw bytes for $value endpoint

	return data, resp.ContentType, nil
}

// StreamAttachment streams an attachment.
func (a *OutlookAdapter) StreamAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) (*out.ProviderAttachmentStream, error) {
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
// Upload Session (for large attachments 3MB ~ 150MB)
// =============================================================================

const (
	outlookRecommendedChunk  = 4 * 1024 * 1024   // 4MB recommended (max per request)
	outlookMinChunkSize      = 320 * 1024        // 320KB minimum (must be multiple)
	outlookMaxAttachmentSize = 150 * 1024 * 1024 // 150MB max total
)

// CreateUploadSession creates an upload session for large attachments.
// Outlook uses createUploadSession API for files 3MB ~ 150MB.
// Returns uploadUrl that frontend can use to directly upload chunks to Microsoft Graph.
func (a *OutlookAdapter) CreateUploadSession(ctx context.Context, token *oauth2.Token, messageID string, req *out.UploadSessionRequest) (*out.UploadSessionResponse, error) {
	client := a.config.Client(ctx, token)

	// Request body for createUploadSession
	body := map[string]interface{}{
		"AttachmentItem": map[string]interface{}{
			"attachmentType": "file",
			"name":           req.Filename,
			"size":           req.Size,
			"contentType":    req.MimeType,
		},
	}

	// Add inline attachment properties if needed
	if req.IsInline && req.ContentID != "" {
		body["AttachmentItem"].(map[string]interface{})["isInline"] = true
		body["AttachmentItem"].(map[string]interface{})["contentId"] = req.ContentID
	}

	var resp struct {
		UploadURL          string `json:"uploadUrl"`
		ExpirationDateTime string `json:"expirationDateTime"`
	}

	endpoint := fmt.Sprintf("%s/me/messages/%s/attachments/createUploadSession", graphBaseURL, messageID)
	if err := a.doPost(client, endpoint, body, &resp); err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}

	if resp.UploadURL == "" {
		return nil, fmt.Errorf("no upload URL returned from Outlook")
	}

	// Parse expiration time
	expiresAt, _ := time.Parse(time.RFC3339, resp.ExpirationDateTime)
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(1 * time.Hour) // Default 1 hour
	}

	// Generate session ID for tracking
	sessionID := fmt.Sprintf("outlook_%s_%d", messageID, time.Now().UnixNano())

	return &out.UploadSessionResponse{
		SessionID:    sessionID,
		UploadURL:    resp.UploadURL,
		ExpiresAt:    expiresAt,
		ChunkSize:    outlookRecommendedChunk,
		MaxChunkSize: outlookRecommendedChunk, // Outlook max is 4MB per chunk
		Provider:     "outlook",
	}, nil
}

// GetUploadSessionStatus checks the status of an upload session.
// For Outlook, we GET the upload URL to check status.
func (a *OutlookAdapter) GetUploadSessionStatus(ctx context.Context, token *oauth2.Token, uploadURL string) (*out.UploadSessionStatus, error) {
	client := a.config.Client(ctx, token)

	// Note: Outlook upload URLs are pre-authenticated, don't need Authorization header
	httpReq, err := http.NewRequestWithContext(ctx, "GET", uploadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get upload status: %w", err)
	}
	defer resp.Body.Close()

	status := &out.UploadSessionStatus{
		SessionID: uploadURL,
	}

	if resp.StatusCode == http.StatusOK {
		var result struct {
			ExpirationDateTime string   `json:"expirationDateTime"`
			NextExpectedRanges []string `json:"nextExpectedRanges"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			// Parse next expected ranges to determine upload progress
			if len(result.NextExpectedRanges) > 0 {
				// Format: "0-" or "12345-67890"
				var start int64
				fmt.Sscanf(result.NextExpectedRanges[0], "%d-", &start)
				status.BytesUploaded = start
				status.NextRangeStart = start
			}
		}
	} else if resp.StatusCode == http.StatusCreated {
		// Upload complete
		status.IsComplete = true
		var result struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			status.AttachmentID = result.ID
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get upload status: %d - %s", resp.StatusCode, string(body))
	}

	return status, nil
}

// CancelUploadSession cancels an upload session.
// For Outlook, we DELETE the upload URL.
func (a *OutlookAdapter) CancelUploadSession(ctx context.Context, token *oauth2.Token, uploadURL string) error {
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

	// 204 No Content or 404 Not Found are both acceptable
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to cancel upload: status %d", resp.StatusCode)
	}

	return nil
}

// =============================================================================
// Profile
// =============================================================================

// GetProfile retrieves user profile.
func (a *OutlookAdapter) GetProfile(ctx context.Context, token *oauth2.Token) (*out.ProviderProfile, error) {
	client := a.config.Client(ctx, token)

	var user struct {
		ID   string `json:"id"`
		Mail string `json:"mail"`
		Name string `json:"displayName"`
	}

	if err := a.doGet(client, graphBaseURL+"/me", &user); err != nil {
		return nil, err
	}

	return &out.ProviderProfile{
		Email: user.Mail,
		Name:  user.Name,
	}, nil
}

// =============================================================================
// Internal Helpers
// =============================================================================

func (a *OutlookAdapter) doGet(client *http.Client, url string, result interface{}) error {
	resp, err := client.Get(url)
	if err != nil {
		return a.wrapError(err, "request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return a.wrapHTTPError(resp.StatusCode, string(body))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (a *OutlookAdapter) doPost(client *http.Client, url string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return a.wrapError(err, "request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return a.wrapHTTPError(resp.StatusCode, string(respBody))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (a *OutlookAdapter) doPatch(client *http.Client, url string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return a.wrapError(err, "request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return a.wrapHTTPError(resp.StatusCode, string(respBody))
	}

	return nil
}

func (a *OutlookAdapter) doDelete(client *http.Client, url string) error {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return a.wrapError(err, "request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return a.wrapHTTPError(resp.StatusCode, string(body))
	}

	return nil
}

func (a *OutlookAdapter) getDeltaLink(client *http.Client) (string, error) {
	var resp struct {
		DeltaLink string `json:"@odata.deltaLink"`
	}

	if err := a.doGet(client, graphBaseURL+"/me/messages/delta?$top=1", &resp); err != nil {
		return "", err
	}

	return resp.DeltaLink, nil
}

func (a *OutlookAdapter) convertMessage(msg *graphMessage) out.ProviderMailMessage {
	result := out.ProviderMailMessage{
		ExternalID:       msg.ID,
		ExternalThreadID: msg.ConversationID,
		Subject:          msg.Subject,
		Snippet:          msg.BodyPreview,
		IsRead:           msg.IsRead,
		IsStarred:        msg.Flag.FlagStatus == "flagged",
		Labels:           msg.Categories,
		HasAttachment:    msg.HasAttachments,
		Folder:           "inbox", // Default
	}

	// From
	if msg.From.EmailAddress.Address != "" {
		result.From = out.ProviderEmailAddress{
			Name:  msg.From.EmailAddress.Name,
			Email: msg.From.EmailAddress.Address,
		}
	}

	// To
	result.To = make([]out.ProviderEmailAddress, len(msg.ToRecipients))
	for i, r := range msg.ToRecipients {
		result.To[i] = out.ProviderEmailAddress{
			Name:  r.EmailAddress.Name,
			Email: r.EmailAddress.Address,
		}
	}

	// CC
	result.CC = make([]out.ProviderEmailAddress, len(msg.CcRecipients))
	for i, r := range msg.CcRecipients {
		result.CC[i] = out.ProviderEmailAddress{
			Name:  r.EmailAddress.Name,
			Email: r.EmailAddress.Address,
		}
	}

	// Body
	if msg.Body.ContentType == "html" {
		// Body is not included by default to reduce payload
	}

	// Parse received time
	if msg.ReceivedDateTime != "" {
		result.ReceivedAt, _ = time.Parse(time.RFC3339, msg.ReceivedDateTime)
	}

	return result
}

func (a *OutlookAdapter) buildGraphMessage(msg *out.ProviderOutgoingMessage) map[string]interface{} {
	contentType := "html"
	if !msg.IsHTML {
		contentType = "text"
	}

	result := map[string]interface{}{
		"subject": msg.Subject,
		"body": map[string]string{
			"contentType": contentType,
			"content":     msg.Body,
		},
	}

	if len(msg.To) > 0 {
		toRecipients := make([]map[string]interface{}, len(msg.To))
		for i, addr := range msg.To {
			toRecipients[i] = map[string]interface{}{
				"emailAddress": map[string]string{
					"name":    addr.Name,
					"address": addr.Email,
				},
			}
		}
		result["toRecipients"] = toRecipients
	}

	if len(msg.CC) > 0 {
		ccRecipients := make([]map[string]interface{}, len(msg.CC))
		for i, addr := range msg.CC {
			ccRecipients[i] = map[string]interface{}{
				"emailAddress": map[string]string{
					"name":    addr.Name,
					"address": addr.Email,
				},
			}
		}
		result["ccRecipients"] = ccRecipients
	}

	// Add attachments (base64 encoded for small files < 4MB)
	if len(msg.Attachments) > 0 {
		attachments := make([]map[string]interface{}, len(msg.Attachments))
		for i, att := range msg.Attachments {
			attachments[i] = map[string]interface{}{
				"@odata.type":  "#microsoft.graph.fileAttachment",
				"name":         att.Filename,
				"contentType":  att.MimeType,
				"contentBytes": base64.StdEncoding.EncodeToString(att.Data),
			}
		}
		result["attachments"] = attachments
	}

	return result
}

func (a *OutlookAdapter) wrapError(err error, defaultMsg string) error {
	if err == nil {
		return nil
	}
	return out.NewProviderError("outlook", out.ProviderErrServer, defaultMsg, err, true)
}

func (a *OutlookAdapter) wrapHTTPError(statusCode int, body string) error {
	switch statusCode {
	case 401:
		return out.NewProviderError("outlook", out.ProviderErrTokenExpired, "Token expired", nil, false)
	case 403:
		return out.NewProviderError("outlook", out.ProviderErrAuth, "Access denied", nil, false)
	case 404:
		return out.NewProviderError("outlook", out.ProviderErrNotFound, "Not found", nil, false)
	case 429:
		return out.NewProviderError("outlook", out.ProviderErrRateLimit, "Too many requests", nil, true)
	case 410:
		return out.NewProviderError("outlook", out.ProviderErrSyncRequired, "Full sync required", nil, false)
	default:
		return out.NewProviderError("outlook", out.ProviderErrServer, fmt.Sprintf("HTTP %d: %s", statusCode, body), nil, true)
	}
}

func extractSkipTokenFromURL(nextLink string) string {
	if nextLink == "" {
		return ""
	}
	u, err := url.Parse(nextLink)
	if err != nil {
		return ""
	}
	return u.Query().Get("$skip")
}

// Graph API types

type graphMessage struct {
	ID               string            `json:"id"`
	ConversationID   string            `json:"conversationId"`
	Subject          string            `json:"subject"`
	BodyPreview      string            `json:"bodyPreview"`
	Body             graphBody         `json:"body"`
	From             graphRecipient    `json:"from"`
	ToRecipients     []graphRecipient  `json:"toRecipients"`
	CcRecipients     []graphRecipient  `json:"ccRecipients"`
	BccRecipients    []graphRecipient  `json:"bccRecipients"`
	IsRead           bool              `json:"isRead"`
	Flag             graphFlag         `json:"flag"`
	Categories       []string          `json:"categories"`
	HasAttachments   bool              `json:"hasAttachments"`
	ReceivedDateTime string            `json:"receivedDateTime"`
	Removed          *graphRemovedInfo `json:"@removed,omitempty"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphRecipient struct {
	EmailAddress graphEmailAddress `json:"emailAddress"`
}

type graphEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type graphFlag struct {
	FlagStatus string `json:"flagStatus"`
}

type graphRemovedInfo struct {
	Reason string `json:"reason"`
}

var _ out.EmailProviderPort = (*OutlookAdapter)(nil)
