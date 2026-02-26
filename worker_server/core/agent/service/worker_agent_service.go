package service

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/agent/entity"
	"worker_server/core/agent/llm"
	"worker_server/core/agent/rag"
	"worker_server/core/agent/session"
	"worker_server/core/agent/tools"
	"worker_server/core/domain"
	"worker_server/core/port/in"
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

// AgentService is the unified AI Agent that integrates LLM, RAG, and Tools
type AgentService struct {
	llmClient      *llm.Client
	retriever      *rag.Retriever
	toolRegistry   *tools.Registry
	toolExecutor   *tools.Executor
	proposalStore  *session.ProposalStore
	sessionManager *session.Manager
	config         *entity.AgentConfig

	// Dependencies for proposal execution
	emailProvider     out.EmailProviderPort
	calendarProvider out.CalendarProviderPort
	oauthProvider    OAuthTokenProvider
}

// NewAgentService creates a new agent service with basic capabilities
func NewAgentService(llmClient *llm.Client, retriever *rag.Retriever) *AgentService {
	registry := tools.NewRegistry()
	return &AgentService{
		llmClient:      llmClient,
		retriever:      retriever,
		toolRegistry:   registry,
		toolExecutor:   tools.NewExecutor(registry),
		proposalStore:  session.NewProposalStore(),
		sessionManager: session.NewManager(),
		config:         entity.DefaultAgentConfig(),
	}
}

// NewAgentServiceWithTools creates a fully configured agent service
func NewAgentServiceWithTools(
	llmClient *llm.Client,
	retriever *rag.Retriever,
	toolRegistry *tools.Registry,
) *AgentService {
	return &AgentService{
		llmClient:      llmClient,
		retriever:      retriever,
		toolRegistry:   toolRegistry,
		toolExecutor:   tools.NewExecutor(toolRegistry),
		proposalStore:  session.NewProposalStore(),
		sessionManager: session.NewManager(),
		config:         entity.DefaultAgentConfig(),
	}
}

// SetToolRegistry allows setting a pre-configured tool registry
func (s *AgentService) SetToolRegistry(registry *tools.Registry) {
	s.toolRegistry = registry
	s.toolExecutor = tools.NewExecutor(registry)
}

// RegisterTool adds a tool to the registry
func (s *AgentService) RegisterTool(tool tools.Tool) {
	s.toolRegistry.Register(tool)
}

// RegisterTools adds multiple tools to the registry
func (s *AgentService) RegisterTools(toolList ...tools.Tool) {
	s.toolRegistry.RegisterAll(toolList...)
}

// SetMailProvider sets the mail provider for proposal execution
func (s *AgentService) SetMailProvider(provider out.EmailProviderPort) {
	s.emailProvider = provider
}

// SetOAuthProvider sets the OAuth provider for token management
func (s *AgentService) SetOAuthProvider(provider OAuthTokenProvider) {
	s.oauthProvider = provider
}

// SetCalendarProvider sets the calendar provider for proposal execution
func (s *AgentService) SetCalendarProvider(provider out.CalendarProviderPort) {
	s.calendarProvider = provider
}

// =============================================================================
// Chat Operations
// =============================================================================

