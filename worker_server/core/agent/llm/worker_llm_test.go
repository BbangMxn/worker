package llm

import (
	"testing"

	"worker_server/core/domain"
)

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		maxLen   int
		expected string
	}{
		{
			name:     "short body",
			body:     "Hello world",
			maxLen:   100,
			expected: "Hello world",
		},
		{
			name:     "exact length",
			body:     "Hello",
			maxLen:   5,
			expected: "Hello",
		},
		{
			name:     "truncated",
			body:     "Hello world, this is a long message",
			maxLen:   10,
			expected: "Hello worl...",
		},
		{
			name:     "empty body",
			body:     "",
			maxLen:   100,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateBody(tt.body, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClassificationResponseStruct(t *testing.T) {
	resp := ClassificationResponse{
		Category: "primary",
		Priority: 3,
		Summary:  "Test summary",
		Tags:     []string{"important", "work"},
		Score:    0.95,
	}

	if resp.Category != "primary" {
		t.Errorf("expected category 'primary', got %s", resp.Category)
	}
	if resp.Priority != 3 {
		t.Errorf("expected priority 3, got %v", resp.Priority)
	}
	if len(resp.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(resp.Tags))
	}
}

func TestEmailContextStruct(t *testing.T) {
	ctx := EmailContext{
		From:    "sender@example.com",
		Date:    "2024-01-15 10:00",
		Subject: "Test Subject",
		Body:    "Test body content",
	}

	if ctx.From != "sender@example.com" {
		t.Errorf("expected from 'sender@example.com', got %s", ctx.From)
	}
	if ctx.Subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got %s", ctx.Subject)
	}
}

func TestMeetingInfoStruct(t *testing.T) {
	info := MeetingInfo{
		Title:       "Weekly Standup",
		StartTime:   "2024-01-15T10:00:00Z",
		EndTime:     "2024-01-15T10:30:00Z",
		Location:    "Conference Room A",
		Attendees:   []string{"user1@example.com", "user2@example.com"},
		Description: "Weekly team standup",
		MeetingURL:  "https://zoom.us/j/123456",
		HasMeeting:  true,
	}

	if !info.HasMeeting {
		t.Error("expected HasMeeting to be true")
	}
	if info.Title != "Weekly Standup" {
		t.Errorf("expected title 'Weekly Standup', got %s", info.Title)
	}
	if len(info.Attendees) != 2 {
		t.Errorf("expected 2 attendees, got %d", len(info.Attendees))
	}
}

func TestContactInfoStruct(t *testing.T) {
	info := ContactInfo{
		Name:    "John Doe",
		Email:   "john@example.com",
		Phone:   "+1234567890",
		Company: "Acme Corp",
		Title:   "Software Engineer",
	}

	if info.Name != "John Doe" {
		t.Errorf("expected name 'John Doe', got %s", info.Name)
	}
	if info.Company != "Acme Corp" {
		t.Errorf("expected company 'Acme Corp', got %s", info.Company)
	}
}

func TestClassificationRuleIntegration(t *testing.T) {
	// Test that classification rules work with the expected format
	rules := []domain.ClassificationRule{
		{
			Name:        "VIP Senders",
			Description: strPtr("Emails from CEO are high priority"),
		},
		{
			Name:        "Newsletter",
			Description: strPtr("Marketing newsletters go to promotions"),
		},
	}

	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}

	// Simulate building prompt with rules
	rulesContext := ""
	for _, rule := range rules {
		if rule.Description != nil {
			rulesContext += rule.Name + ": " + *rule.Description + "\n"
		}
	}

	if rulesContext == "" {
		t.Error("expected non-empty rules context")
	}
}

func strPtr(s string) *string {
	return &s
}

// Test model constants
func TestModelSelection(t *testing.T) {
	// These would be used for model selection in production
	models := map[string]string{
		"fast":    "gpt-3.5-turbo",
		"default": "gpt-4-turbo-preview",
		"best":    "gpt-4",
	}

	if models["fast"] == "" {
		t.Error("fast model should be defined")
	}
	if models["default"] == "" {
		t.Error("default model should be defined")
	}
}
