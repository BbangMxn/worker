// Package outlook provides Microsoft Outlook/Graph API adapter.
package outlook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/port/out"

	"golang.org/x/oauth2"
)

const graphBaseURL = "https://graph.microsoft.com/v1.0"

// Provider implements out.EmailProvider for Outlook.
type Provider struct {
	client *http.Client
	email  string
}

// NewProvider creates a new Outlook provider.
func NewProvider(ctx context.Context, token *oauth2.Token, config *oauth2.Config) (*Provider, error) {
	client := config.Client(ctx, token)

	p := &Provider{
		client: client,
	}

	// Get user email
	profile, err := p.getProfile(ctx)
	if err != nil {
		return nil, err
	}
	p.email = profile.Mail

	return p, nil
}

// GetProviderName returns the provider name.
func (p *Provider) GetProviderName() string {
	return "outlook"
}

// GetEmail returns the user's email.
func (p *Provider) GetEmail() string {
	return p.email
}

// GetMessage retrieves a message by ID.
func (p *Provider) GetMessage(ctx context.Context, messageID string) (*out.ProviderMessage, error) {
	var msg graphMessage
	err := p.get(ctx, fmt.Sprintf("/me/messages/%s", messageID), &msg)
	if err != nil {
		return nil, err
	}

	return convertMessage(&msg), nil
}

// ListMessages lists messages.
func (p *Provider) ListMessages(ctx context.Context, query *out.ProviderListQuery) (*out.ProviderMessageListResult, error) {
	params := url.Values{}
	params.Set("$top", fmt.Sprintf("%d", query.PageSize))
	params.Set("$orderby", "receivedDateTime desc")

	if query.PageToken != "" {
		// PageToken is the $skip value
		params.Set("$skip", query.PageToken)
	}
	if query.Query != "" {
		params.Set("$search", fmt.Sprintf("\"%s\"", query.Query))
	}

	var resp struct {
		Value    []graphMessage `json:"value"`
		NextLink string         `json:"@odata.nextLink"`
	}

	err := p.get(ctx, "/me/messages?"+params.Encode(), &resp)
	if err != nil {
		return nil, err
	}

	messages := make([]*out.ProviderMessage, len(resp.Value))
	for i, m := range resp.Value {
		messages[i] = convertMessage(&m)
	}

	return &out.ProviderMessageListResult{
		Messages:      messages,
		NextPageToken: extractSkipToken(resp.NextLink),
		ResultSize:    len(messages),
	}, nil
}

// SendMessage sends a message.
func (p *Provider) SendMessage(ctx context.Context, req *out.ProviderSendRequest) (*out.ProviderMessage, error) {
	msg := buildGraphMessage(req)

	body := struct {
		Message         graphMessage `json:"message"`
		SaveToSentItems bool         `json:"saveToSentItems"`
	}{
		Message:         msg,
		SaveToSentItems: true,
	}

	err := p.post(ctx, "/me/sendMail", body, nil)
	if err != nil {
		return nil, err
	}

	// Outlook sendMail doesn't return the sent message
	return nil, nil
}

// MarkRead marks a message as read.
func (p *Provider) MarkRead(ctx context.Context, messageID string) error {
	return p.patch(ctx, fmt.Sprintf("/me/messages/%s", messageID), map[string]bool{
		"isRead": true,
	})
}

// MarkUnread marks a message as unread.
func (p *Provider) MarkUnread(ctx context.Context, messageID string) error {
	return p.patch(ctx, fmt.Sprintf("/me/messages/%s", messageID), map[string]bool{
		"isRead": false,
	})
}

// Archive moves a message to archive.
func (p *Provider) Archive(ctx context.Context, messageID string) error {
	// Get archive folder ID
	var folder struct {
		ID string `json:"id"`
	}
	err := p.get(ctx, "/me/mailFolders/archive", &folder)
	if err != nil {
		return err
	}

	return p.post(ctx, fmt.Sprintf("/me/messages/%s/move", messageID), map[string]string{
		"destinationId": folder.ID,
	}, nil)
}

// Trash moves a message to trash.
func (p *Provider) Trash(ctx context.Context, messageID string) error {
	return p.post(ctx, fmt.Sprintf("/me/messages/%s/move", messageID), map[string]string{
		"destinationId": "deleteditems",
	}, nil)
}

// Delete permanently deletes a message.
func (p *Provider) Delete(ctx context.Context, messageID string) error {
	return p.delete(ctx, fmt.Sprintf("/me/messages/%s", messageID))
}

