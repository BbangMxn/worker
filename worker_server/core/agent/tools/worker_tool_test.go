package tools

import (
	"context"
	"testing"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// Mock repositories for testing
type mockEmailRepository struct {
	emails map[int64]*domain.Email
	bodies map[int64]*domain.EmailBody
}

func newMockEmailRepo() *mockEmailRepository {
	userID := uuid.New()
	now := time.Now()

	return &mockEmailRepository{
		emails: map[int64]*domain.Email{
			1: {
				ID:        1,
				UserID:    userID,
				Subject:   "Test Email 1",
				FromEmail: "sender@example.com",
				FromName:  strPtr("Test Sender"),
				ToEmails:  []string{"user@example.com"},
				Date:      now.Add(-1 * time.Hour),
				Folder:    domain.LegacyFolderInbox,
				IsRead:    false,
				AISummary: strPtr("This is a test email"),
			},
			2: {
				ID:        2,
				UserID:    userID,
				Subject:   "Meeting Request",
				FromEmail: "boss@company.com",
				FromName:  strPtr("Boss"),
				ToEmails:  []string{"user@example.com"},
				Date:      now.Add(-2 * time.Hour),
				Folder:    domain.LegacyFolderInbox,
				IsRead:    true,
				AISummary: strPtr("Meeting scheduled for tomorrow"),
			},
		},
		bodies: map[int64]*domain.EmailBody{
			1: {EmailID: 1, TextBody: "Hello, this is a test email body."},
			2: {EmailID: 2, TextBody: "Please join the meeting tomorrow at 10am."},
		},
	}
}

func (r *mockEmailRepository) GetByID(id int64) (*domain.Email, error) {
	if email, ok := r.emails[id]; ok {
		return email, nil
	}
	return nil, nil
}

func (r *mockEmailRepository) List(filter *domain.EmailFilter) ([]*domain.Email, int, error) {
	var result []*domain.Email
	for _, email := range r.emails {
		if email.UserID == filter.UserID {
			result = append(result, email)
		}
	}
	return result, len(result), nil
}

func (r *mockEmailRepository) GetBody(emailID int64) (*domain.EmailBody, error) {
	if body, ok := r.bodies[emailID]; ok {
		return body, nil
	}
	return nil, nil
}

// Implement other required methods with empty implementations
func (r *mockEmailRepository) GetByProviderID(userID uuid.UUID, provider domain.Provider, providerID string) (*domain.Email, error) {
	return nil, nil
}
func (r *mockEmailRepository) GetByThreadID(threadID string) ([]*domain.Email, error) {
	return nil, nil
}
func (r *mockEmailRepository) GetByDateRange(userID uuid.UUID, startDate, endDate time.Time) ([]*domain.Email, error) {
	return nil, nil
}
func (r *mockEmailRepository) Create(email *domain.Email) error         { return nil }
func (r *mockEmailRepository) CreateBatch(emails []*domain.Email) error { return nil }
func (r *mockEmailRepository) Update(email *domain.Email) error         { return nil }
func (r *mockEmailRepository) Delete(id int64) error                    { return nil }
func (r *mockEmailRepository) SaveBody(body *domain.EmailBody) error    { return nil }

type mockCalendarRepository struct {
	calendars map[int64]*domain.Calendar
	events    map[int64]*domain.CalendarEvent
}

func newMockCalendarRepo() *mockCalendarRepository {
	userID := uuid.New()
	now := time.Now()

	return &mockCalendarRepository{
		calendars: map[int64]*domain.Calendar{
			1: {
				ID:        1,
				UserID:    userID,
				Provider:  domain.CalendarProviderGoogle,
				Name:      "Primary",
				IsDefault: true,
			},
			2: {
				ID:        2,
				UserID:    userID,
				Provider:  domain.CalendarProviderMicrosoft,
				Name:      "Work",
				IsDefault: false,
			},
		},
		events: map[int64]*domain.CalendarEvent{
			1: {
				ID:         1,
				CalendarID: 1,
				UserID:     userID,
				Title:      "Team Meeting",
				StartTime:  now.Add(24 * time.Hour),
				EndTime:    now.Add(25 * time.Hour),
				Location:   strPtr("Conference Room A"),
			},
		},
	}
}

func (r *mockCalendarRepository) GetCalendarsByUser(userID uuid.UUID) ([]*domain.Calendar, error) {
	var result []*domain.Calendar
	for _, cal := range r.calendars {
		result = append(result, cal)
	}
	return result, nil
}

func (r *mockCalendarRepository) ListEvents(filter *domain.CalendarEventFilter) ([]*domain.CalendarEvent, int, error) {
	var result []*domain.CalendarEvent
	for _, event := range r.events {
		result = append(result, event)
	}
	return result, len(result), nil
}

func (r *mockCalendarRepository) GetCalendarByID(id int64) (*domain.Calendar, error) {
	if cal, ok := r.calendars[id]; ok {
		return cal, nil
	}
	return nil, nil
}
func (r *mockCalendarRepository) CreateCalendar(cal *domain.Calendar) error { return nil }
func (r *mockCalendarRepository) UpdateCalendar(cal *domain.Calendar) error { return nil }
func (r *mockCalendarRepository) DeleteCalendar(id int64) error             { return nil }
func (r *mockCalendarRepository) GetEventByID(id int64) (*domain.CalendarEvent, error) {
	return nil, nil
}
func (r *mockCalendarRepository) CreateEvent(event *domain.CalendarEvent) error { return nil }
func (r *mockCalendarRepository) UpdateEvent(event *domain.CalendarEvent) error { return nil }
func (r *mockCalendarRepository) DeleteEvent(id int64) error                    { return nil }

type mockContactRepository struct {
	contacts map[int64]*domain.Contact
}

func newMockContactRepo() *mockContactRepository {
	userID := uuid.New()

	return &mockContactRepository{
		contacts: map[int64]*domain.Contact{
			1: {
				ID:      1,
				UserID:  userID,
				Name:    "John Doe",
				Email:   "john@example.com",
				Phone:   "+1234567890",
				Company: "Acme Corp",
			},
			2: {
				ID:      2,
				UserID:  userID,
				Name:    "Jane Smith",
				Email:   "jane@example.com",
				Company: "Tech Inc",
			},
		},
	}
}

func (r *mockContactRepository) List(filter *domain.ContactFilter) ([]*domain.Contact, int, error) {
	var result []*domain.Contact
	for _, contact := range r.contacts {
		result = append(result, contact)
	}
	return result, len(result), nil
}

func (r *mockContactRepository) GetByID(id int64) (*domain.Contact, error) {
	if contact, ok := r.contacts[id]; ok {
		return contact, nil
	}
	return nil, nil
}

func (r *mockContactRepository) GetByEmail(userID uuid.UUID, email string) (*domain.Contact, error) {
	for _, contact := range r.contacts {
		if contact.Email == email {
			return contact, nil
		}
	}
	return nil, nil
}

func (r *mockContactRepository) Create(contact *domain.Contact) error { return nil }
func (r *mockContactRepository) Update(contact *domain.Contact) error { return nil }
func (r *mockContactRepository) Delete(id int64) error                { return nil }
func (r *mockContactRepository) GetCompanyByID(id int64) (*domain.Company, error) {
	return nil, nil
}
func (r *mockContactRepository) GetCompanyByDomain(userID uuid.UUID, d string) (*domain.Company, error) {
	return nil, nil
}
func (r *mockContactRepository) ListCompanies(userID uuid.UUID, limit, offset int) ([]*domain.Company, int, error) {
	return nil, 0, nil
}
func (r *mockContactRepository) CreateCompany(company *domain.Company) error { return nil }
func (r *mockContactRepository) UpdateCompany(company *domain.Company) error { return nil }
func (r *mockContactRepository) DeleteCompany(id int64) error                { return nil }

func strPtr(s string) *string {
	return &s
}

// Tests
func TestRegistryBasics(t *testing.T) {
	registry := NewRegistry()

	// Register tools
	emailRepo := newMockEmailRepo()
	registry.Register(NewMailListTool(emailRepo))
	registry.Register(NewMailReadTool(emailRepo))

	// Test list
	tools := registry.List()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Test get
	tool, err := registry.Get("mail.list")
	if err != nil {
		t.Errorf("failed to get tool: %v", err)
	}
	if tool.Name() != "mail.list" {
		t.Errorf("expected mail.list, got %s", tool.Name())
	}

	// Test get non-existent
	_, err = registry.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent tool")
	}
}

