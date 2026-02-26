package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/agent/llm"
	"worker_server/core/agent/rag"
	"worker_server/core/agent/session"
	"worker_server/core/agent/tools"
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// OAuthTokenProvider interface for getting OAuth tokens
type OAuthTokenProvider interface {
	GetOAuth2Token(ctx context.Context, connectionID int64) (*oauth2.Token, error)
	GetConnectionByUserID(ctx context.Context, userID uuid.UUID, provider string) (*domain.OAuthConnection, error)
}

// Orchestrator is the central AI Agent that manages all operations
type Orchestrator struct {
	llmClient      *llm.Client
	ragRetriever   *rag.Retriever
	toolRegistry   *tools.Registry
	proposalStore  *session.ProposalStore
	sessionManager *session.Manager

	// Dependencies for proposal execution
	emailProvider     out.EmailProviderPort
	calendarProvider out.CalendarProviderPort
	oauthProvider    OAuthTokenProvider
	labelRepo        domain.LabelRepository
}

// NewOrchestrator creates a new AI Agent orchestrator
func NewOrchestrator(
	llmClient *llm.Client,
	ragRetriever *rag.Retriever,
	toolRegistry *tools.Registry,
) *Orchestrator {
	return &Orchestrator{
		llmClient:      llmClient,
		ragRetriever:   ragRetriever,
		toolRegistry:   toolRegistry,
		proposalStore:  session.NewProposalStore(),
		sessionManager: session.NewManager(),
	}
}

// NewOrchestratorFull creates orchestrator with all dependencies for proposal execution
func NewOrchestratorFull(
	llmClient *llm.Client,
	ragRetriever *rag.Retriever,
	toolRegistry *tools.Registry,
	emailProvider out.EmailProviderPort,
	oauthProvider OAuthTokenProvider,
) *Orchestrator {
	return &Orchestrator{
		llmClient:      llmClient,
		ragRetriever:   ragRetriever,
		toolRegistry:   toolRegistry,
		proposalStore:  session.NewProposalStore(),
		sessionManager: session.NewManager(),
		emailProvider:   emailProvider,
		oauthProvider:  oauthProvider,
	}
}

// SetMailProvider sets the mail provider for proposal execution
func (o *Orchestrator) SetMailProvider(provider out.EmailProviderPort) {
	o.emailProvider = provider
}

// SetOAuthProvider sets the OAuth provider for token management
func (o *Orchestrator) SetOAuthProvider(provider OAuthTokenProvider) {
	o.oauthProvider = provider
}

// SetCalendarProvider sets the calendar provider for proposal execution
func (o *Orchestrator) SetCalendarProvider(provider out.CalendarProviderPort) {
	o.calendarProvider = provider
}

// SetLabelRepository sets the label repository for proposal execution
func (o *Orchestrator) SetLabelRepository(repo domain.LabelRepository) {
	o.labelRepo = repo
}