// Star flags a message (Outlook uses flags instead of stars).
func (p *Provider) Star(ctx context.Context, messageID string) error {
	return p.patch(ctx, fmt.Sprintf("/me/messages/%s", messageID), map[string]interface{}{
		"flag": map[string]string{
			"flagStatus": "flagged",
		},
	})
}

// Unstar removes flag from a message.
func (p *Provider) Unstar(ctx context.Context, messageID string) error {
	return p.patch(ctx, fmt.Sprintf("/me/messages/%s", messageID), map[string]interface{}{
		"flag": map[string]string{
			"flagStatus": "notFlagged",
		},
	})
}

// AddLabel adds a category (Outlook uses categories instead of labels).
func (p *Provider) AddLabel(ctx context.Context, messageID string, label string) error {
	// Get current categories
	var msg struct {
		Categories []string `json:"categories"`
	}
	err := p.get(ctx, fmt.Sprintf("/me/messages/%s?$select=categories", messageID), &msg)
	if err != nil {
		return err
	}

	// Add new category if not exists
	for _, c := range msg.Categories {
		if c == label {
			return nil
		}
	}
	msg.Categories = append(msg.Categories, label)

	return p.patch(ctx, fmt.Sprintf("/me/messages/%s", messageID), msg)
}

// RemoveLabel removes a category.
func (p *Provider) RemoveLabel(ctx context.Context, messageID string, label string) error {
	var msg struct {
		Categories []string `json:"categories"`
	}
	err := p.get(ctx, fmt.Sprintf("/me/messages/%s?$select=categories", messageID), &msg)
	if err != nil {
		return err
	}

	// Remove category
	newCategories := make([]string, 0, len(msg.Categories))
	for _, c := range msg.Categories {
		if c != label {
			newCategories = append(newCategories, c)
		}
	}
	msg.Categories = newCategories

	return p.patch(ctx, fmt.Sprintf("/me/messages/%s", messageID), msg)
}

// GetLabels retrieves all categories.
func (p *Provider) GetLabels(ctx context.Context) ([]*out.ProviderLabel, error) {
	var resp struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Color       string `json:"color"`
		} `json:"value"`
	}

	err := p.get(ctx, "/me/outlook/masterCategories", &resp)
	if err != nil {
		return nil, err
	}

	labels := make([]*out.ProviderLabel, len(resp.Value))
	for i, c := range resp.Value {
		labels[i] = &out.ProviderLabel{
			ID:   c.ID,
			Name: c.DisplayName,
			Type: "user",
		}
	}

	return labels, nil
}

// Subscribe creates a push notification subscription.
func (p *Provider) Subscribe(ctx context.Context, webhookURL string) (*out.ProviderSubscription, error) {
	expiresAt := time.Now().Add(4230 * time.Minute) // Max ~3 days

	body := map[string]interface{}{
		"changeType":         "created,updated",
		"notificationUrl":    webhookURL,
		"resource":           "me/mailFolders('Inbox')/messages",
		"expirationDateTime": expiresAt.Format(time.RFC3339),
		"clientState":        "bridgify-subscription",
	}

	var resp struct {
		ID                 string `json:"id"`
		ExpirationDateTime string `json:"expirationDateTime"`
	}

	err := p.post(ctx, "/subscriptions", body, &resp)
	if err != nil {
		return nil, err
	}

	exp, _ := time.Parse(time.RFC3339, resp.ExpirationDateTime)

	return &out.ProviderSubscription{
		ID:        resp.ID,
		ExpiresAt: exp,
	}, nil
}

// Unsubscribe removes a subscription.
func (p *Provider) Unsubscribe(ctx context.Context, subscriptionID string) error {
	return p.delete(ctx, fmt.Sprintf("/subscriptions/%s", subscriptionID))
}

// RenewSubscription renews a subscription.
func (p *Provider) RenewSubscription(ctx context.Context, subscriptionID string) (*out.ProviderSubscription, error) {
	expiresAt := time.Now().Add(4230 * time.Minute)

	body := map[string]string{
		"expirationDateTime": expiresAt.Format(time.RFC3339),
	}

	var resp struct {
		ID                 string `json:"id"`
		ExpirationDateTime string `json:"expirationDateTime"`
	}

	err := p.patch(ctx, fmt.Sprintf("/subscriptions/%s", subscriptionID), body)
	if err != nil {
		return nil, err
	}

	// Re-fetch to get updated expiration
	err = p.get(ctx, fmt.Sprintf("/subscriptions/%s", subscriptionID), &resp)
	if err != nil {
		return nil, err
	}

	exp, _ := time.Parse(time.RFC3339, resp.ExpirationDateTime)

	return &out.ProviderSubscription{
		ID:        resp.ID,
		ExpiresAt: exp,
	}, nil
}