func TestMailListTool(t *testing.T) {
	emailRepo := newMockEmailRepo()
	tool := NewMailListTool(emailRepo)

	// Get userID from mock
	var userID uuid.UUID
	for _, email := range emailRepo.emails {
		userID = email.UserID
		break
	}

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"folder": "inbox",
		"limit":  10,
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	data, ok := result.Data.([]map[string]any)
	if !ok {
		t.Errorf("expected []map[string]any, got %T", result.Data)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 emails, got %d", len(data))
	}
}

func TestMailReadTool(t *testing.T) {
	emailRepo := newMockEmailRepo()
	tool := NewMailReadTool(emailRepo)

	var userID uuid.UUID
	for _, email := range emailRepo.emails {
		userID = email.UserID
		break
	}

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"email_id": float64(1), // JSON numbers come as float64
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Errorf("expected map[string]any, got %T", result.Data)
	}
	if data["subject"] != "Test Email 1" {
		t.Errorf("expected 'Test Email 1', got %v", data["subject"])
	}
}

func TestMailSendToolProposal(t *testing.T) {
	tool := NewMailSendTool()
	userID := uuid.New()

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"to":       []any{"recipient@example.com"},
		"subject":  "Test Subject",
		"body":     "Test body content",
		"provider": "gmail",
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Should return a proposal, not execute directly
	if result.Proposal == nil {
		t.Error("expected proposal for mail.send")
	}
	if result.Proposal.Action != "mail.send" {
		t.Errorf("expected action 'mail.send', got %s", result.Proposal.Action)
	}
}