// AgentRequest represents a request to the AI Agent
type AgentRequest struct {
	UserID    uuid.UUID      `json:"user_id"`
	SessionID string         `json:"session_id"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
}

// AgentResponse represents the AI Agent's response
type AgentResponse struct {
	Message     string                  `json:"message"`
	SessionID   string                  `json:"session_id"`
	Proposals   []*tools.ActionProposal `json:"proposals,omitempty"`
	Data        any                     `json:"data,omitempty"`
	Suggestions []string                `json:"suggestions,omitempty"`
}

// Process handles user requests through the AI Agent
func (o *Orchestrator) Process(ctx context.Context, req *AgentRequest) (*AgentResponse, error) {
	// 1. Get or create session
	session := o.sessionManager.GetOrCreate(req.SessionID, req.UserID)

	// 2. Detect intent and required tools
	intent, err := o.detectIntent(ctx, req.Message, session)
	if err != nil {
		return nil, fmt.Errorf("intent detection failed: %w", err)
	}

	// 3. Gather context from RAG if needed
	ragContext := ""
	if intent.NeedsContext {
		ragContext = o.gatherContext(ctx, req.UserID, req.Message, intent)
	}

	// 4. Execute tools based on intent
	var toolResults []*tools.ToolResult
	var proposals []*tools.ActionProposal

	for _, toolCall := range intent.ToolCalls {
		result, err := o.toolRegistry.Execute(ctx, req.UserID, toolCall.Name, toolCall.Args)
		if err != nil {
			result = &tools.ToolResult{Success: false, Error: err.Error()}
		}

		toolResults = append(toolResults, result)

		// Collect proposals for confirmation
		if result.Proposal != nil {
			o.proposalStore.Store(req.UserID, result.Proposal)
			proposals = append(proposals, result.Proposal)
		}
	}

	// 5. Generate response with LLM
	response, err := o.generateResponse(ctx, req.Message, ragContext, toolResults, intent)
	if err != nil {
		return nil, fmt.Errorf("response generation failed: %w", err)
	}

	// 6. Update session
	session.AddMessage("user", req.Message)
	session.AddMessage("assistant", response)

	return &AgentResponse{
		Message:     response,
		SessionID:   session.ID,
		Proposals:   proposals,
		Suggestions: o.generateSuggestions(intent),
	}, nil
}

// ConfirmProposal confirms and executes a pending proposal
func (o *Orchestrator) ConfirmProposal(ctx context.Context, userID uuid.UUID, proposalID string) (*AgentResponse, error) {
	proposal := o.proposalStore.Get(userID, proposalID)
	if proposal == nil {
		return nil, fmt.Errorf("proposal not found or expired")
	}

	// Execute the confirmed action
	result, err := o.executeProposal(ctx, userID, proposal)
	if err != nil {
		return &AgentResponse{
			Message: fmt.Sprintf("Failed to execute: %s", err.Error()),
		}, nil
	}

	// Remove from pending
	o.proposalStore.Remove(userID, proposalID)

	return &AgentResponse{
		Message: fmt.Sprintf("✓ %s completed successfully", proposal.Description),
		Data:    result,
	}, nil
}

// RejectProposal cancels a pending proposal
func (o *Orchestrator) RejectProposal(ctx context.Context, userID uuid.UUID, proposalID string) error {
	o.proposalStore.Remove(userID, proposalID)
	return nil
}

// ListProposals returns all pending proposals for a user
func (o *Orchestrator) ListProposals(ctx context.Context, userID uuid.UUID) []*tools.ActionProposal {
	return o.proposalStore.List(userID)
}

// Intent represents detected user intent
type Intent struct {
	Type         IntentType       `json:"type"`
	Category     string           `json:"category"` // mail, calendar, contact, search
	Action       string           `json:"action"`   // list, read, create, search, etc.
	ToolCalls    []tools.ToolCall `json:"tool_calls"`
	NeedsContext bool             `json:"needs_context"`
	Parameters   map[string]any   `json:"parameters"`
}

type IntentType string

const (
	IntentQuery    IntentType = "query"    // Information retrieval
	IntentAction   IntentType = "action"   // Perform an action
	IntentAnalysis IntentType = "analysis" // Analyze something
	IntentChat     IntentType = "chat"     // General conversation
)

// detectIntent analyzes user message to determine intent
func (o *Orchestrator) detectIntent(ctx context.Context, message string, session *session.Session) (*Intent, error) {
	// Build prompt for intent detection
	prompt := buildIntentPrompt(message, o.toolRegistry.GetDefinitions())

	// Get LLM response
	response, err := o.llmClient.CompleteJSON(ctx, prompt)
	if err != nil {
		// Fallback to chat intent
		return &Intent{
			Type:         IntentChat,
			NeedsContext: true,
		}, nil
	}

	var intent Intent
	if err := json.Unmarshal([]byte(response), &intent); err != nil {
		return &Intent{Type: IntentChat, NeedsContext: true}, nil
	}

	return &intent, nil
}

// gatherContext retrieves relevant context from RAG
func (o *Orchestrator) gatherContext(ctx context.Context, userID uuid.UUID, query string, intent *Intent) string {
	if o.ragRetriever == nil {
		return ""
	}

	var context string

	// Get email context
	if intent.Category == "" || intent.Category == "mail" {
		results, err := o.ragRetriever.RetrieveForContext(ctx, userID, query, 5)
		if err == nil && len(results) > 0 {
			context += "Related emails:\n"
			for _, r := range results {
				context += fmt.Sprintf("- %s\n", r.Content)
			}
		}
	}

	return context
}

// generateResponse creates the final response using LLM
func (o *Orchestrator) generateResponse(ctx context.Context, userMessage, ragContext string, toolResults []*tools.ToolResult, intent *Intent) (string, error) {
	// Build prompt
	prompt := systemPromptOrchestrator + "\n\n"

	if ragContext != "" {
		prompt += "Context:\n" + ragContext + "\n\n"
	}

	if len(toolResults) > 0 {
		prompt += "Tool Results:\n"
		for _, r := range toolResults {
			resultJSON, _ := json.Marshal(r)
			prompt += string(resultJSON) + "\n"
		}
		prompt += "\n"
	}

	prompt += "User: " + userMessage

	return o.llmClient.Complete(ctx, prompt)
}

// executeProposal executes a confirmed proposal
func (o *Orchestrator) executeProposal(ctx context.Context, userID uuid.UUID, proposal *tools.ActionProposal) (any, error) {
	switch proposal.Action {
	case "mail.send":
		return o.executeSendMail(ctx, userID, proposal.Data)
	case "mail.reply":
		return o.executeReplyMail(ctx, userID, proposal.Data)
	case "mail.delete":
		return o.executeDeleteMail(ctx, userID, proposal.Data)
	case "mail.archive":
		return o.executeArchiveMail(ctx, userID, proposal.Data)
	case "mail.mark_read":
		return o.executeMarkReadMail(ctx, userID, proposal.Data)
	case "mail.star":
		return o.executeStarMail(ctx, userID, proposal.Data)
	case "calendar.create":
		return o.executeCreateEvent(ctx, userID, proposal.Data)
	case "calendar.delete":
		return o.executeDeleteEvent(ctx, userID, proposal.Data)
	case "calendar.update":
		return o.executeUpdateEvent(ctx, userID, proposal.Data)
	case "label.add":
		return o.executeAddLabel(ctx, userID, proposal.Data)
	case "label.remove":
		return o.executeRemoveLabel(ctx, userID, proposal.Data)
	case "label.create":
		return o.executeCreateLabel(ctx, userID, proposal.Data)
	default:
		return nil, fmt.Errorf("unknown action: %s", proposal.Action)
	}
}

func (o *Orchestrator) executeSendMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.emailProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("mail provider or oauth not configured")
	}

	// Extract data
	to := getStringArrayFromAny(data["to"])
	cc := getStringArrayFromAny(data["cc"])
	subject := getStringFromAny(data["subject"])
	body := getStringFromAny(data["body"])
	provider := getStringFromAny(data["provider"])
	isHTML := getBoolFromAny(data["is_html"])

	if len(to) == 0 || subject == "" || body == "" {
		return nil, fmt.Errorf("missing required fields: to, subject, body")
	}

	// Get OAuth connection for the provider
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build outgoing message
	outgoing := &out.ProviderOutgoingMessage{
		Subject: subject,
		Body:    body,
		IsHTML:  isHTML,
	}

	for _, addr := range to {
		outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: addr})
	}
	for _, addr := range cc {
		outgoing.CC = append(outgoing.CC, out.ProviderEmailAddress{Email: addr})
	}

	// Send email
	result, err := o.emailProvider.Send(ctx, token, outgoing)
	if err != nil {
		logger.Error("[Orchestrator.executeSendMail] Failed to send: %v", err)
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	logger.Info("[Orchestrator.executeSendMail] Sent successfully: %s", result.ExternalID)
	return map[string]any{
		"status":      "sent",
		"external_id": result.ExternalID,
		"sent_at":     result.SentAt,
	}, nil
}

func (o *Orchestrator) executeReplyMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.emailProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("mail provider or oauth not configured")
	}

	// Extract data
	originalID := getStringFromAny(data["original_id"])
	to := getStringArrayFromAny(data["to"])
	cc := getStringArrayFromAny(data["cc"])
	subject := getStringFromAny(data["subject"])
	body := getStringFromAny(data["body"])
	provider := getStringFromAny(data["provider"])

	if originalID == "" || body == "" {
		return nil, fmt.Errorf("missing required fields: original_id, body")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build reply message
	outgoing := &out.ProviderOutgoingMessage{
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	}

	for _, addr := range to {
		outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: addr})
	}
	for _, addr := range cc {
		outgoing.CC = append(outgoing.CC, out.ProviderEmailAddress{Email: addr})
	}

	// Send reply
	result, err := o.emailProvider.Reply(ctx, token, originalID, outgoing)
	if err != nil {
		logger.Error("[Orchestrator.executeReplyMail] Failed to reply: %v", err)
		return nil, fmt.Errorf("failed to send reply: %w", err)
	}

	logger.Info("[Orchestrator.executeReplyMail] Replied successfully: %s", result.ExternalID)
	return map[string]any{
		"status":      "replied",
		"external_id": result.ExternalID,
		"sent_at":     result.SentAt,
	}, nil
}

func (o *Orchestrator) executeDeleteMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.emailProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("mail provider or oauth not configured")
	}

	providerID := getStringFromAny(data["provider_id"])
	provider := getStringFromAny(data["provider"])
	permanent := getBoolFromAny(data["permanent"])
	subject := getStringFromAny(data["subject"])

	if providerID == "" {
		return nil, fmt.Errorf("missing required field: provider_id")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Delete email (Trash or permanent delete)
	var action string
	if permanent {
		if err := o.emailProvider.Delete(ctx, token, providerID); err != nil {
			logger.Error("[Orchestrator.executeDeleteMail] Failed to delete: %v", err)
			return nil, fmt.Errorf("failed to delete email: %w", err)
		}
		action = "permanently deleted"
	} else {
		if err := o.emailProvider.Trash(ctx, token, providerID); err != nil {
			logger.Error("[Orchestrator.executeDeleteMail] Failed to trash: %v", err)
			return nil, fmt.Errorf("failed to move email to trash: %w", err)
		}
		action = "moved to trash"
	}

	logger.Info("[Orchestrator.executeDeleteMail] Email %s: %s", action, subject)
	return map[string]any{
		"status":  action,
		"subject": subject,
	}, nil
}

func (o *Orchestrator) executeArchiveMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.emailProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("mail provider or oauth not configured")
	}

	providerID := getStringFromAny(data["provider_id"])
	provider := getStringFromAny(data["provider"])
	subject := getStringFromAny(data["subject"])

	if providerID == "" {
		return nil, fmt.Errorf("missing required field: provider_id")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Archive email (remove from inbox)
	if err := o.emailProvider.Archive(ctx, token, providerID); err != nil {
		logger.Error("[Orchestrator.executeArchiveMail] Failed to archive: %v", err)
		return nil, fmt.Errorf("failed to archive email: %w", err)
	}

	logger.Info("[Orchestrator.executeArchiveMail] Archived: %s", subject)
	return map[string]any{
		"status":  "archived",
		"subject": subject,
	}, nil
}

func (o *Orchestrator) executeMarkReadMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.emailProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("mail provider or oauth not configured")
	}

	providerID := getStringFromAny(data["provider_id"])
	provider := getStringFromAny(data["provider"])
	isRead := getBoolFromAny(data["is_read"])

	if providerID == "" {
		return nil, fmt.Errorf("missing required field: provider_id")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Mark as read/unread
	var status string
	if isRead {
		if err := o.emailProvider.MarkAsRead(ctx, token, providerID); err != nil {
			logger.Error("[Orchestrator.executeMarkReadMail] Failed: %v", err)
			return nil, fmt.Errorf("failed to mark as read: %w", err)
		}
		status = "read"
	} else {
		if err := o.emailProvider.MarkAsUnread(ctx, token, providerID); err != nil {
			logger.Error("[Orchestrator.executeMarkReadMail] Failed: %v", err)
			return nil, fmt.Errorf("failed to mark as unread: %w", err)
		}
		status = "unread"
	}

	logger.Info("[Orchestrator.executeMarkReadMail] Marked as %s", status)
	return map[string]any{
		"status": "marked as " + status,
	}, nil
}

func (o *Orchestrator) executeStarMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.emailProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("mail provider or oauth not configured")
	}

	providerID := getStringFromAny(data["provider_id"])
	provider := getStringFromAny(data["provider"])
	starred := getBoolFromAny(data["starred"])

	if providerID == "" {
		return nil, fmt.Errorf("missing required field: provider_id")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Star/unstar
	var status string
	if starred {
		if err := o.emailProvider.Star(ctx, token, providerID); err != nil {
			logger.Error("[Orchestrator.executeStarMail] Failed: %v", err)
			return nil, fmt.Errorf("failed to star email: %w", err)
		}
		status = "starred"
	} else {
		if err := o.emailProvider.Unstar(ctx, token, providerID); err != nil {
			logger.Error("[Orchestrator.executeStarMail] Failed: %v", err)
			return nil, fmt.Errorf("failed to unstar email: %w", err)
		}
		status = "unstarred"
	}

	logger.Info("[Orchestrator.executeStarMail] Email %s", status)
	return map[string]any{
		"status": status,
	}, nil
}

func (o *Orchestrator) executeCreateEvent(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.calendarProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("calendar provider or oauth not configured")
	}

	// Extract data
	title := getStringFromAny(data["title"])
	description := getStringFromAny(data["description"])
	location := getStringFromAny(data["location"])
	startTimeStr := getStringFromAny(data["start_time"])
	endTimeStr := getStringFromAny(data["end_time"])
	calendarID := getStringFromAny(data["calendar_id"])
	provider := getStringFromAny(data["provider"])
	attendeeEmails := getStringArrayFromAny(data["attendees"])
	notifyAttendees := getBoolFromAny(data["notify_attendees"])

	if title == "" || startTimeStr == "" {
		return nil, fmt.Errorf("missing required fields: title, start_time")
	}

	// Parse times
	startTime, err := parseTimeFlexible(startTimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time format: %w", err)
	}

	var endTime time.Time
	if endTimeStr != "" {
		endTime, err = parseTimeFlexible(endTimeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end_time format: %w", err)
		}
	} else {
		// Default 1 hour duration
		endTime = startTime.Add(1 * time.Hour)
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build attendees
	var attendees []*out.ProviderAttendee
	for _, email := range attendeeEmails {
		attendees = append(attendees, &out.ProviderAttendee{Email: email})
	}

	// Determine notification setting
	sendNotifications := "none"
	if notifyAttendees {
		sendNotifications = "all"
	}

	// Build event
	event := &out.ProviderCalendarEvent{
		Title:             title,
		Description:       description,
		Location:          location,
		StartTime:         startTime,
		EndTime:           endTime,
		Attendees:         attendees,
		SendNotifications: sendNotifications,
	}

	// Default to primary calendar
	if calendarID == "" {
		calendarID = "primary"
	}

	// Create event
	created, err := o.calendarProvider.CreateEvent(ctx, token, calendarID, event)
	if err != nil {
		logger.Error("[Orchestrator.executeCreateEvent] Failed to create event: %v", err)
		return nil, fmt.Errorf("failed to create event: %w", err)
	}

	logger.Info("[Orchestrator.executeCreateEvent] Created event successfully: %s", created.ID)
	return map[string]any{
		"status":     "created",
		"event_id":   created.ID,
		"title":      created.Title,
		"start_time": created.StartTime,
		"end_time":   created.EndTime,
	}, nil
}

func (o *Orchestrator) executeDeleteEvent(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.calendarProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("calendar provider or oauth not configured")
	}

	providerID := getStringFromAny(data["provider_id"])
	calendarID := getStringFromAny(data["calendar_id"])
	provider := getStringFromAny(data["provider"])
	title := getStringFromAny(data["title"])
	notifyAttendees := getBoolFromAny(data["notify_attendees"])

	if providerID == "" {
		return nil, fmt.Errorf("missing required field: provider_id")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Default to primary calendar
	if calendarID == "" {
		calendarID = "primary"
	}

	// Delete event
	// Note: For delete operations, Google Calendar requires sendUpdates as query param
	// Current implementation doesn't send notifications on delete (safest default)
	_ = notifyAttendees // Reserved for future DeleteEventWithOptions interface
	if err := o.calendarProvider.DeleteEvent(ctx, token, calendarID, providerID); err != nil {
		logger.Error("[Orchestrator.executeDeleteEvent] Failed to delete: %v", err)
		return nil, fmt.Errorf("failed to delete event: %w", err)
	}

	logger.Info("[Orchestrator.executeDeleteEvent] Deleted event: %s", title)
	return map[string]any{
		"status": "deleted",
		"title":  title,
	}, nil
}

func (o *Orchestrator) executeUpdateEvent(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.calendarProvider == nil || o.oauthProvider == nil {
		return nil, fmt.Errorf("calendar provider or oauth not configured")
	}

	providerID := getStringFromAny(data["provider_id"])
	calendarID := getStringFromAny(data["calendar_id"])
	provider := getStringFromAny(data["provider"])
	notifyAttendees := getBoolFromAny(data["notify_attendees"])

	if providerID == "" {
		return nil, fmt.Errorf("missing required field: provider_id")
	}

	// Get OAuth connection
	conn, err := o.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := o.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Default to primary calendar
	if calendarID == "" {
		calendarID = "primary"
	}

	// Build update event
	event := &out.ProviderCalendarEvent{}

	if title := getStringFromAny(data["title"]); title != "" {
		event.Title = title
	}
	if description := getStringFromAny(data["description"]); description != "" {
		event.Description = description
	}
	if location := getStringFromAny(data["location"]); location != "" {
		event.Location = location
	}
	if startTime, ok := data["start_time"].(time.Time); ok {
		event.StartTime = startTime
	}
	if endTime, ok := data["end_time"].(time.Time); ok {
		event.EndTime = endTime
	}

	// Set notification setting
	if notifyAttendees {
		event.SendNotifications = "all"
	} else {
		event.SendNotifications = "none"
	}

	// Update event
	updated, err := o.calendarProvider.UpdateEvent(ctx, token, calendarID, providerID, event)
	if err != nil {
		logger.Error("[Orchestrator.executeUpdateEvent] Failed to update: %v", err)
		return nil, fmt.Errorf("failed to update event: %w", err)
	}

	logger.Info("[Orchestrator.executeUpdateEvent] Updated event: %s", updated.Title)
	return map[string]any{
		"status":     "updated",
		"event_id":   updated.ID,
		"title":      updated.Title,
		"start_time": updated.StartTime,
		"end_time":   updated.EndTime,
	}, nil
}

func (o *Orchestrator) executeAddLabel(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.labelRepo == nil {
		return nil, fmt.Errorf("label repository not configured")
	}

	emailID := getInt64FromAny(data["email_id"])
	labelID := getInt64FromAny(data["label_id"])
	labelName := getStringFromAny(data["label_name"])

	if emailID == 0 || labelID == 0 {
		return nil, fmt.Errorf("missing required fields: email_id, label_id")
	}

	// Add label to email
	if err := o.labelRepo.AddEmailLabel(emailID, labelID); err != nil {
		logger.Error("[Orchestrator.executeAddLabel] Failed to add label: %v", err)
		return nil, fmt.Errorf("failed to add label: %w", err)
	}

	logger.Info("[Orchestrator.executeAddLabel] Added label '%s' to email %d", labelName, emailID)
	return map[string]any{
		"status":     "added",
		"email_id":   emailID,
		"label_id":   labelID,
		"label_name": labelName,
	}, nil
}

func (o *Orchestrator) executeRemoveLabel(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.labelRepo == nil {
		return nil, fmt.Errorf("label repository not configured")
	}

	emailID := getInt64FromAny(data["email_id"])
	labelID := getInt64FromAny(data["label_id"])
	labelName := getStringFromAny(data["label_name"])

	if emailID == 0 || labelID == 0 {
		return nil, fmt.Errorf("missing required fields: email_id, label_id")
	}

	// Remove label from email
	if err := o.labelRepo.RemoveEmailLabel(emailID, labelID); err != nil {
		logger.Error("[Orchestrator.executeRemoveLabel] Failed to remove label: %v", err)
		return nil, fmt.Errorf("failed to remove label: %w", err)
	}

	logger.Info("[Orchestrator.executeRemoveLabel] Removed label '%s' from email %d", labelName, emailID)
	return map[string]any{
		"status":     "removed",
		"email_id":   emailID,
		"label_id":   labelID,
		"label_name": labelName,
	}, nil
}

func (o *Orchestrator) executeCreateLabel(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if o.labelRepo == nil {
		return nil, fmt.Errorf("label repository not configured")
	}

	name := getStringFromAny(data["name"])
	color := getStringFromAny(data["color"])

	if name == "" {
		return nil, fmt.Errorf("missing required field: name")
	}

	// Create new label
	label := &domain.Label{
		UserID:    userID,
		Name:      name,
		IsSystem:  false,
		IsVisible: true,
	}

	if color != "" {
		label.Color = &color
	}

	if err := o.labelRepo.Create(label); err != nil {
		logger.Error("[Orchestrator.executeCreateLabel] Failed to create label: %v", err)
		return nil, fmt.Errorf("failed to create label: %w", err)
	}

	logger.Info("[Orchestrator.executeCreateLabel] Created label '%s' with ID %d", name, label.ID)
	return map[string]any{
		"status":   "created",
		"label_id": label.ID,
		"name":     name,
		"color":    color,
	}, nil
}

// Helper functions
func getStringFromAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func getBoolFromAny(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func getInt64FromAny(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	}
	return 0
}

func getStringArrayFromAny(v any) []string {
	if arr, ok := v.([]string); ok {
		return arr
	}
	if arr, ok := v.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// parseTimeFlexible parses time strings in multiple formats
func parseTimeFlexible(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}

// generateSuggestions creates follow-up suggestions
func (o *Orchestrator) generateSuggestions(intent *Intent) []string {
	switch intent.Category {
	case "mail":
		return []string{
			"Show unread emails",
			"Search for important emails",
			"Draft a reply",
		}
	case "calendar":
		return []string{
			"Show today's schedule",
			"Find free time for meeting",
			"Create a new event",
		}
	default:
		return []string{
			"Check my inbox",
			"Show today's calendar",
			"Search contacts",
		}
	}
}

// buildIntentPrompt creates the prompt for intent detection
func buildIntentPrompt(message string, toolDefs []tools.ToolDefinition) string {
	prompt := `Analyze the user's message and determine their intent.

Available tools:
`
	for _, t := range toolDefs {
		prompt += fmt.Sprintf("- %s: %s\n", t.Name, t.Description)
	}

	prompt += `
Return a JSON object with:
{
  "type": "query|action|analysis|chat",
  "category": "mail|calendar|contact|search",
  "action": "specific action like list, read, create, search",
  "tool_calls": [{"name": "tool.name", "args": {...}}],
  "needs_context": true/false,
  "parameters": {...extracted parameters...}
}

User message: ` + message

	return prompt
}

const systemPromptOrchestrator = `You are Workspace AI - an intelligent assistant for email and calendar management.

Your role:
1. Help users manage emails efficiently
2. Organize calendar and schedule meetings
3. Search and find information across emails and contacts
4. Suggest actions and automate repetitive tasks

Guidelines:
- Be concise and helpful
- When proposing actions (send email, create event), clearly describe what will happen
- Use the provided context and tool results to give accurate answers
- Suggest relevant follow-up actions

For actions that modify data, explain the proposal and wait for user confirmation.`

// Helper to check classification result
func (o *Orchestrator) ClassifyEmail(ctx context.Context, email *domain.Email, body string, userRules []domain.ClassificationRule) (*domain.ClassificationResult, error) {
	result, err := o.llmClient.ClassifyEmail(ctx, email.Subject, body, email.FromEmail, userRules)
	if err != nil {
		return nil, err
	}

	category := domain.EmailCategory(result.Category)
	priority := domain.Priority(result.Priority)

	return &domain.ClassificationResult{
		EmailID:  email.ID,
		Category: &category,
		Priority: &priority,
		Summary:  &result.Summary,
		Tags:     result.Tags,
		Score:    result.Score,
	}, nil
}

// GenerateReply generates a reply using RAG for style learning
func (o *Orchestrator) GenerateReply(ctx context.Context, userID uuid.UUID, originalEmail *domain.Email, body string, tone string) (string, error) {
	// Get style context from RAG
	styleContext := ""
	if o.ragRetriever != nil {
		results, err := o.ragRetriever.RetrieveForStyle(ctx, userID, originalEmail.Subject+" "+body, 3)
		if err == nil && len(results) > 0 {
			styleContext = "User's writing style:\n"
			for _, r := range results {
				styleContext += fmt.Sprintf("---\n%s\n", r.Content)
			}
		}
	}

	return o.llmClient.GenerateReplySimple(ctx, originalEmail.Subject, body, originalEmail.FromEmail, styleContext, tone)
}

// SummarizeEmail generates a summary
func (o *Orchestrator) SummarizeEmail(ctx context.Context, subject, body string) (string, error) {
	return o.llmClient.SummarizeEmail(ctx, subject, body)
}