// Chat handles a chat request with tool support
func (s *AgentService) Chat(ctx context.Context, userID uuid.UUID, req *in.ChatRequest) (*in.ChatResponse, error) {
	// 1. Get or create session
	session := s.sessionManager.GetOrCreate(req.SessionID, userID)

	// 2. Detect intent and required tools
	intent, err := s.detectIntent(ctx, req.Message, session)
	if err != nil {
		// Fallback to simple chat
		return s.simpleChat(ctx, userID, req, session)
	}

	// 3. Gather context from RAG if needed
	ragContext := ""
	if intent.NeedsContext && s.retriever != nil {
		ragContext = s.gatherContext(ctx, userID, req.Message, intent)
	}

	// 4. Execute tools if needed
	var toolResults []*tools.ToolResult
	var proposals []*tools.ActionProposal

	for _, toolCall := range intent.ToolCalls {
		result, err := s.toolExecutor.Execute(ctx, userID, &toolCall)
		if err != nil {
			result = &tools.ToolResult{Success: false, Error: err.Error()}
		}

		toolResults = append(toolResults, result)

		// Collect proposals for confirmation
		if result.Proposal != nil {
			s.proposalStore.Store(userID, result.Proposal)
			proposals = append(proposals, result.Proposal)
		}
	}

	// 5. Generate response
	response, err := s.generateResponse(ctx, req.Message, ragContext, toolResults, intent)
	if err != nil {
		return nil, fmt.Errorf("response generation failed: %w", err)
	}

	// 6. Update session
	session.AddMessage("user", req.Message)
	session.AddMessage("assistant", response)

	// Build response
	chatResp := &in.ChatResponse{
		Message:     response,
		SessionID:   session.ID,
		Suggestions: s.generateSuggestions(intent),
	}

	// Add proposals if any
	if len(proposals) > 0 {
		chatResp.Proposals = make([]in.ProposalInfo, len(proposals))
		for i, p := range proposals {
			chatResp.Proposals[i] = in.ProposalInfo{
				ID:          p.ID,
				Action:      p.Action,
				Description: p.Description,
				ExpiresAt:   p.ExpiresAt,
			}
		}
	}

	return chatResp, nil
}

// simpleChat handles basic chat without tools
func (s *AgentService) simpleChat(ctx context.Context, userID uuid.UUID, req *in.ChatRequest, sess *session.Session) (*in.ChatResponse, error) {
	// Get RAG context
	ragResults, _ := s.retriever.RetrieveForContext(ctx, userID, req.Message, 5)

	// Build prompt
	prompt := s.buildPrompt(req.Message, ragResults)

	// Get LLM response
	response, err := s.llmClient.CompleteWithSystem(ctx, s.config.SystemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	// Update session
	sess.AddMessage("user", req.Message)
	sess.AddMessage("assistant", response)

	return &in.ChatResponse{
		Message:   response,
		SessionID: sess.ID,
	}, nil
}

// ChatStream handles streaming chat responses
func (s *AgentService) ChatStream(ctx context.Context, userID uuid.UUID, req *in.ChatRequest, handler in.StreamHandler) error {
	// Get RAG context
	ragResults, _ := s.retriever.RetrieveForContext(ctx, userID, req.Message, 5)

	// Build prompt
	prompt := s.buildPrompt(req.Message, ragResults)

	// Stream LLM response
	fullPrompt := s.config.SystemPrompt + "\n\n" + prompt
	return s.llmClient.Stream(ctx, fullPrompt, handler)
}

// =============================================================================
// Proposal Management
// =============================================================================

// ConfirmProposal confirms and executes a pending proposal
func (s *AgentService) ConfirmProposal(ctx context.Context, userID uuid.UUID, proposalID string) (*in.ChatResponse, error) {
	proposal := s.proposalStore.Get(userID, proposalID)
	if proposal == nil {
		return nil, fmt.Errorf("proposal not found or expired")
	}

	// Execute the confirmed action
	result, err := s.executeProposal(ctx, userID, proposal)
	if err != nil {
		return &in.ChatResponse{
			Message: fmt.Sprintf("Failed to execute: %s", err.Error()),
		}, nil
	}

	// Remove from pending
	s.proposalStore.Remove(userID, proposalID)

	return &in.ChatResponse{
		Message: fmt.Sprintf("✓ %s completed successfully", proposal.Description),
		Data:    result,
	}, nil
}

// RejectProposal cancels a pending proposal
func (s *AgentService) RejectProposal(ctx context.Context, userID uuid.UUID, proposalID string) error {
	s.proposalStore.Remove(userID, proposalID)
	return nil
}

// GetPendingProposals returns all pending proposals for a user
func (s *AgentService) GetPendingProposals(ctx context.Context, userID uuid.UUID) []*tools.ActionProposal {
	return s.proposalStore.GetAll(userID)
}

// =============================================================================
// Email Operations (using tools internally)
// =============================================================================

// ClassifyEmail classifies an email using LLM
func (s *AgentService) ClassifyEmail(ctx context.Context, email *domain.Email, body string, userRules []domain.ClassificationRule) (*domain.ClassificationResult, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM client not configured")
	}

	result, err := s.llmClient.ClassifyEmail(ctx, email.Subject, body, email.FromEmail, userRules)
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
func (s *AgentService) GenerateReply(ctx context.Context, userID uuid.UUID, originalEmail *domain.Email, body string, tone string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM client not configured")
	}

	// Get style context from RAG
	styleContext := ""
	if s.retriever != nil {
		results, err := s.retriever.RetrieveForStyle(ctx, userID, originalEmail.Subject+" "+body, 3)
		if err == nil && len(results) > 0 {
			styleContext = "User's writing style examples:\n"
			for _, r := range results {
				styleContext += fmt.Sprintf("---\n%s\n", r.Content)
			}
		}
	}

	return s.llmClient.GenerateReplySimple(ctx, originalEmail.Subject, body, originalEmail.FromEmail, styleContext, tone)
}