func TestCalendarListTool(t *testing.T) {
	calRepo := newMockCalendarRepo()
	tool := NewCalendarListTool(calRepo)
	userID := uuid.New()

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"provider":   "all",
		"start_date": "today",
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestCalendarCreateToolProposal(t *testing.T) {
	calRepo := newMockCalendarRepo()
	tool := NewCalendarCreateTool(calRepo)
	userID := uuid.New()

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"title":      "New Meeting",
		"start_time": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"end_time":   time.Now().Add(25 * time.Hour).Format(time.RFC3339),
		"provider":   "google",
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Should return a proposal
	if result.Proposal == nil {
		t.Error("expected proposal for calendar.create")
	}
	if result.Proposal.Action != "calendar.create" {
		t.Errorf("expected action 'calendar.create', got %s", result.Proposal.Action)
	}
}

func TestContactListTool(t *testing.T) {
	contactRepo := newMockContactRepo()
	tool := NewContactListTool(contactRepo)
	userID := uuid.New()

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"limit": 10,
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	data, ok := result.Data.([]map[string]any)
	if !ok {
		t.Errorf("expected []map[string]any, got %T", result.Data)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 contacts, got %d", len(data))
	}
}

func TestContactSearchTool(t *testing.T) {
	contactRepo := newMockContactRepo()
	tool := NewContactSearchTool(contactRepo)
	userID := uuid.New()

	result, err := tool.Execute(context.Background(), userID, map[string]any{
		"email": "john@example.com",
	})

	if err != nil {
		t.Errorf("execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Errorf("expected map[string]any, got %T", result.Data)
	}
	if data["name"] != "John Doe" {
		t.Errorf("expected 'John Doe', got %v", data["name"])
	}
}

func TestToolDefinitions(t *testing.T) {
	registry := NewRegistry()

	emailRepo := newMockEmailRepo()
	calRepo := newMockCalendarRepo()
	contactRepo := newMockContactRepo()

	// Register all tools
	registry.RegisterAll(
		NewMailListTool(emailRepo),
		NewMailReadTool(emailRepo),
		NewMailSearchTool(emailRepo),
		NewMailSendTool(),
		NewMailReplyTool(emailRepo),
		NewCalendarListTool(calRepo),
		NewCalendarCreateTool(calRepo),
		NewCalendarFindFreeTool(calRepo),
		NewContactListTool(contactRepo),
		NewContactGetTool(contactRepo),
		NewContactSearchTool(contactRepo),
	)

	defs := registry.GetDefinitions()
	if len(defs) != 11 {
		t.Errorf("expected 11 tool definitions, got %d", len(defs))
	}

	// Check definition format
	for _, def := range defs {
		if def.Name == "" {
			t.Error("tool definition missing name")
		}
		if def.Description == "" {
			t.Errorf("tool %s missing description", def.Name)
		}
		if def.Parameters.Type != "object" {
			t.Errorf("tool %s parameters type should be 'object'", def.Name)
		}
	}
}

func TestExecutor(t *testing.T) {
	registry := NewRegistry()
	emailRepo := newMockEmailRepo()
	registry.Register(NewMailListTool(emailRepo))

	executor := NewExecutor(registry)

	var userID uuid.UUID
	for _, email := range emailRepo.emails {
		userID = email.UserID
		break
	}

	result, err := executor.Execute(context.Background(), userID, &ToolCall{
		ID:   "test-1",
		Name: "mail.list",
		Args: map[string]any{"folder": "inbox"},
	})

	if err != nil {
		t.Errorf("executor failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestExecutorMissingParameter(t *testing.T) {
	registry := NewRegistry()
	emailRepo := newMockEmailRepo()
	registry.Register(NewMailReadTool(emailRepo))

	executor := NewExecutor(registry)
	userID := uuid.New()

	result, err := executor.Execute(context.Background(), userID, &ToolCall{
		ID:   "test-1",
		Name: "mail.read",
		Args: map[string]any{}, // Missing required email_id
	})

	if err != nil {
		t.Errorf("executor should not return error for missing param: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing required parameter")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestParseRelativeDate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		input    string
		expected time.Time
	}{
		{"today", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())},
		{"오늘", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())},
		{"tomorrow", time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())},
		{"내일", time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())},
	}

	for _, test := range tests {
		result := parseRelativeDate(test.input, now)
		if !result.Equal(test.expected) {
			t.Errorf("parseRelativeDate(%s): expected %v, got %v", test.input, test.expected, result)
		}
	}
}