// HTTP helpers

func (p *Provider) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", graphBaseURL+path, nil)
	if err != nil {
		return err
	}

	return p.doRequest(req, result)
}

func (p *Provider) post(ctx context.Context, path string, body interface{}, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", graphBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return p.doRequest(req, result)
}

func (p *Provider) patch(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", graphBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return p.doRequest(req, nil)
}

func (p *Provider) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", graphBaseURL+path, nil)
	if err != nil {
		return err
	}

	return p.doRequest(req, nil)
}

func (p *Provider) doRequest(req *http.Request, result interface{}) error {
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("graph API error: %d - %s", resp.StatusCode, string(body))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (p *Provider) getProfile(ctx context.Context) (*graphUser, error) {
	var user graphUser
	err := p.get(ctx, "/me", &user)
	return &user, err
}

// Graph API types

type graphUser struct {
	ID   string `json:"id"`
	Mail string `json:"mail"`
}

type graphMessage struct {
	ID               string           `json:"id"`
	ConversationID   string           `json:"conversationId"`
	Subject          string           `json:"subject"`
	BodyPreview      string           `json:"bodyPreview"`
	Body             graphBody        `json:"body"`
	From             graphRecipient   `json:"from"`
	ToRecipients     []graphRecipient `json:"toRecipients"`
	CcRecipients     []graphRecipient `json:"ccRecipients"`
	BccRecipients    []graphRecipient `json:"bccRecipients"`
	IsRead           bool             `json:"isRead"`
	Flag             graphFlag        `json:"flag"`
	Categories       []string         `json:"categories"`
	HasAttachments   bool             `json:"hasAttachments"`
	ReceivedDateTime string           `json:"receivedDateTime"`
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

func convertMessage(msg *graphMessage) *out.ProviderMessage {
	pm := &out.ProviderMessage{
		ID:        msg.ID,
		ThreadID:  msg.ConversationID,
		Subject:   msg.Subject,
		Snippet:   msg.BodyPreview,
		From:      formatAddress(msg.From),
		IsRead:    msg.IsRead,
		IsStarred: msg.Flag.FlagStatus == "flagged",
		Labels:    msg.Categories,
	}

	pm.To = make([]string, len(msg.ToRecipients))
	for i, r := range msg.ToRecipients {
		pm.To[i] = formatAddress(r)
	}

	pm.Cc = make([]string, len(msg.CcRecipients))
	for i, r := range msg.CcRecipients {
		pm.Cc[i] = formatAddress(r)
	}

	pm.Bcc = make([]string, len(msg.BccRecipients))
	for i, r := range msg.BccRecipients {
		pm.Bcc[i] = formatAddress(r)
	}

	if msg.Body.ContentType == "html" {
		pm.BodyHTML = msg.Body.Content
	} else {
		pm.BodyText = msg.Body.Content
	}

	pm.ReceivedAt, _ = time.Parse(time.RFC3339, msg.ReceivedDateTime)

	return pm
}

func formatAddress(r graphRecipient) string {
	if r.EmailAddress.Name != "" {
		return fmt.Sprintf("%s <%s>", r.EmailAddress.Name, r.EmailAddress.Address)
	}
	return r.EmailAddress.Address
}

func buildGraphMessage(req *out.ProviderSendRequest) graphMessage {
	msg := graphMessage{
		Subject: req.Subject,
		Body: graphBody{
			ContentType: "html",
			Content:     req.BodyHTML,
		},
	}

	msg.ToRecipients = make([]graphRecipient, len(req.To))
	for i, addr := range req.To {
		msg.ToRecipients[i] = graphRecipient{
			EmailAddress: graphEmailAddress{Address: addr},
		}
	}

	msg.CcRecipients = make([]graphRecipient, len(req.Cc))
	for i, addr := range req.Cc {
		msg.CcRecipients[i] = graphRecipient{
			EmailAddress: graphEmailAddress{Address: addr},
		}
	}

	msg.BccRecipients = make([]graphRecipient, len(req.Bcc))
	for i, addr := range req.Bcc {
		msg.BccRecipients[i] = graphRecipient{
			EmailAddress: graphEmailAddress{Address: addr},
		}
	}

	return msg
}

func extractSkipToken(nextLink string) string {
	if nextLink == "" {
		return ""
	}
	u, err := url.Parse(nextLink)
	if err != nil {
		return ""
	}
	return u.Query().Get("$skip")
}

// Ensure Provider implements out.EmailProvider
var _ out.EmailProvider = (*Provider)(nil)