// SummarizeEmail generates a summary
func (s *AgentService) SummarizeEmail(ctx context.Context, subject, body string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM client not configured")
	}
	return s.llmClient.SummarizeEmail(ctx, subject, body)
}

// =============================================================================
// Internal Methods
// =============================================================================

// Intent represents detected user intent
type Intent struct {
	Type         IntentType       `json:"type"`
	Category     string           `json:"category"`
	Action       string           `json:"action"`
	ToolCalls    []tools.ToolCall `json:"tool_calls"`
	NeedsContext bool             `json:"needs_context"`
	Parameters   map[string]any   `json:"parameters"`
}

type IntentType string

const (
	IntentQuery    IntentType = "query"
	IntentAction   IntentType = "action"
	IntentAnalysis IntentType = "analysis"
	IntentChat     IntentType = "chat"
)

func (s *AgentService) detectIntent(ctx context.Context, message string, sess *session.Session) (*Intent, error) {
	if s.llmClient == nil {
		return &Intent{Type: IntentChat, NeedsContext: true}, nil
	}

	// Build intent detection prompt
	prompt := s.buildIntentPrompt(message)

	// Get LLM response
	response, err := s.llmClient.CompleteJSON(ctx, prompt)
	if err != nil {
		return &Intent{Type: IntentChat, NeedsContext: true}, nil
	}

	var intent Intent
	if err := json.Unmarshal([]byte(response), &intent); err != nil {
		return &Intent{Type: IntentChat, NeedsContext: true}, nil
	}

	return &intent, nil
}

func (s *AgentService) buildIntentPrompt(message string) string {
	toolDefs := s.toolRegistry.GetDefinitions()

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
  "action": "specific action",
  "tool_calls": [{"id": "unique_id", "name": "tool.name", "args": {...}}],
  "needs_context": true/false,
  "parameters": {...}
}

