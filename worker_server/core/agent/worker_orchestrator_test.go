package agent

import (
	"testing"
	"time"

	"worker_server/core/agent/session"
	"worker_server/core/agent/tools"

	"github.com/google/uuid"
)

func TestProposalStore(t *testing.T) {
	store := session.NewProposalStore()
	userID := uuid.New()

	// Create proposal
	proposal := &tools.ActionProposal{
		ID:          "test-proposal-1",
		Action:      "mail.send",
		Description: "Send email to test@example.com",
		Data: map[string]any{
			"to":      []string{"test@example.com"},
			"subject": "Test",
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	// Store
	store.Store(userID, proposal)

	// Get
	retrieved := store.Get(userID, "test-proposal-1")
	if retrieved == nil {
		t.Error("expected to retrieve proposal")
	}
	if retrieved.Action != "mail.send" {
		t.Errorf("expected action 'mail.send', got %s", retrieved.Action)
	}

	// Get non-existent
	notFound := store.Get(userID, "non-existent")
	if notFound != nil {
		t.Error("expected nil for non-existent proposal")
	}

	// Remove
	store.Remove(userID, "test-proposal-1")
	removed := store.Get(userID, "test-proposal-1")
	if removed != nil {
		t.Error("expected nil after removal")
	}
}

func TestProposalStoreExpiration(t *testing.T) {
	store := session.NewProposalStore()
	userID := uuid.New()

	// Create expired proposal
	proposal := &tools.ActionProposal{
		ID:          "expired-proposal",
		Action:      "mail.send",
		Description: "Expired proposal",
		ExpiresAt:   time.Now().Add(-1 * time.Minute), // Already expired
	}

	store.Store(userID, proposal)

	// Should not retrieve expired proposal
	retrieved := store.Get(userID, "expired-proposal")
	if retrieved != nil {
		t.Error("expected nil for expired proposal")
	}
}

func TestSessionManager(t *testing.T) {
	manager := session.NewManager()
	userID := uuid.New()

	// Create new session
	session1 := manager.GetOrCreate("", userID)
	if session1.ID == "" {
		t.Error("expected session ID to be generated")
	}
	if session1.UserID != userID {
		t.Error("expected session to have correct user ID")
	}

	// Get existing session
	session2 := manager.GetOrCreate(session1.ID, userID)
	if session2.ID != session1.ID {
		t.Error("expected same session ID")
	}

	// Create with specific ID
	session3 := manager.GetOrCreate("custom-session-id", userID)
	if session3.ID != "custom-session-id" {
		t.Errorf("expected 'custom-session-id', got %s", session3.ID)
	}
}

func TestSessionMessages(t *testing.T) {
	s := &session.Session{
		ID:        "test-session",
		UserID:    uuid.New(),
		Messages:  []session.Message{},
		CreatedAt: time.Now(),
	}

	// Add messages
	s.AddMessage("user", "Hello")
	s.AddMessage("assistant", "Hi there!")

	if len(s.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(s.Messages))
	}

	// Check message content
	if s.Messages[0].Role != "user" {
		t.Errorf("expected role 'user', got %s", s.Messages[0].Role)
	}
	if s.Messages[0].Content != "Hello" {
		t.Errorf("expected content 'Hello', got %s", s.Messages[0].Content)
	}
}

func TestSessionMessageLimit(t *testing.T) {
	s := &session.Session{
		ID:        "test-session",
		UserID:    uuid.New(),
		Messages:  []session.Message{},
		CreatedAt: time.Now(),
	}

	// Add 25 messages (over limit of 20)
	for i := 0; i < 25; i++ {
		s.AddMessage("user", "Message")
	}

	// Should only keep last 20
	if len(s.Messages) != 20 {
		t.Errorf("expected 20 messages, got %d", len(s.Messages))
	}
}

func TestSessionGetRecentContext(t *testing.T) {
	s := &session.Session{
		ID:        "test-session",
		UserID:    uuid.New(),
		Messages:  []session.Message{},
		CreatedAt: time.Now(),
	}

	s.AddMessage("user", "First message")
	s.AddMessage("assistant", "First response")
	s.AddMessage("user", "Second message")
	s.AddMessage("assistant", "Second response")

	context := s.GetRecentContext(2)

	// Should contain last 2 messages
	if context == "" {
		t.Error("expected non-empty context")
	}
}

func TestIntentTypes(t *testing.T) {
	// Test intent type constants
	if IntentQuery != "query" {
		t.Errorf("expected 'query', got %s", IntentQuery)
	}
	if IntentAction != "action" {
		t.Errorf("expected 'action', got %s", IntentAction)
	}
	if IntentAnalysis != "analysis" {
		t.Errorf("expected 'analysis', got %s", IntentAnalysis)
	}
	if IntentChat != "chat" {
		t.Errorf("expected 'chat', got %s", IntentChat)
	}
}

func TestAgentRequestResponse(t *testing.T) {
	userID := uuid.New()

	req := &AgentRequest{
		UserID:    userID,
		SessionID: "test-session",
		Message:   "Show me my unread emails",
		Context:   map[string]any{"source": "web"},
	}

	if req.UserID != userID {
		t.Error("request user ID mismatch")
	}
	if req.Message != "Show me my unread emails" {
		t.Error("request message mismatch")
	}

	resp := &AgentResponse{
		Message:   "You have 5 unread emails",
		SessionID: "test-session",
		Proposals: nil,
		Suggestions: []string{
			"Read first email",
			"Mark all as read",
		},
	}

	if resp.Message == "" {
		t.Error("response should have message")
	}
	if len(resp.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(resp.Suggestions))
	}
}

func TestGenerateSuggestions(t *testing.T) {
	o := &Orchestrator{}

	// Mail intent
	mailIntent := &Intent{Category: "mail"}
	mailSuggestions := o.generateSuggestions(mailIntent)
	if len(mailSuggestions) == 0 {
		t.Error("expected mail suggestions")
	}

	// Calendar intent
	calIntent := &Intent{Category: "calendar"}
	calSuggestions := o.generateSuggestions(calIntent)
	if len(calSuggestions) == 0 {
		t.Error("expected calendar suggestions")
	}

	// Default intent
	defaultIntent := &Intent{Category: ""}
	defaultSuggestions := o.generateSuggestions(defaultIntent)
	if len(defaultSuggestions) == 0 {
		t.Error("expected default suggestions")
	}
}

func TestBuildIntentPrompt(t *testing.T) {
	toolDefs := []tools.ToolDefinition{
		{
			Name:        "mail.list",
			Description: "List emails",
			Category:    tools.CategoryMail,
		},
		{
			Name:        "calendar.list",
			Description: "List calendar events",
			Category:    tools.CategoryCalendar,
		},
	}

	prompt := buildIntentPrompt("Show me my emails", toolDefs)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	// Should contain tool names
	if !containsString(prompt, "mail.list") {
		t.Error("prompt should contain mail.list")
	}
	if !containsString(prompt, "calendar.list") {
		t.Error("prompt should contain calendar.list")
	}
	if !containsString(prompt, "Show me my emails") {
		t.Error("prompt should contain user message")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