User message: ` + message

	return prompt
}

func (s *AgentService) gatherContext(ctx context.Context, userID uuid.UUID, query string, intent *Intent) string {
	if s.retriever == nil {
		return ""
	}

	var context string

	// Get relevant email context
	results, err := s.retriever.RetrieveForContext(ctx, userID, query, 5)
	if err == nil && len(results) > 0 {
		context += "Related information:\n"
		for _, r := range results {
			context += fmt.Sprintf("- %s\n", r.Content)
		}
	}

	return context
}

func (s *AgentService) generateResponse(ctx context.Context, userMessage, ragContext string, toolResults []*tools.ToolResult, intent *Intent) (string, error) {
	if s.llmClient == nil {
		return "I'm sorry, but the AI service is not currently available.", nil
	}

	// Build prompt
	prompt := systemPrompt + "\n\n"

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

	return s.llmClient.Complete(ctx, prompt)
}

func (s *AgentService) executeProposal(ctx context.Context, userID uuid.UUID, proposal *tools.ActionProposal) (any, error) {
	switch proposal.Action {
	case "mail.send":
		return s.executeSendMail(ctx, userID, proposal.Data)
	case "mail.reply":
		return s.executeReplyMail(ctx, userID, proposal.Data)
	case "calendar.create":
		return s.executeCreateEvent(ctx, userID, proposal.Data)
	default:
		return nil, fmt.Errorf("unknown action: %s", proposal.Action)
	}
}

func (s *AgentService) executeSendMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if s.emailProvider == nil || s.oauthProvider == nil {
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
	conn, err := s.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthProvider.GetOAuth2Token(ctx, conn.ID)
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
	result, err := s.emailProvider.Send(ctx, token, outgoing)
	if err != nil {
		logger.Error("[AgentService.executeSendMail] Failed to send: %v", err)
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	logger.Info("[AgentService.executeSendMail] Sent successfully: %s", result.ExternalID)
	return map[string]any{
		"status":      "sent",
		"external_id": result.ExternalID,
		"sent_at":     result.SentAt,
	}, nil
}

func (s *AgentService) executeReplyMail(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if s.emailProvider == nil || s.oauthProvider == nil {
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
	conn, err := s.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthProvider.GetOAuth2Token(ctx, conn.ID)
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
	result, err := s.emailProvider.Reply(ctx, token, originalID, outgoing)
	if err != nil {
		logger.Error("[AgentService.executeReplyMail] Failed to reply: %v", err)
		return nil, fmt.Errorf("failed to send reply: %w", err)
	}

	logger.Info("[AgentService.executeReplyMail] Replied successfully: %s", result.ExternalID)
	return map[string]any{
		"status":      "replied",
		"external_id": result.ExternalID,
		"sent_at":     result.SentAt,
	}, nil
}

func (s *AgentService) executeCreateEvent(ctx context.Context, userID uuid.UUID, data map[string]any) (any, error) {
	if s.calendarProvider == nil || s.oauthProvider == nil {
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
		endTime = startTime.Add(1 * time.Hour)
	}

	// Get OAuth connection
	conn, err := s.oauthProvider.GetConnectionByUserID(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthProvider.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build attendees
	var attendees []*out.ProviderAttendee
	for _, email := range attendeeEmails {
		attendees = append(attendees, &out.ProviderAttendee{Email: email})
	}

	// Build event
	event := &out.ProviderCalendarEvent{
		Title:       title,
		Description: description,
		Location:    location,
		StartTime:   startTime,
		EndTime:     endTime,
		Attendees:   attendees,
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	// Create event
	created, err := s.calendarProvider.CreateEvent(ctx, token, calendarID, event)
	if err != nil {
		logger.Error("[AgentService.executeCreateEvent] Failed to create event: %v", err)
		return nil, fmt.Errorf("failed to create event: %w", err)
	}

	logger.Info("[AgentService.executeCreateEvent] Created event successfully: %s", created.ID)
	return map[string]any{
		"status":     "created",
		"event_id":   created.ID,
		"title":      created.Title,
		"start_time": created.StartTime,
		"end_time":   created.EndTime,
	}, nil
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

// Helper functions for type conversion
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

func (s *AgentService) generateSuggestions(intent *Intent) []string {
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
	case "contact":
		return []string{
			"Search contacts",
			"Find contact by email",
			"List recent contacts",
		}
	default:
		return []string{
			"Check my inbox",
			"Show today's calendar",
			"Search contacts",
		}
	}
}

func (s *AgentService) buildPrompt(message string, ragResults []*rag.RetrievalResult) string {
	prompt := ""

	if len(ragResults) > 0 {
		prompt += "Relevant context from user's emails:\n\n"
		for i, r := range ragResults {
			if i >= 3 {
				break
			}
			prompt += "---\n" + r.Content + "\n---\n\n"
		}
		prompt += "\n"
	}

	prompt += "User message: " + message

	return prompt
}

const systemPrompt = `You are Workspace AI - an intelligent assistant for email, calendar, and contact management.

Your capabilities:
1. Help users manage and organize their emails efficiently
2. Schedule meetings and manage calendar events
3. Search and find information across emails and contacts
4. Suggest actions and automate repetitive tasks
5. Generate email replies in the user's writing style

Guidelines:
- Be concise, professional, and helpful
- When proposing actions (send email, create event), clearly describe what will happen
- Use the provided context and tool results to give accurate, personalized answers
- Suggest relevant follow-up actions when appropriate
- For actions that modify data, explain the proposal and wait for user confirmation`
